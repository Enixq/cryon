package core

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"Cryon2/internal/config"
	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/playback"
	"Cryon2/internal/recommendations"
	"Cryon2/internal/services"
	"Cryon2/internal/services/audiofetcher"
	"Cryon2/internal/services/lastfm"
	"Cryon2/internal/services/local"
	"Cryon2/internal/services/lyrics"
	"Cryon2/internal/services/soundcloud"
	"Cryon2/internal/services/spotify"
	"Cryon2/internal/services/yandex"
	"Cryon2/internal/services/youtube"
	"Cryon2/internal/store"
	storeSqlite "Cryon2/internal/store/sqlite"

	"golang.org/x/crypto/bcrypt"
)

const appName = "Cryon2"
const sourceStatusChangedEvent = "source:status-changed"
const favoritesChangedEvent = "favorites:changed"
const notificationsChangedEvent = "notifications:changed"

// recoChangedEvent — профиль вкусов изменился (пользователь оценил трек).
// Фронтенд по нему сбрасывает кэш всех подборок: оценка должна отражаться
// сразу, а не после истечения TTL готовой подборки.
const recoChangedEvent = "reco:changed"

// FavoritesImportResult — итог импорта избранного из внешнего сервиса.
// Экспортируется в Wails, поэтому фронтенд получает понятный прогресс.
type FavoritesImportResult struct {
	Source   string `json:"source"`
	Found    int    `json:"found"`
	Imported int    `json:"imported"`
}

// AccountConnectionStatus is a token-safe description of an external music
// account integration. Client credentials and tokens are deliberately never
// included in this value: it is returned to the WebView UI.
type AccountConnectionStatus struct {
	Service          string `json:"service"`
	Name             string `json:"name"`
	OAuthConfigured  bool   `json:"oauthConfigured"`
	AccountConnected bool   `json:"accountConnected"`
	CanImportLibrary bool   `json:"canImportLibrary"`
	SetupHint        string `json:"setupHint"`
}

// LocalAccount — локальный профиль, открывающий доступ к приложению. Данные
// остаются на компьютере и не синхронизируются с внешними сервисами.
type LocalAccount = store.Account

// notifCounter обеспечивает уникальность id оповещений при генерации в один
// и тот же наносекундный момент.
var notifCounter uint64

// App — корневой сервис приложения, связывающий адаптеры источников
// и локальное хранилище. Методы с большой буквы доступны фронтенду
// через Wails-биндинги.
type App struct {
	ctx      context.Context
	cfg      config.Config
	registry map[domain.ServiceID]domain.MusicService
	local    *local.Service
	player   *playback.Controller
	store    store.Store
	reco     *recommendations.Engine
	lyrics   *lyrics.Client
	mu       sync.Mutex
	statusMu sync.RWMutex
	// validationErrors хранит только результат явной проверки API в текущем
	// запуске. Это не подменяет конфигурационный статус, а дополняет его.
	validationErrors map[domain.ServiceID]string
	// runtimeErrors фиксирует источники, отказавшие при реальном обращении
	// (пользовательский поиск, новинки, периодический health-check). В отличие
	// от validationErrors (явная проверка ключей пользователем) наполняется
	// автоматически и снимается при первом успешном ответе источника.
	runtimeErrors map[domain.ServiceID]string
	yaLogin       yandexLogin
	oauth         *oauthCoordinator
	// platform абстрагирует зависящие от среды вызовы (события, браузер,
	// диалоги) за интерфейсом, чтобы логика App не зависела от Wails-рантайма
	// напрямую. Десктоп внедряет wailsHost (в main), мобайл — SSE/no-op. По
	// умолчанию nullHost, поэтому поле никогда не nil (тесты, ранний старт).
	platform platformHost
}

func NewApp() *App {
	cfg := config.Load()
	registry, localSvc := services.BuildRegistry(cfg)
	app := &App{
		cfg:              cfg,
		registry:         registry,
		local:            localSvc,
		player:           playback.New(cfg.MPVPath),
		validationErrors: make(map[domain.ServiceID]string),
		runtimeErrors:    make(map[domain.ServiceID]string),
		platform:         nullHost{}, // десктоп заменит на wailsHost в main
	}
	app.lyrics = lyrics.New()
	app.oauth = newOAuthCoordinator(app)
	st, err := storeSqlite.New(appName)
	if err != nil {
		logging.L().Error("не удалось подготовить хранилище", "err", err)
	} else {
		app.store = st
	}
	// Движок рекомендаций поверх истории/избранного и Last.fm (если задан ключ).
	// Поиск делегируем SearchAll, чтобы находить реальные проигрываемые треки.
	app.reco = recommendations.New(app.store, lastfm.New(cfg.LastFMAPIKey), func(ctx context.Context, query string) ([]domain.Track, error) {
		return app.SearchAll(query)
	})
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.store != nil {
		if err := a.store.Init(ctx); err != nil {
			logging.L().Error("не удалось инициализировать БД", "err", err)
		} else {
			a.loadLocalLibraryCache(ctx)
			a.addDefaultMusicFolder(ctx)
			a.restoreYandexToken(ctx)
			a.restoreLastFMKey(ctx)
			a.restoreServiceCredentials(ctx)
		}
	}
	logging.L().Info("Cryon2 запущен")
	// Периодический live-контроль доступности источников (задача 38).
	a.startHealthChecks(ctx)
	// LAN-сервер для телефона (только сборка `wails build -tags cryonlan`;
	// иначе no-op из lanserver_stub.go).
	a.startLANServer()
}

func (a *App) CurrentAccount() (LocalAccount, bool, error) {
	if a.store == nil || a.ctx == nil {
		return LocalAccount{}, false, nil
	}
	return a.store.AccountCurrent(a.ctx)
}

func (a *App) RegisterAccount(login, email, password string) (LocalAccount, error) {
	if a.store == nil || a.ctx == nil {
		return LocalAccount{}, fmt.Errorf("хранилище недоступно")
	}
	login, email = strings.TrimSpace(login), strings.TrimSpace(email)
	if len([]rune(login)) < 3 {
		return LocalAccount{}, fmt.Errorf("логин должен содержать не менее 3 символов")
	}
	if !strings.Contains(email, "@") {
		return LocalAccount{}, fmt.Errorf("укажите корректный email")
	}
	if len(password) < 8 {
		return LocalAccount{}, fmt.Errorf("пароль должен содержать не менее 8 символов")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return LocalAccount{}, fmt.Errorf("не удалось защитить пароль: %w", err)
	}
	return a.store.AccountRegister(a.ctx, login, email, string(hash))
}

func (a *App) LoginAccount(login, password string) (LocalAccount, error) {
	if a.store == nil || a.ctx == nil {
		return LocalAccount{}, fmt.Errorf("хранилище недоступно")
	}
	account, hash, err := a.store.AccountLogin(a.ctx, login)
	if err != nil {
		return LocalAccount{}, fmt.Errorf("неверный логин или пароль")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return LocalAccount{}, fmt.Errorf("неверный логин или пароль")
	}
	if err := a.store.AccountActivate(a.ctx, account.ID); err != nil {
		return LocalAccount{}, err
	}
	return account, nil
}

func (a *App) ContinueAsGuest() (LocalAccount, error) {
	if a.store == nil || a.ctx == nil {
		return LocalAccount{}, fmt.Errorf("хранилище недоступно")
	}
	return a.store.AccountSetGuest(a.ctx)
}

func (a *App) LogoutAccount() error {
	if a.store == nil || a.ctx == nil {
		return nil
	}
	return a.store.AccountLogout(a.ctx)
}

// ImportFavorites импортирует любимые треки подключённого источника в локальное
// избранное Cryon. Дубликаты безопасно игнорируются уникальным ключом SQLite.
// Сейчас поддержан Yandex Music — его OAuth уже подключается в SettingsPage.
func (a *App) ImportFavorites(source string) (FavoritesImportResult, error) {
	result := FavoritesImportResult{Source: strings.TrimSpace(source)}
	if a.store == nil {
		return result, fmt.Errorf("хранилище недоступно")
	}
	if a.ctx == nil {
		return result, fmt.Errorf("приложение ещё не запущено")
	}
	if result.Source == string(domain.ServiceSpotify) {
		token, ok, err := a.store.TokenGet(a.ctx, domain.ServiceSpotify)
		if err != nil || !ok || strings.TrimSpace(token.AccessToken) == "" {
			return result, fmt.Errorf("сначала подключите аккаунт Spotify")
		}
		tracks, refreshed, err := spotify.LikedTracks(a.ctx, token.AccessToken)
		if err != nil && token.RefreshToken != "" {
			clientID, _, _ := a.store.SettingGet(a.ctx, keySpotifyOAuthClientID)
			updated, refreshErr := refreshSpotifyToken(a.ctx, clientID, token.RefreshToken)
			if refreshErr == nil {
				_ = a.store.TokenUpsert(a.ctx, domain.ServiceSpotify, updated)
				tracks, refreshed, err = spotify.LikedTracks(a.ctx, updated.AccessToken)
			}
		}
		if err != nil {
			return result, err
		}
		_ = refreshed
		result.Found = len(tracks)
		for _, track := range tracks {
			if err := a.store.FavoriteAdd(a.ctx, track); err != nil {
				return result, err
			}
			result.Imported++
		}
		a.platform.Emit(favoritesChangedEvent)
		a.pushNotification("success", "Импорт Spotify завершён", fmt.Sprintf("Обработано треков: %d из %d.", result.Imported, result.Found))
		return result, nil
	}
	if result.Source != string(domain.ServiceYandex) {
		return result, fmt.Errorf("импорт избранного для «%s» пока не поддержан", result.Source)
	}

	service, ok := a.registry[domain.ServiceYandex].(*yandex.Service)
	if !ok {
		return result, fmt.Errorf("адаптер Yandex Music недоступен")
	}
	tracks, err := service.LikedTracks(a.ctx)
	if err != nil {
		return result, err
	}
	result.Found = len(tracks)
	for _, track := range tracks {
		if err := a.store.FavoriteAdd(a.ctx, track); err != nil {
			return result, fmt.Errorf("не удалось сохранить «%s»: %w", track.Title, err)
		}
		result.Imported++
	}
	a.platform.Emit(favoritesChangedEvent)
	a.pushNotification("success", "Импорт избранного завершён", fmt.Sprintf("Yandex Music: обработано треков: %d из %d; дубликаты пропущены.", result.Imported, result.Found))
	return result, nil
}

// PickSpotifyLibraryExport открывает выбор выгрузки персональных данных Spotify.
// Такой импорт не требует OAuth Client ID: Spotify формирует архив на странице
// Account privacy, а приложение читает только YourLibrary.json локально.
func (a *App) PickSpotifyLibraryExport() (string, error) {
	return a.platform.PickFile(
		"Выберите выгрузку Spotify (ZIP или YourLibrary.json)",
		"Spotify export",
		"*.zip;*.json",
	)
}

// ImportSpotifyLibraryExport импортирует liked songs из официальной выгрузки
// Spotify. Файл остаётся на устройстве и никуда не загружается.
func (a *App) ImportSpotifyLibraryExport(path string) (FavoritesImportResult, error) {
	result := FavoritesImportResult{Source: string(domain.ServiceSpotify)}
	if a.store == nil || a.ctx == nil {
		return result, fmt.Errorf("хранилище недоступно")
	}
	tracks, err := spotifyLibraryTracks(path)
	if err != nil {
		return result, err
	}
	result.Found = len(tracks)
	for _, track := range tracks {
		if err := a.store.FavoriteAdd(a.ctx, track); err != nil {
			return result, err
		}
		result.Imported++
	}
	a.platform.Emit(favoritesChangedEvent)
	a.pushNotification("success", "Импорт Spotify завершён", fmt.Sprintf("Из выгрузки добавлено треков: %d из %d.", result.Imported, result.Found))
	return result, nil
}

func spotifyLibraryTracks(path string) ([]domain.Track, error) {
	var data []byte
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		archive, err := zip.OpenReader(path)
		if err != nil {
			return nil, fmt.Errorf("не удалось открыть ZIP-выгрузку Spotify: %w", err)
		}
		defer archive.Close()
		for _, file := range archive.File {
			if strings.EqualFold(filepath.Base(file.Name), "YourLibrary.json") {
				reader, openErr := file.Open()
				if openErr != nil {
					return nil, openErr
				}
				data, err = io.ReadAll(reader)
				reader.Close()
				if err != nil {
					return nil, err
				}
				break
			}
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("в выгрузке не найден файл YourLibrary.json")
		}
	} else {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать файл выгрузки: %w", err)
		}
	}
	var rows []struct {
		Track  string `json:"track"`
		Artist string `json:"artist"`
		Album  string `json:"album"`
		URI    string `json:"uri"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("некорректный YourLibrary.json: %w", err)
	}
	tracks := make([]domain.Track, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Track) == "" || strings.TrimSpace(row.Artist) == "" {
			continue
		}
		id := strings.TrimPrefix(row.URI, "spotify:track:")
		externalURL := ""
		if id == row.URI || id == "" {
			id = "export:" + row.Artist + ":" + row.Track
		} else {
			externalURL = "https://open.spotify.com/track/" + url.PathEscape(id)
		}
		tracks = append(tracks, domain.Track{ID: id, Service: domain.ServiceSpotify, Title: row.Track, Artists: []string{row.Artist}, Album: row.Album, ExternalURL: externalURL, PlayableKind: domain.PlayableExternal})
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("в выгрузке не найдено сохранённых треков")
	}
	return tracks, nil
}

// addDefaultMusicFolder добавляет только стандартный каталог пользователя
// «Музыка» и только при совершенно пустом списке источников. Downloads и
// произвольные каталоги намеренно не трогаем: там часто лежат временные файлы
// и пользователь не ожидает их индексирования. LocalFolderAdd идемпотентен,
// однако проверка списка не позволяет делать сканирование на каждом старте.
func (a *App) addDefaultMusicFolder(ctx context.Context) {
	folders, err := a.store.LocalFolderList(ctx)
	if err != nil || len(folders) != 0 {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	musicDir := filepath.Join(home, "Music")
	info, err := os.Stat(musicDir)
	if err != nil || !info.IsDir() {
		return
	}
	if err := a.store.LocalFolderAdd(ctx, musicDir); err != nil {
		logging.L().Warn("не удалось добавить стандартную папку музыки", "folder", musicDir, "err", err)
		return
	}
	tracks, err := a.RescanLocalLibrary()
	if err != nil {
		logging.L().Warn("не удалось просканировать стандартную папку музыки", "folder", musicDir, "err", err)
		return
	}
	logging.L().Info("добавлена стандартная папка музыки", "folder", musicDir, "tracks", len(tracks))
	if len(tracks) > 0 {
		a.pushNotification("success", "Локальная музыка найдена", fmt.Sprintf("Добавлена папка «Музыка»: найдено треков: %d.", len(tracks)))
	}
}

// keyYandexToken — ключ настройки с сохранённым OAuth-токеном Yandex Music.
const keyYandexToken = "yandex.token"

// keyLastFMAPIKey — ключ настройки с сохранённым API-ключом Last.fm.
const keyLastFMAPIKey = "lastfm.apikey"

// Ключи настроек с учётными данными источников, задаваемыми из UI.
// Хранятся локально в SQLite и применяются к адаптерам при старте.
const (
	keySpotifyClientID     = "spotify.clientID"
	keySpotifyClientSecret = "spotify.clientSecret"
	keySoundCloudClientID  = "soundcloud.clientID"
	keyYouTubeAPIKey       = "youtube.apiKey"
	// OAuth application identifiers are local application configuration. They
	// are different from a listener's account tokens, which live in tokens.
	keySpotifyOAuthClientID    = "spotify.oauth.clientID"
	keyYouTubeOAuthClientID    = "youtube.oauth.clientID"
	keySoundCloudOAuthClientID = "soundcloud.oauth.clientID"
	keySoundCloudOAuthSecret   = "soundcloud.oauth.clientSecret"
)

// restoreYandexToken подхватывает сохранённый в настройках токен Yandex Music,
// чтобы прямой поток работал между запусками без переменной окружения.
func (a *App) restoreYandexToken(ctx context.Context) {
	saved, _, err := a.store.SettingGet(ctx, keyYandexToken)
	if err != nil || strings.TrimSpace(saved) == "" {
		return
	}
	if ya, ok := a.registry[domain.ServiceYandex].(*yandex.Service); ok {
		ya.SetToken(saved)
		logging.L().Info("yandex: токен восстановлен из настроек")
	}
}

// restoreLastFMKey подхватывает сохранённый в настройках ключ Last.fm, чтобы
// онлайн-рекомендации работали между запусками без переменной окружения.
func (a *App) restoreLastFMKey(ctx context.Context) {
	saved, _, err := a.store.SettingGet(ctx, keyLastFMAPIKey)
	if err != nil || strings.TrimSpace(saved) == "" {
		return
	}
	if a.reco != nil {
		a.reco.SetLastFMKey(saved)
		logging.L().Info("lastfm: ключ восстановлен из настроек")
	}
}

// restoreServiceCredentials подхватывает сохранённые в настройках учётные данные
// источников (Spotify client_id/secret, SoundCloud client_id, YouTube API-ключ),
// чтобы подключение сохранялось между запусками без переменных окружения.
func (a *App) restoreServiceCredentials(ctx context.Context) {
	get := func(key string) string {
		v, _, err := a.store.SettingGet(ctx, key)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(v)
	}

	if sp, ok := a.registry[domain.ServiceSpotify].(*spotify.Service); ok {
		id, secret := get(keySpotifyClientID), get(keySpotifyClientSecret)
		if id != "" && secret != "" {
			sp.SetCredentials(id, secret)
			logging.L().Info("spotify: учётные данные восстановлены из настроек")
		}
	}
	if sc, ok := a.registry[domain.ServiceSoundCloud].(*soundcloud.Service); ok {
		if id := get(keySoundCloudClientID); id != "" {
			sc.SetClientID(id)
			logging.L().Info("soundcloud: client_id восстановлен из настроек")
		}
	}
	if yt, ok := a.registry[domain.ServiceYouTube].(*youtube.Service); ok {
		if key := get(keyYouTubeAPIKey); key != "" {
			yt.SetAPIKey(key)
			logging.L().Info("youtube: API-ключ восстановлен из настроек")
		}
	}
}

func (a *App) emitSourceStatusChanged() {
	if a.ctx != nil {
		a.platform.Emit(sourceStatusChangedEvent)
	}
}

// loadLocalLibraryCache подхватывает ранее просканированные локальные треки
// из БД, чтобы библиотека была доступна сразу при запуске без пересканирования.
func (a *App) loadLocalLibraryCache(ctx context.Context) {
	rows, err := a.store.LocalTrackList(ctx)
	if err != nil {
		logging.L().Warn("не удалось загрузить кэш локальной библиотеки", "err", err)
		return
	}
	tracks := make([]local.LocalTrack, 0, len(rows))
	for _, r := range rows {
		tracks = append(tracks, local.LocalTrack{
			Track: domain.Track{
				ID:           r.ID,
				Service:      domain.ServiceLocal,
				Title:        r.Title,
				Artists:      r.Artists,
				Album:        r.Album,
				PlayableKind: domain.PlayableStream,
			},
			FilePath: r.FilePath,
		})
	}
	a.local.Replace(tracks)
}

func (a *App) shutdown(_ context.Context) {
	if a.player != nil {
		a.player.Shutdown()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
}

// recoverGoroutine перехватывает панику в ФОНОВОЙ горутине backend, чтобы она
// не уронила весь процесс. net/http перехватывает панику только в горутине
// самого хендлера, но НЕ в горутинах, которые тот порождает (фан-аут поиска,
// подбор потока, радар новинок). На Android процесс один на всё приложение,
// поэтому непойманная паника в такой горутине = мгновенное закрытие приложения.
// where — метка места вызова для лога.
func (a *App) recoverGoroutine(where string) {
	if r := recover(); r != nil {
		logging.L().Error("перехвачена паника в фоновой горутине", "where", where, "panic", r)
	}
}

// Search ищет треки в одном источнике по его идентификатору.
func (a *App) Search(service string, query string) ([]domain.Track, error) {
	svc, ok := a.registry[domain.ServiceID(service)]
	if !ok {
		return nil, fmt.Errorf("неизвестный источник: %s", service)
	}
	ctx, cancel := context.WithTimeout(a.ctx, searchSourceTimeout)
	defer cancel()
	tracks, err := svc.Search(ctx, query)
	a.recordSourceResult(domain.ServiceID(service), err)
	if err != nil {
		logging.L().Warn("поиск завершился ошибкой", "service", service, "err", err)
		return nil, err
	}
	return tracks, nil
}

// SearchAll параллельно опрашивает все источники и объединяет результаты.
func (a *App) SearchAll(query string) ([]domain.Track, error) {
	return a.SearchInSources(query, nil)
}

// searchExcluded — источники, исключённые из агрегированного поиска. Их выдача
// либо мусорная, либо ведёт только на внешние ссылки, которые засоряют очередь
// невоспроизводимыми треками. Адаптеры остаются в реестре ради других функций
// (Spotify — OAuth-импорт Liked Songs и «Радар новинок»), но SearchAll/
// SearchInSources их не опрашивают. Прямой Search(service, …) по-прежнему
// доступен, если источник запрошен явно.
var searchExcluded = map[domain.ServiceID]bool{
	domain.ServiceSpotify: true,
}

// searchSourceTimeout — предел ожидания одного источника в агрегированном
// поиске. Ниже собственных http-таймаутов адаптеров (например, 15 с у
// SoundCloud): без общего дедлайна самый медленный/деградировавший источник
// держал бы всю выдачу до своего таймаута, теперь он просто выпадает, а
// остальные отдаются сразу. Заодно ограничивает время жизни «осиротевших»
// горутин, если пользователь уже начал новый поиск.
const searchSourceTimeout = 8 * time.Second

// SearchInSources параллельно ищет только в указанных источниках. Пустой
// список означает «во всех», что сохраняет обратную совместимость SearchAll.
func (a *App) SearchInSources(query string, sourceIDs []string) ([]domain.Track, error) {
	selected := make(map[domain.ServiceID]bool, len(sourceIDs))
	for _, source := range sourceIDs {
		if id := domain.ServiceID(source); a.registry[id] != nil {
			selected[id] = true
		}
	}

	var wg sync.WaitGroup
	results := make(map[domain.ServiceID][]domain.Track, len(a.registry))
	var mu sync.Mutex

	for id, svc := range a.registry {
		if len(selected) > 0 && !selected[id] {
			continue
		}
		// Исключённые из поиска источники (Spotify) пропускаем даже во «всех»,
		// чтобы внешние ссылки не забивали выдачу и очередь.
		if searchExcluded[id] {
			continue
		}
		wg.Add(1)
		go func(id domain.ServiceID, svc domain.MusicService) {
			defer wg.Done()
			defer a.recoverGoroutine("SearchInSources")
			ctx, cancel := context.WithTimeout(a.ctx, searchSourceTimeout)
			defer cancel()
			tracks, err := svc.Search(ctx, query)
			a.recordSourceResult(id, err)
			if err != nil {
				logging.L().Warn("поиск источника завершился ошибкой", "service", id, "err", err)
				return
			}
			mu.Lock()
			results[id] = tracks
			mu.Unlock()
		}(id, svc)
	}
	wg.Wait()

	// Стабильный порядок источников для предсказуемой выдачи. Spotify и VK
	// исключены из поиска (см. searchExcluded / реестр), поэтому здесь их нет.
	order := []domain.ServiceID{
		domain.ServiceYouTube,
		domain.ServiceSoundCloud,
		domain.ServiceYandex,
		domain.ServiceLocal,
	}
	total := 0
	for _, tracks := range results {
		total += len(tracks)
	}
	out := make([]domain.Track, 0, total)
	for _, id := range order {
		out = append(out, results[id]...)
	}
	return out, nil
}

// ResolvePlayableTrack подбирает прямую, наиболее предпочтительную версию
// трека перед воспроизведением. Это особенно полезно для импортированных
// записей (Spotify/VK), у которых нет собственного аудиопотока. Совпадение
// намеренно строгое: нормализованные исполнитель и название должны совпасть,
// чтобы не подменять композицию кавером, ремиксом или другим треком.
//
// Порядок определён продуктовым правилом: SoundCloud, Yandex Music, локальная
// библиотека, YouTube Music. Если надёжную потоковую версию найти не удалось,
// возвращается исходный трек без изменения его поведения.
//
// Метод лежит на горячем пути старта воспроизведения, поэтому источники
// опрашиваются только для треков без собственного потока — и все сразу,
// параллельно. Выбор при этом остаётся детерминированным: результаты
// раскладываются по индексам, и берётся совпадение из самого приоритетного
// источника, а не то, что ответило первым.
func (a *App) ResolvePlayableTrack(track domain.Track) (domain.Track, error) {
	// У трека уже есть прямой поток — подменять нечего. Это подавляющее
	// большинство запусков (YouTube/SoundCloud/Yandex/локальные файлы), поэтому
	// без этой проверки каждый старт трека стоил бы до четырёх сетевых поисков.
	if track.PlayableKind == domain.PlayableStream {
		return track, nil
	}
	query := strings.TrimSpace(strings.Join(append(append([]string{}, track.Artists...), track.Title), " "))
	if query == "" {
		return track, nil
	}

	sources := []domain.ServiceID{
		domain.ServiceSoundCloud,
		domain.ServiceYandex,
		domain.ServiceLocal,
		domain.ServiceYouTube,
	}
	// Источники опрашиваются параллельно, а результат берётся строго по
	// приоритету списка: раньше это были четыре последовательных поиска, что
	// давало заметную задержку перед первым звуком.
	type resolved struct {
		track domain.Track
		ok    bool
	}
	found := make([]resolved, len(sources))

	var wg sync.WaitGroup
	for i, source := range sources {
		svc, ok := a.registry[source]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(i int, source domain.ServiceID, svc domain.MusicService) {
			defer wg.Done()
			defer a.recoverGoroutine("ResolvePlayableTrack")
			ctx, cancel := context.WithTimeout(a.ctx, searchSourceTimeout)
			defer cancel()
			candidates, err := svc.Search(ctx, query)
			if err != nil {
				logging.L().Debug("не удалось подобрать альтернативный источник", "service", source, "err", err)
				return
			}
			for _, candidate := range candidates {
				if candidate.PlayableKind != domain.PlayableStream || !sameTrack(track, candidate) {
					continue
				}
				found[i] = resolved{track: candidate, ok: true}
				return
			}
		}(i, source, svc)
	}
	wg.Wait()

	for _, r := range found {
		if !r.ok {
			continue
		}
		logging.L().Info("выбрана альтернативная версия трека", "from", track.Service, "to", r.track.Service, "title", track.Title)
		return r.track, nil
	}
	return track, nil
}

func sameTrack(a, b domain.Track) bool {
	if normalizeTrackText(a.Title) != normalizeTrackText(b.Title) {
		return false
	}
	return normalizeTrackText(strings.Join(a.Artists, " ")) == normalizeTrackText(strings.Join(b.Artists, " "))
}

func normalizeTrackText(value string) string {
	value = strings.ToLower(value)
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		// 'ё' вне диапазона 'а'..'я' и раньше просто выбрасывалась, из-за чего
		// «Ёлка» и «Елка» считались разными треками. Складываем её в 'е'.
		if r == 'ё' {
			r = 'е'
		}
		if (r >= 'a' && r <= 'z') || (r >= 'а' && r <= 'я') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// GetAudioStream возвращает прямой аудиопоток трека, если источник его поддерживает.
func (a *App) GetAudioStream(service string, trackID string) (*audiofetcher.AudioStream, error) {
	switch domain.ServiceID(service) {
	case domain.ServiceYouTube:
		return audiofetcher.GetYouTubeAudioStream(a.ctx, trackID)
	case domain.ServiceSoundCloud:
		sc, ok := a.registry[domain.ServiceSoundCloud].(*soundcloud.Service)
		if !ok {
			return nil, fmt.Errorf("адаптер SoundCloud недоступен")
		}
		info, err := sc.ResolveStream(a.ctx, trackID)
		if err != nil {
			return nil, err
		}
		return &audiofetcher.AudioStream{
			URL:      info.URL,
			Format:   info.MimeType,
			Duration: int64(info.DurationMs / 1000),
		}, nil
	case domain.ServiceYandex:
		ya, ok := a.registry[domain.ServiceYandex].(*yandex.Service)
		if !ok {
			return nil, fmt.Errorf("адаптер Yandex Music недоступен")
		}
		info, err := ya.ResolveStream(a.ctx, trackID)
		if err != nil {
			return nil, err
		}
		return &audiofetcher.AudioStream{
			URL:      info.URL,
			Format:   info.MimeType,
			Duration: int64(info.DurationMs / 1000),
		}, nil
	case domain.ServiceLocal:
		path, ok := a.local.FilePath(trackID)
		if !ok {
			return nil, fmt.Errorf("локальный трек не найден: %s", trackID)
		}
		return &audiofetcher.AudioStream{
			URL:    path,
			Format: "local",
		}, nil
	default:
		return nil, fmt.Errorf("источник %s пока не поддерживает прямое воспроизведение", service)
	}
}

// AddFavorite добавляет трек в локальные избранные.
func (a *App) AddFavorite(t domain.Track) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	if err := a.store.FavoriteAdd(a.ctx, t); err != nil {
		return err
	}
	if a.ctx != nil {
		a.platform.Emit(favoritesChangedEvent)
	}
	return nil
}

// RemoveFavorite удаляет трек из избранных.
func (a *App) RemoveFavorite(service string, trackID string) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	if err := a.store.FavoriteRemove(a.ctx, domain.ServiceID(service), trackID); err != nil {
		return err
	}
	if a.ctx != nil {
		a.platform.Emit(favoritesChangedEvent)
	}
	return nil
}

// ListFavorites возвращает избранные треки.
func (a *App) ListFavorites() ([]domain.Track, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	return a.store.FavoriteList(a.ctx)
}

// --- Оповещения (колокольчик) ---

// pushNotification сохраняет оповещение в SQLite и уведомляет фронтенд событием
// notifications:changed, чтобы колокольчик перечитал список. Вызывается из
// backend при реальных событиях (истёк токен, завершение сканирования, ошибки
// воспроизведения и т.п.). Ошибки хранилища не роняют вызывающий код — только
// логируются, оповещения не критичны.
func (a *App) pushNotification(kind, title, message string) {
	if a.store == nil {
		return
	}
	row := store.NotificationRow{
		ID:          fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&notifCounter, 1)),
		Kind:        kind,
		Title:       title,
		Message:     message,
		CreatedAtMs: time.Now().UnixMilli(),
		Read:        false,
	}
	if err := a.store.NotificationAdd(a.ctx, row); err != nil {
		logging.L().Warn("не удалось сохранить оповещение", "err", err)
		return
	}
	if a.ctx != nil {
		a.platform.Emit(notificationsChangedEvent)
	}
}

// AddNotification сохраняет оповещение, инициированное с фронтенда (например,
// ошибка воспроизведения из audioEngine). kind — info/success/warning/error.
func (a *App) AddNotification(kind, title, message string) error {
	if a.store == nil {
		return nil
	}
	if strings.TrimSpace(kind) == "" {
		kind = "info"
	}
	a.pushNotification(kind, title, message)
	return nil
}

// ListNotifications возвращает последние оповещения (новые сверху), не более 50.
func (a *App) ListNotifications() ([]store.NotificationRow, error) {
	if a.store == nil {
		return []store.NotificationRow{}, nil
	}
	rows, err := a.store.NotificationList(a.ctx, 50)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []store.NotificationRow{}
	}
	return rows, nil
}

// MarkAllNotificationsRead помечает все оповещения прочитанными.
func (a *App) MarkAllNotificationsRead() error {
	if a.store == nil {
		return nil
	}
	if err := a.store.NotificationMarkAllRead(a.ctx); err != nil {
		return err
	}
	a.platform.Emit(notificationsChangedEvent)
	return nil
}

// RemoveNotification удаляет одно оповещение по id.
func (a *App) RemoveNotification(id string) error {
	if a.store == nil {
		return nil
	}
	if err := a.store.NotificationRemove(a.ctx, id); err != nil {
		return err
	}
	a.platform.Emit(notificationsChangedEvent)
	return nil
}

// ClearNotifications удаляет все оповещения.
func (a *App) ClearNotifications() error {
	if a.store == nil {
		return nil
	}
	if err := a.store.NotificationClear(a.ctx); err != nil {
		return err
	}
	a.platform.Emit(notificationsChangedEvent)
	return nil
}

// --- Рекомендации ---

// ListRecommendations возвращает до limit рекомендованных треков на основе
// истории и избранного (похожие артисты Last.fm при наличии ключа, иначе
// оффлайн-фолбэк по любимым артистам). Пустой список — если данных о вкусах
// ещё нет.
func (a *App) ListRecommendations(limit int) ([]domain.Track, error) {
	if a.reco == nil {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	tracks, err := a.reco.Recommendations(a.ctx, limit)
	if err != nil {
		logging.L().Warn("рекомендации не удалось получить", "err", err)
		return []domain.Track{}, nil
	}
	return tracks, nil
}

// RecommendationsAvailable сообщает, доступен ли онлайн-движок (задан ли ключ
// Last.fm). false — работает только оффлайн-фолбэк.
func (a *App) RecommendationsAvailable() bool {
	return a.reco != nil && a.reco.OnlineAvailable()
}

// SetLastFMKey сохраняет API-ключ Last.fm и сразу применяет его к движку
// рекомендаций. С валидным ключом включается онлайн-режим (похожие
// артисты/треки cross-service); пустая строка возвращает в оффлайн-фолбэк.
// Бесплатный ключ выдаётся на last.fm/api/account/create.
func (a *App) SetLastFMKey(apiKey string) error {
	if a.reco == nil {
		return fmt.Errorf("движок рекомендаций недоступен")
	}
	apiKey = strings.TrimSpace(apiKey)
	a.reco.SetLastFMKey(apiKey)
	if a.store != nil {
		if err := a.store.SettingSet(a.ctx, keyLastFMAPIKey, apiKey); err != nil {
			return err
		}
	}
	// Сообщаем фронтенду, чтобы экран настроек и Home обновили статус без
	// перезагрузки.
	if a.ctx != nil {
		a.platform.Emit("lastfm:changed")
	}
	return nil
}

// LastFMConnected сообщает, задан ли ключ Last.fm (без раскрытия самого ключа).
func (a *App) LastFMConnected() bool {
	return a.reco != nil && a.reco.OnlineAvailable()
}

// ListAutoMix возвращает авто-подборку: kind = "daily" («Микс дня») или
// "weekly" («Открытия недели»). Подборка стабильна в пределах суток/недели и
// обновляется сама. Пустой список — если данных о вкусах ещё нет.
func (a *App) ListAutoMix(kind string, limit int) ([]domain.Track, error) {
	if a.reco == nil {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	tracks, err := a.reco.AutoMix(a.ctx, recommendations.MixKind(kind), limit)
	if err != nil {
		logging.L().Warn("авто-подборку не удалось получить", "kind", kind, "err", err)
		return []domain.Track{}, nil
	}
	return tracks, nil
}

// ListDailyMix — авто-подборка «Микс дня» (стабильна в течение суток).
func (a *App) ListDailyMix(limit int) ([]domain.Track, error) {
	return a.ListAutoMix(string(recommendations.MixDaily), limit)
}

// ListWeeklyDiscoveries — авто-подборка «Открытия недели» (стабильна в течение
// недели, уклон в менее знакомых артистов).
func (a *App) ListWeeklyDiscoveries(limit int) ([]domain.Track, error) {
	return a.ListAutoMix(string(recommendations.MixWeekly), limit)
}

// ListRelatedTracks возвращает до limit треков, похожих на seed — основа умной
// очереди (радио): когда очередь заканчивается, фронтенд дозаполняет её этими
// треками. Пустой список — если похожих не нашлось.
func (a *App) ListRelatedTracks(seed domain.Track, limit int) ([]domain.Track, error) {
	if a.reco == nil {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 15
	}
	tracks, err := a.reco.RelatedTracks(a.ctx, seed, limit)
	if err != nil {
		logging.L().Warn("похожие треки не удалось получить", "err", err)
		return []domain.Track{}, nil
	}
	return tracks, nil
}

// SetRecoFeedback сохраняет оценку рекомендации: score = 1 («нравится»),
// -1 («не нравится»), 0 (снять оценку). Повторный клик по той же кнопке фронтенд
// присылает как 0. Возвращает актуальную карту оценок «исполнитель|название» →
// оценка, чтобы UI подсветил кнопки без второго запроса.
//
// Оценка попадает в профиль вкусов: «нравится» усиливает исполнителя сильнее
// избранного, накопленные «не нравится» полностью исключают его из подборок,
// а оценённый трек больше не предлагается.
func (a *App) SetRecoFeedback(artist, title string, score int) (map[string]int, error) {
	if a.reco == nil {
		return map[string]int{}, nil
	}
	state, err := a.reco.SetFeedback(a.ctx, artist, title, score)
	if err != nil {
		logging.L().Warn("оценку рекомендации не удалось сохранить", "artist", artist, "title", title, "err", err)
		return map[string]int{}, err
	}
	if a.ctx != nil {
		a.platform.Emit(recoChangedEvent)
	}
	return state, nil
}

// RecoFeedbackState отдаёт сохранённые оценки рекомендаций для подсветки кнопок
// «нравится»/«не нравится» на карточках.
func (a *App) RecoFeedbackState() (map[string]int, error) {
	if a.reco == nil {
		return map[string]int{}, nil
	}
	state, err := a.reco.FeedbackState(a.ctx)
	if err != nil {
		return map[string]int{}, nil
	}
	return state, nil
}

// ListNewReleases — «Радар новинок»: свежие релизы, агрегированные из
// источников, которые умеют их отдавать (реализуют domain.NewReleaser: Yandex с
// токеном, Spotify с ключами). Дедуп по паре артист+название. Результат
// кэшируется в reco_cache на 6 часов (ключ учитывает набор активных сервисов),
// чтобы не дёргать внешние API на каждый заход на главную. Пустой список — если
// ни один сервис не подключён.
//
// Перед выдачей список упорядочивается под профиль вкусов
// (Engine.PersonalizeReleases): сверху релизы исполнителей, которых
// пользователь слушает, дизлайкнутые исполнители исключаются. Содержимое при
// этом не подменяется — радар остаётся фактическим списком новинок.
func (a *App) ListNewReleases(limit int) ([]domain.Track, error) {
	if limit <= 0 {
		limit = 40
	}

	// Активные поставщики новинок в стабильном порядке.
	order := []domain.ServiceID{
		domain.ServiceSpotify,
		domain.ServiceYandex,
		domain.ServiceYouTube,
		domain.ServiceSoundCloud,
		domain.ServiceVK,
	}
	var providers []struct {
		id  domain.ServiceID
		svc domain.NewReleaser
	}
	for _, id := range order {
		if nr, ok := a.registry[id].(domain.NewReleaser); ok {
			providers = append(providers, struct {
				id  domain.ServiceID
				svc domain.NewReleaser
			}{id, nr})
		}
	}
	// Источники, умеющие отдавать релизы конкретного артиста — для «засева» радара
	// по вкусу (свежие релизы знакомых и похожих исполнителей, а не редакционный
	// мейнстрим). Реализуют domain.ArtistReleaser: Yandex с токеном, Spotify с ключами.
	var seeders []struct {
		id  domain.ServiceID
		svc domain.ArtistReleaser
	}
	for _, id := range order {
		if ar, ok := a.registry[id].(domain.ArtistReleaser); ok {
			seeders = append(seeders, struct {
				id  domain.ServiceID
				svc domain.ArtistReleaser
			}{id, ar})
		}
	}
	if len(providers) == 0 && len(seeders) == 0 {
		return []domain.Track{}, nil
	}

	// Затравки по вкусу: имена знакомых и похожих артистов. Их свежие релизы идут
	// первыми, поэтому радар наполняется тем, что человек действительно слушает.
	// Пусто, когда вкусы ещё не набраны — тогда работает обычная редакционная лента.
	var seeds []string
	if a.reco != nil && len(seeders) > 0 {
		seeds = a.reco.RadarSeeds(a.ctx, 12)
	}

	// Кэш: ключ включает набор активных сервисов, limit и сигнатуру затравок,
	// чтобы подключение источника или смена вкусов (лайк/дизлайк, новое избранное)
	// инвалидировали запись естественным образом.
	var ids []string
	for _, p := range providers {
		ids = append(ids, string(p.id))
	}
	cacheKey := fmt.Sprintf("newreleases:%s:%s:%d", strings.Join(ids, ","), seedSignature(seeds), limit)
	if a.store != nil {
		if raw, ok, _ := a.store.RecoCacheGet(a.ctx, cacheKey); ok {
			var cached []domain.Track
			if json.Unmarshal([]byte(raw), &cached) == nil {
				// Кэшируется фактическая выдача источников, а под профиль список
				// упорядочивается на каждом запросе: оценка или новое
				// прослушивание должны сказаться сразу, не дожидаясь TTL.
				return a.reco.PersonalizeReleases(a.ctx, cached), nil
			}
		}
	}

	// Опрашиваем редакционные ленты новинок параллельно.
	results := make(map[domain.ServiceID][]domain.Track, len(providers)+len(seeders))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Add(1)
		go func(id domain.ServiceID, svc domain.NewReleaser) {
			defer wg.Done()
			defer a.recoverGoroutine("NewReleases")
			tracks, err := svc.NewReleases(a.ctx, limit)
			if err != nil {
				logging.L().Warn("новинки источника не удалось получить", "service", id, "err", err)
				return
			}
			mu.Lock()
			results[id] = tracks
			mu.Unlock()
		}(p.id, p.svc)
	}
	wg.Wait()

	// Засев по вкусу: свежие релизы затравочных артистов из каждого источника,
	// поддерживающего ArtistReleaser. Ставим их ПЕРЕД редакционными в рамках своего
	// сервиса — так они выходят в начало радара (а PersonalizeReleases ниже ещё и
	// отсортирует по «знакомости»). Ограничиваем параллелизм: артистов до дюжины,
	// на каждого — поиск + альбомы, гнать всё разом по внешним API незачем.
	if len(seeds) > 0 && len(seeders) > 0 {
		const perArtist = 4
		const maxConcurrent = 4
		seeded := make(map[domain.ServiceID][]domain.Track, len(seeders))
		sem := make(chan struct{}, maxConcurrent)
		var sg sync.WaitGroup
		for _, sd := range seeders {
			for _, artist := range seeds {
				sg.Add(1)
				sem <- struct{}{}
				go func(id domain.ServiceID, svc domain.ArtistReleaser, artist string) {
					defer sg.Done()
					defer func() { <-sem }()
					defer a.recoverGoroutine("ArtistNewReleases")
					tracks, err := svc.ArtistNewReleases(a.ctx, artist, perArtist)
					if err != nil {
						logging.L().Debug("засев радара: релизы артиста не удались", "service", id, "artist", artist, "err", err)
						return
					}
					if len(tracks) == 0 {
						return
					}
					mu.Lock()
					seeded[id] = append(seeded[id], tracks...)
					mu.Unlock()
				}(sd.id, sd.svc, artist)
			}
		}
		sg.Wait()
		for id, tracks := range seeded {
			results[id] = append(tracks, results[id]...)
		}
	}

	// Чередуем источники (round-robin), чтобы радар не был завален одним
	// сервисом, дедуп по артист+название.
	seen := map[string]bool{}
	out := make([]domain.Track, 0, limit)
	for idx := 0; len(out) < limit; idx++ {
		progressed := false
		for _, id := range order {
			list := results[id]
			if idx >= len(list) {
				continue
			}
			progressed = true
			t := list[idx]
			key := strings.ToLower(strings.TrimSpace(firstArtist(t) + "|" + t.Title))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
			if len(out) >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}

	if a.store != nil {
		if data, err := json.Marshal(out); err == nil {
			_ = a.store.RecoCacheSet(a.ctx, cacheKey, string(data), 6*time.Hour)
		}
	}
	// В кэш кладём фактическую агрегацию, пользователю отдаём упорядоченную под
	// его вкусы (знакомые исполнители сверху, дизлайкнутые — исключены).
	return a.reco.PersonalizeReleases(a.ctx, out), nil
}

// seedSignature — короткая устойчивая подпись набора затравок для ключа кэша
// радара. Порядок затравок не важен (их всё равно пересортирует
// PersonalizeReleases), поэтому сортируем перед хэшированием. Пустой набор даёт
// стабильный ключ «-», как было до засева по вкусу.
func seedSignature(seeds []string) string {
	if len(seeds) == 0 {
		return "-"
	}
	norm := make([]string, len(seeds))
	for i, s := range seeds {
		norm[i] = strings.ToLower(strings.TrimSpace(s))
	}
	sort.Strings(norm)
	h := fnv.New32a()
	for _, s := range norm {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

// firstArtist возвращает первого артиста трека (или пустую строку).
func firstArtist(t domain.Track) string {
	if len(t.Artists) > 0 {
		return t.Artists[0]
	}
	return ""
}

// --- Настройки ---

// GetSetting возвращает значение настройки по ключу. Если ключа нет — пустая строка.
func (a *App) GetSetting(key string) (string, error) {
	if a.store == nil {
		return "", fmt.Errorf("хранилище недоступно")
	}
	value, _, err := a.store.SettingGet(a.ctx, key)
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetSetting сохраняет значение настройки по ключу.
func (a *App) SetSetting(key string, value string) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.SettingSet(a.ctx, key, value)
}

// --- Yandex Music: токен ---

// SetYandexToken сохраняет OAuth-токен Yandex Music и сразу применяет его к
// адаптеру. С валидным токеном поиск идёт через официальный API, а треки
// разрешаются в прямой mp3-поток. Токен получают через yandex-music-token.
func (a *App) SetYandexToken(token string) error {
	token = strings.TrimSpace(token)
	ya, ok := a.registry[domain.ServiceYandex].(*yandex.Service)
	if !ok {
		return fmt.Errorf("адаптер Yandex Music недоступен")
	}
	ya.SetToken(token)
	a.clearSourceValidation(domain.ServiceYandex)
	if a.store != nil {
		if err := a.store.SettingSet(a.ctx, keyYandexToken, token); err != nil {
			return err
		}
	}
	// Сообщаем фронтенду об успешном подключении, чтобы боковая панель и
	// экран настроек обновили статус без перезагрузки.
	if a.ctx != nil {
		a.platform.Emit("yandex:connected")
		a.emitSourceStatusChanged()
	}
	if token != "" {
		a.pushNotification("success", "Yandex Music подключён", "Токен сохранён — поиск и воспроизведение доступны.")
	}
	return nil
}

// yandexTokenHelperURL — веб-приложение MarshalX для получения токена
// Яндекс Музыки. По документации yandex-music-token это основной путь для
// входа в аккаунт и копирования готового токена в приложение.
const yandexTokenHelperURL = "https://ym-token.marshal.dev/"

// StartYandexLogin открывает в системном браузере страницу получения токена
// Yandex Music. Пользователь авторизуется и копирует выданный токен, после
// чего вставляет его в настройках (см. SetYandexToken).
func (a *App) StartYandexLogin() error {
	if a.ctx == nil {
		return fmt.Errorf("приложение ещё не готово")
	}
	a.platform.OpenURL(yandexTokenHelperURL)
	return nil
}

// YandexTokenConnected сообщает фронтенду, подключён ли Yandex Music по токену
// (без раскрытия самого токена).
func (a *App) YandexTokenConnected() bool {
	ya, ok := a.registry[domain.ServiceYandex].(*yandex.Service)
	return ok && ya.HasToken()
}

// --- Spotify: client_id/secret ---

// SetSpotifyCredentials сохраняет client_id/secret Spotify и сразу применяет их
// к адаптеру. С валидными ключами поиск идёт через официальный Web API и
// доступен «Радар новинок»; без них Spotify работает через веб-поиск. Ключи
// получают в панели разработчика Spotify (developer.spotify.com/dashboard).
// Пустые значения возвращают адаптер к режиму веб-поиска.
func (a *App) SetSpotifyCredentials(clientID string, clientSecret string) error {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	sp, ok := a.registry[domain.ServiceSpotify].(*spotify.Service)
	if !ok {
		return fmt.Errorf("адаптер Spotify недоступен")
	}
	sp.SetCredentials(clientID, clientSecret)
	a.clearSourceValidation(domain.ServiceSpotify)
	if a.store != nil {
		if err := a.store.SettingSet(a.ctx, keySpotifyClientID, clientID); err != nil {
			return err
		}
		if err := a.store.SettingSet(a.ctx, keySpotifyClientSecret, clientSecret); err != nil {
			return err
		}
	}
	a.emitSourceStatusChanged()
	return nil
}

// SpotifyConnected сообщает, заданы ли ключи Spotify (без их раскрытия).
func (a *App) SpotifyConnected() bool {
	sp, ok := a.registry[domain.ServiceSpotify].(*spotify.Service)
	return ok && sp.HasCredentials()
}

// ListAccountConnections returns configuration/account state without exposing
// credentials. OAuth sign-in itself requires a Client ID supplied by the owner
// of the Cryon application in the provider developer console.
func (a *App) ListAccountConnections() ([]AccountConnectionStatus, error) {
	if a.store == nil || a.ctx == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	has := func(key string) bool { v, _, _ := a.store.SettingGet(a.ctx, key); return strings.TrimSpace(v) != "" }
	hasToken := func(id domain.ServiceID) bool {
		t, ok, _ := a.store.TokenGet(a.ctx, id)
		return ok && strings.TrimSpace(t.AccessToken) != ""
	}
	return []AccountConnectionStatus{
		{Service: "yandex", Name: "Yandex Music", OAuthConfigured: true, AccountConnected: a.YandexTokenConnected(), CanImportLibrary: a.YandexTokenConnected(), SetupHint: "Войдите через кнопку Yandex Music и вставьте выданный токен."},
		{Service: "spotify", Name: "Spotify", OAuthConfigured: has(keySpotifyOAuthClientID), AccountConnected: hasToken(domain.ServiceSpotify), CanImportLibrary: hasToken(domain.ServiceSpotify), SetupHint: "Укажите OAuth Client ID приложения типа Desktop app, затем войдите через браузер. Будут импортированы Liked Songs."},
		{Service: "youtube", Name: "YouTube Music", OAuthConfigured: has(keyYouTubeOAuthClientID), AccountConnected: hasToken(domain.ServiceYouTube), CanImportLibrary: false, SetupHint: "Укажите OAuth Client ID типа Desktop app из Google Cloud. Импорт коллекции будет добавлен после авторизации."},
		{Service: "soundcloud", Name: "SoundCloud", OAuthConfigured: has(keySoundCloudOAuthClientID) && has(keySoundCloudOAuthSecret), AccountConnected: hasToken(domain.ServiceSoundCloud), CanImportLibrary: false, SetupHint: "Укажите OAuth Client ID и Client Secret приложения SoundCloud. Для личной коллекции требуется одобренное OAuth-приложение."},
	}, nil
}

// SetOAuthApplicationConfig saves provider application credentials locally.
// Tokens of listener accounts are separate and never returned to the UI.
func (a *App) SetOAuthApplicationConfig(service, clientID, clientSecret string) error {
	if a.store == nil || a.ctx == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	var idKey, secretKey string
	switch domain.ServiceID(service) {
	case domain.ServiceSpotify:
		idKey = keySpotifyOAuthClientID
	case domain.ServiceYouTube:
		idKey = keyYouTubeOAuthClientID
	case domain.ServiceSoundCloud:
		idKey, secretKey = keySoundCloudOAuthClientID, keySoundCloudOAuthSecret
	default:
		return fmt.Errorf("OAuth-настройка для %q не поддерживается", service)
	}
	if err := a.store.SettingSet(a.ctx, idKey, clientID); err != nil {
		return err
	}
	if secretKey != "" {
		if err := a.store.SettingSet(a.ctx, secretKey, clientSecret); err != nil {
			return err
		}
	}
	a.emitSourceStatusChanged()
	return nil
}

// --- SoundCloud: client_id ---

// SetSoundCloudClientID сохраняет client_id SoundCloud и сразу применяет его к
// адаптеру. Ключ необязателен — client_id определяется автоматически со страниц
// SoundCloud, — но заданный вручную надёжнее (автоопределение иногда ломается).
// Пустая строка возвращает адаптер к автоопределению.
func (a *App) SetSoundCloudClientID(clientID string) error {
	clientID = strings.TrimSpace(clientID)
	sc, ok := a.registry[domain.ServiceSoundCloud].(*soundcloud.Service)
	if !ok {
		return fmt.Errorf("адаптер SoundCloud недоступен")
	}
	sc.SetClientID(clientID)
	if a.store != nil {
		if err := a.store.SettingSet(a.ctx, keySoundCloudClientID, clientID); err != nil {
			return err
		}
	}
	a.emitSourceStatusChanged()
	return nil
}

// SoundCloudClientIDSet сообщает, задан ли client_id SoundCloud вручную.
func (a *App) SoundCloudClientIDSet() bool {
	sc, ok := a.registry[domain.ServiceSoundCloud].(*soundcloud.Service)
	return ok && sc.HasClientID()
}

// --- YouTube: API-ключ ---

// SetYouTubeAPIKey сохраняет ключ YouTube Data API и сразу применяет его к
// адаптеру. Ключ необязателен — поиск работает через Bing и парсинг страниц, —
// но с ним выдача стабильнее. Пустая строка возвращает бесключевой режим.
func (a *App) SetYouTubeAPIKey(apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	yt, ok := a.registry[domain.ServiceYouTube].(*youtube.Service)
	if !ok {
		return fmt.Errorf("адаптер YouTube недоступен")
	}
	yt.SetAPIKey(apiKey)
	a.clearSourceValidation(domain.ServiceYouTube)
	if a.store != nil {
		if err := a.store.SettingSet(a.ctx, keyYouTubeAPIKey, apiKey); err != nil {
			return err
		}
	}
	a.emitSourceStatusChanged()
	return nil
}

// YouTubeAPIKeySet сообщает, задан ли ключ YouTube Data API.
func (a *App) YouTubeAPIKeySet() bool {
	yt, ok := a.registry[domain.ServiceYouTube].(*youtube.Service)
	return ok && yt.HasAPIKey()
}

// --- История прослушивания ---

// AddHistory записывает трек в историю прослушивания.
func (a *App) AddHistory(t domain.Track) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.HistoryAdd(a.ctx, t)
}

// HistoryItem — запись истории для фронтенда: трек и время воспроизведения.
type HistoryItem struct {
	Track      domain.Track `json:"track"`
	PlayedAtMs int64        `json:"playedAtMs"`
}

// ListHistory возвращает историю прослушивания (по умолчанию до 200 записей).
func (a *App) ListHistory() ([]HistoryItem, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	rows, err := a.store.HistoryList(a.ctx, 200)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, HistoryItem{Track: r.Track, PlayedAtMs: r.PlayedAtMs})
	}
	return out, nil
}

// ClearHistory очищает историю прослушивания.
func (a *App) ClearHistory() error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.HistoryClear(a.ctx)
}

// ResetAllData очищает ВСЕ пользовательские данные: историю, избранное,
// плейлисты, локальную библиотеку, настройки, токены источников и кэш
// рекомендаций. Возвращает приложение к «чистому аккаунту» — удобно перед
// передачей другому пользователю. Токены Yandex/Last.fm тоже сбрасываются,
// так что источники вернутся в режим веб-поиска до повторного подключения.
//
// NOTE: добавлено koda.
func (a *App) ResetAllData() error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	if err := a.store.ResetAllData(a.ctx); err != nil {
		return err
	}
	// Сбрасываем адаптеры в рантайме, чтобы не ждать перезапуска.
	if ya, ok := a.registry[domain.ServiceYandex].(*yandex.Service); ok {
		ya.SetToken("")
	}
	if sp, ok := a.registry[domain.ServiceSpotify].(*spotify.Service); ok {
		sp.SetCredentials("", "")
	}
	if sc, ok := a.registry[domain.ServiceSoundCloud].(*soundcloud.Service); ok {
		sc.SetClientID("")
	}
	if yt, ok := a.registry[domain.ServiceYouTube].(*youtube.Service); ok {
		yt.SetAPIKey("")
	}
	if a.reco != nil {
		a.reco.SetLastFMKey("")
	}
	if a.local != nil {
		a.local.Replace(nil)
	}
	if a.ctx != nil {
		a.emitSourceStatusChanged()
		a.platform.Emit(favoritesChangedEvent)
		a.platform.Emit("lastfm:changed")
		a.platform.Emit("yandex:connected")
	}
	return nil
}

// --- Пользовательские плейлисты ---

// CreatePlaylist создаёт новый пользовательский плейлист.
func (a *App) CreatePlaylist(title string, description string) (domain.UserPlaylist, error) {
	if a.store == nil {
		return domain.UserPlaylist{}, fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistCreate(a.ctx, title, description)
}

// ListPlaylists возвращает все пользовательские плейлисты.
func (a *App) ListPlaylists() ([]domain.UserPlaylist, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistList(a.ctx)
}

// UpdatePlaylist обновляет название и описание плейлиста.
func (a *App) UpdatePlaylist(id string, title string, description string) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistUpdate(a.ctx, id, title, description)
}

// DeletePlaylist удаляет плейлист вместе с его треками.
func (a *App) DeletePlaylist(id string) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistDelete(a.ctx, id)
}

// ListPlaylistTracks возвращает треки плейлиста.
func (a *App) ListPlaylistTracks(playlistID string) ([]domain.Track, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistTracks(a.ctx, playlistID)
}

// AddTrackToPlaylist добавляет трек в плейлист.
func (a *App) AddTrackToPlaylist(playlistID string, t domain.Track) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistAddTrack(a.ctx, playlistID, t)
}

// RemoveTrackFromPlaylist убирает трек из плейлиста.
func (a *App) RemoveTrackFromPlaylist(playlistID string, service string, trackID string) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistRemoveTrack(a.ctx, playlistID, domain.ServiceID(service), trackID)
}

// ReorderPlaylistTracks задаёт новый порядок треков плейлиста. keys — пары
// service/trackID в желаемом порядке (drag-and-drop на фронтенде).
func (a *App) ReorderPlaylistTracks(playlistID string, keys []store.PlaylistTrackKey) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	return a.store.PlaylistReorder(a.ctx, playlistID, keys)
}

// ListSources возвращает идентификаторы всех подключённых источников.
func (a *App) ListSources() []string {
	out := make([]string, 0, len(a.registry))
	for id := range a.registry {
		out = append(out, string(id))
	}
	sort.Strings(out)
	return out
}

// SourceStatus — реальное состояние источника для боковой панели.
// Level описывает готовность тремя градациями (см. ниже), Connected
// сохранён для обратной совместимости (true для "ok" и "limited").
type SourceStatus struct {
	ID        string `json:"id"`
	Connected bool   `json:"connected"`
	// Level: "ok" — полностью работает (поиск + прямой поток);
	// "limited" — работает частично (только веб-поиск/внешние ссылки, без
	// прямого потока или без API-ключа); "off" — не подключён/заглушка.
	Level string `json:"level"`
	// Error содержит результат последней явной проверки соединения.
	Error string `json:"error,omitempty"`
}

// Градации статуса источника.
const (
	sourceOK      = "ok"
	sourceLimited = "limited"
	sourceOff     = "off"
)

// ListSourceStatus сообщает, насколько каждый источник готов к работе:
//   - "ok" (зелёный): поиск и прямой поток работают. YouTube и SoundCloud
//     играют напрямую без ключей; Yandex — с валидным токеном; локальная
//     музыка — если добавлена хотя бы одна папка.
//   - "limited" (жёлтый): источник работает частично. Spotify отдаёт только
//     внешние ссылки (поиск есть, прямого потока нет); Yandex без токена
//     живёт на веб-поиске; локальная библиотека без папок пуста.
//   - "off" (красный): источник недоступен. VK — заглушка.
func (a *App) ListSourceStatus() []SourceStatus {
	a.statusMu.RLock()
	validationErrors := make(map[domain.ServiceID]string, len(a.validationErrors))
	for id, message := range a.validationErrors {
		validationErrors[id] = message
	}
	runtimeErrors := make(map[domain.ServiceID]string, len(a.runtimeErrors))
	for id, message := range a.runtimeErrors {
		runtimeErrors[id] = message
	}
	a.statusMu.RUnlock()

	levels := a.configLevels()

	out := make([]SourceStatus, 0, len(a.registry))
	for id := range a.registry {
		level := levels[id]
		// Явная проверка ключей приоритетнее live-сбоя: если пользователь
		// только что проверил токен и он невалиден — показываем именно это.
		errMsg := validationErrors[id]
		if errMsg == "" {
			errMsg = runtimeErrors[id]
		}
		if errMsg != "" {
			level = sourceOff
		}
		if level == "" {
			level = sourceOff
		}
		out = append(out, SourceStatus{
			ID:        string(id),
			Connected: level != sourceOff,
			Level:     level,
			Error:     errMsg,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ValidateSource проверяет настроенные учётные данные источника реальным API.
// Бесключевые веб-поиски не проверяются: их доступность меняется у внешних
// поисковиков и не является состоянием сохранённого подключения.
func (a *App) ValidateSource(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	var err error
	switch domain.ServiceID(id) {
	case domain.ServiceYandex:
		if s, ok := a.registry[domain.ServiceYandex].(*yandex.Service); ok {
			err = s.ValidateToken(ctx)
		} else {
			err = fmt.Errorf("адаптер Yandex Music недоступен")
		}
	case domain.ServiceSpotify:
		if s, ok := a.registry[domain.ServiceSpotify].(*spotify.Service); ok {
			err = s.ValidateCredentials(ctx)
		} else {
			err = fmt.Errorf("адаптер Spotify недоступен")
		}
	case domain.ServiceYouTube:
		if s, ok := a.registry[domain.ServiceYouTube].(*youtube.Service); ok {
			if s.HasAPIKey() {
				err = s.ValidateAPIKey(ctx)
			}
		} else {
			err = fmt.Errorf("адаптер YouTube Music недоступен")
		}
	default:
		return fmt.Errorf("проверка для источника %q недоступна", id)
	}
	if err != nil {
		a.statusMu.Lock()
		a.validationErrors[domain.ServiceID(id)] = err.Error()
		a.statusMu.Unlock()
		a.emitSourceStatusChanged()
		a.pushNotification("error", "Не удалось проверить подключение", fmt.Sprintf("%s: %v", id, err))
		return err
	}
	a.statusMu.Lock()
	delete(a.validationErrors, domain.ServiceID(id))
	a.statusMu.Unlock()
	a.emitSourceStatusChanged()
	return nil
}

func (a *App) clearSourceValidation(id domain.ServiceID) {
	a.statusMu.Lock()
	delete(a.validationErrors, id)
	a.statusMu.Unlock()
}

// --- Локальная библиотека ---

// PickMusicFolder открывает системный диалог выбора папки и возвращает
// выбранный путь. Пустая строка — пользователь отменил выбор.
func (a *App) PickMusicFolder() (string, error) {
	return a.platform.PickDirectory("Выберите папку с музыкой")
}

// AddLocalFolder добавляет папку в список источников локальной музыки
// и сразу её сканирует.
func (a *App) AddLocalFolder(path string) ([]domain.Track, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	if err := a.store.LocalFolderAdd(a.ctx, path); err != nil {
		return nil, err
	}
	return a.RescanLocalLibrary()
}

// RemoveLocalFolder убирает папку из источников и пересканирует библиотеку.
func (a *App) RemoveLocalFolder(path string) ([]domain.Track, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	if err := a.store.LocalFolderRemove(a.ctx, path); err != nil {
		return nil, err
	}
	return a.RescanLocalLibrary()
}

// ListLocalFolders возвращает папки-источники локальной музыки.
func (a *App) ListLocalFolders() ([]string, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	return a.store.LocalFolderList(a.ctx)
}

// RescanLocalLibrary пересканирует все папки-источники, обновляет индекс
// и кэш в БД, затем возвращает актуальную библиотеку.
func (a *App) RescanLocalLibrary() ([]domain.Track, error) {
	if a.store == nil {
		return nil, fmt.Errorf("хранилище недоступно")
	}
	folders, err := a.store.LocalFolderList(a.ctx)
	if err != nil {
		return nil, err
	}

	// Одна и та же композиция может попасть в обход дважды, если пользователь
	// добавил и Music, и вложенную в неё папку. Сначала устраняем точные дубли
	// файла по каноническому пути, затем — одинаковые метаданные (title/artist/
	// album). Второй фильтр намеренно консервативен: без совпадения альбома
	// разные версии одной песни остаются в библиотеке.
	all := make([]local.LocalTrack, 0)
	seenPaths := make(map[string]struct{})
	seenMetadata := make(map[string]struct{})
	var scanErrors int
	for _, folder := range folders {
		tracks, serr := local.ScanFolder(a.ctx, folder)
		if serr != nil {
			logging.L().Warn("сканирование папки завершилось с ошибкой", "folder", folder, "err", serr)
			scanErrors++
		}
		for _, track := range tracks {
			canonicalPath := filepath.Clean(track.FilePath)
			if resolved, err := filepath.EvalSymlinks(canonicalPath); err == nil {
				canonicalPath = resolved
			}
			pathKey := strings.ToLower(canonicalPath)
			metadataKey := strings.Join([]string{
				strings.ToLower(strings.TrimSpace(track.Title)),
				strings.ToLower(strings.TrimSpace(strings.Join(track.Artists, ","))),
			}, "\x00")
			if _, duplicate := seenPaths[pathKey]; duplicate {
				continue
			}
			if strings.TrimSpace(track.Title) != "" && len(track.Artists) > 0 {
				if _, duplicate := seenMetadata[metadataKey]; duplicate {
					continue
				}
				seenMetadata[metadataKey] = struct{}{}
			}
			seenPaths[pathKey] = struct{}{}
			all = append(all, track)
		}
	}
	a.local.Replace(all)

	// Обновляем кэш в БД.
	rows := make([]store.LocalTrackRow, 0, len(all))
	for _, t := range all {
		rows = append(rows, store.LocalTrackRow{
			ID:       t.ID,
			Title:    t.Title,
			Artists:  t.Artists,
			Album:    t.Album,
			FilePath: t.FilePath,
		})
	}
	if err := a.store.LocalTrackReplaceAll(a.ctx, rows); err != nil {
		logging.L().Warn("не удалось сохранить кэш локальной библиотеки", "err", err)
	}
	a.emitSourceStatusChanged()

	// Оповещение о завершении сканирования локальной библиотеки (задача 24).
	if scanErrors > 0 {
		a.pushNotification(
			"warning",
			"Сканирование завершено с ошибками",
			fmt.Sprintf("Найдено треков: %d. Часть папок (%d) не удалось прочитать.", len(all), scanErrors),
		)
	} else {
		a.pushNotification(
			"success",
			"Локальная библиотека обновлена",
			fmt.Sprintf("Просканировано треков: %d.", len(all)),
		)
	}

	return a.ListLocalTracks(), nil
}

// ListLocalTracks возвращает все треки локальной библиотеки.
func (a *App) ListLocalTracks() []domain.Track {
	items := a.local.All()
	out := make([]domain.Track, 0, len(items))
	for _, t := range items {
		out = append(out, t.Track)
	}
	return out
}

// GetLocalCover возвращает обложку локального трека как data-URL
// (base64), пригодный для тега <img>. Пусто, если обложки нет.
func (a *App) GetLocalCover(trackID string) (string, error) {
	data, mime, err := a.local.Cover(trackID)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// --- Управление воспроизведением (mpv) ---

// PlayerBackendAvailable сообщает, доступно ли воспроизведение через mpv.
// Если false — фронтенд должен играть поток сам (HTML5 audio).
func (a *App) PlayerBackendAvailable() bool {
	return a.player.Available()
}

// PlayerPlay начинает воспроизведение потока через mpv.
func (a *App) PlayerPlay(trackID string, streamURL string) error {
	return a.player.Play(trackID, streamURL)
}

// PlayerPause ставит на паузу.
func (a *App) PlayerPause() error { return a.player.Pause() }

// PlayerResume снимает с паузы.
func (a *App) PlayerResume() error { return a.player.Resume() }

// PlayerStop останавливает воспроизведение.
func (a *App) PlayerStop() error { return a.player.Stop() }

// PlayerSeek перематывает на позицию в секундах.
func (a *App) PlayerSeek(positionS float64) error { return a.player.Seek(positionS) }

// PlayerSetVolume задаёт громкость 0..100.
func (a *App) PlayerSetVolume(volume int) error { return a.player.SetVolume(volume) }

// PlayerSetEqualizer задаёт усиления полос эквалайзера (дБ) для mpv. Пустой
// срез/все нули — фильтр снимается. Ошибку глушить не нужно: без mpv метод
// просто ничего не делает.
func (a *App) PlayerSetEqualizer(gains []float64) error { return a.player.SetEqualizer(gains) }

// PlayerSetNormalize включает/выключает нормализацию громкости (dynaudnorm) для
// mpv. Без mpv метод ничего не делает (нормализация встроенного плеера — на
// стороне фронтенда через Web Audio).
func (a *App) PlayerSetNormalize(enabled bool) error { return a.player.SetNormalize(enabled) }

// PlayerStatus возвращает текущее состояние плеера.
func (a *App) PlayerStatus() playback.Status { return a.player.Status() }

// GetLyrics возвращает текст песни (по возможности синхронизированный, LRC) из
// lrclib.net. artist/title обязательны; album и durationS уточняют совпадение.
// «Не найдено» — это Found:false, а не ошибка; ошибка только при сетевом сбое.
func (a *App) GetLyrics(artist, title, album string, durationS int) (lyrics.Result, error) {
	if a.lyrics == nil {
		return lyrics.Result{}, nil
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.lyrics.Get(ctx, artist, title, album, durationS)
}

// GetArtist собирает страницу исполнителя: фото, популярные треки и релизы
// (альбомы/синглы). Стратегия мультисервиса: структурированный каталог (альбомы,
// синглы, фото, реальный топ) берётся у первого доступного ArtistBrowser
// (Yandex с токеном), а популярные треки дополняются мультипоиском по рабочим
// источникам. По требованию продукта в топ в первую очередь попадают
// «нецензурные» версии с YouTube/SoundCloud, поэтому найденные поиском треки
// идут раньше каталожных, а каталог лишь дозаполняет пропуски.
func (a *App) GetArtist(name string) (domain.ArtistInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.ArtistInfo{}, fmt.Errorf("пустое имя исполнителя")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	info := domain.ArtistInfo{Name: name}

	// 1. Структурированный каталог: альбомы/синглы/фото/реальный топ.
	//    Приоритет: Yandex (богатый каталог по токену) → YouTube Music (через
	//    yt-dlp, без токенов) → Spotify. Берём первый, отдавший релизы.
	for _, id := range []domain.ServiceID{domain.ServiceYandex, domain.ServiceYouTube, domain.ServiceSpotify} {
		svc, ok := a.registry[id]
		if !ok {
			continue
		}
		browser, ok := svc.(domain.ArtistBrowser)
		if !ok {
			continue
		}
		cat, err := browser.GetArtist(ctx, name)
		if err != nil {
			logging.L().Debug("get-artist: каталог недоступен", "service", id, "err", err)
			continue
		}
		if len(cat.TopTracks) == 0 && len(cat.Albums) == 0 && len(cat.Singles) == 0 {
			continue
		}
		info.Name = firstNonEmptyStr(cat.Name, info.Name)
		info.Service = id
		info.ArtworkURL = cat.ArtworkURL
		info.Albums = cat.Albums
		info.Singles = cat.Singles
		info.AppearsOn = cat.AppearsOn
		info.TopTracks = cat.TopTracks
		break
	}

	// 2. Мультипоиск по рабочим источникам — уточняет топ-треки (uncensored,
	//    с обложками) и даёт запасной путь, если структурированного каталога нет.
	found, _ := a.SearchInSources(name, []string{
		string(domain.ServiceYouTube),
		string(domain.ServiceSoundCloud),
		string(domain.ServiceYandex),
	})
	searchTracks := filterTracksByArtist(found, name)

	// Поисковые треки идут первыми (нецензурные версии в приоритете), каталог
	// лишь дозаполняет уникальные по названию позиции.
	info.TopTracks = mergeTopTracks(searchTracks, info.TopTracks, 20)

	// Фото: если каталог не дал — берём первую доступную обложку трека.
	if info.ArtworkURL == "" {
		for _, t := range info.TopTracks {
			if t.ArtworkURL != "" {
				info.ArtworkURL = t.ArtworkURL
				break
			}
		}
	}

	// 3. Если каталога не было — соберём альбомы/синглы из результатов поиска.
	if len(info.Albums) == 0 && len(info.Singles) == 0 {
		info.Albums, info.Singles = deriveReleasesFromTracks(searchTracks)
	}

	return info, nil
}

// GetAlbumTracks возвращает трек-лист релиза по его ID. Работает для источников
// со структурированным каталогом (ArtistBrowser); для остальных — пусто.
func (a *App) GetAlbumTracks(service string, albumID string) ([]domain.Track, error) {
	svc, ok := a.registry[domain.ServiceID(service)]
	if !ok {
		return []domain.Track{}, nil
	}
	browser, ok := svc.(domain.ArtistBrowser)
	if !ok {
		return []domain.Track{}, nil
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	tracks, err := browser.AlbumTracks(ctx, albumID)
	if err != nil {
		return nil, err
	}
	return tracks, nil
}

// ResolveAlbum добирает полный трек-лист альбома по паре «исполнитель +
// название». Нужен странице альбома, открытой из поиска: там альбом собран из
// отдельных найденных треков и не имеет ID каталога, поэтому показывался лишь
// один-два трека. Спрашиваем источники по порядку борьбы с цензурой
// (SoundCloud → Yandex → YouTube Music) и берём первый непустой трек-лист.
// «Не нашли» — это пустой срез без ошибки: страница честно оставит найденное.
func (a *App) ResolveAlbum(artist, title string) ([]domain.Track, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return []domain.Track{}, nil
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// Порядок продиктован борьбой с цензурой: SoundCloud отдаёт нецензурные
	// версии и опрашивается первым, затем богатый каталог Yandex, затем YouTube.
	for _, id := range []domain.ServiceID{domain.ServiceSoundCloud, domain.ServiceYandex, domain.ServiceYouTube} {
		svc, ok := a.registry[id]
		if !ok {
			continue
		}
		resolver, ok := svc.(domain.AlbumResolver)
		if !ok {
			continue
		}
		tracks, err := resolver.ResolveAlbum(ctx, artist, title)
		if err != nil {
			logging.L().Debug("resolve-album: источник недоступен", "service", id, "err", err)
			continue
		}
		if len(tracks) > 0 {
			return tracks, nil
		}
	}
	return []domain.Track{}, nil
}

// --- Вспомогательные функции страницы исполнителя ---

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// normalizeArtistKey приводит имя/название к сравнимому виду (нижний регистр,
// схлопнутые пробелы) для нестрогого сопоставления исполнителей и дедупа.
func normalizeArtistKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// filterTracksByArtist оставляет треки, у которых имя исполнителя нестрого
// совпадает с искомым (одно содержит другое) — так «GUF» матчит «Guf» и
// «Alex "Guf" Dolmatov», но отсекает совсем посторонних артистов из выдачи.
func filterTracksByArtist(tracks []domain.Track, name string) []domain.Track {
	want := normalizeArtistKey(name)
	if want == "" {
		return nil
	}
	out := make([]domain.Track, 0, len(tracks))
	for _, t := range tracks {
		for _, a := range t.Artists {
			key := normalizeArtistKey(a)
			if key == "" {
				continue
			}
			if key == want || strings.Contains(key, want) || strings.Contains(want, key) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// mergeTopTracks объединяет два списка треков, отбрасывая дубли по
// «исполнитель|название», сохраняя порядок (primary раньше secondary) и обрезая
// до limit. Пустые названия пропускаются.
func mergeTopTracks(primary, secondary []domain.Track, limit int) []domain.Track {
	seen := make(map[string]bool)
	out := make([]domain.Track, 0, limit)
	add := func(list []domain.Track) {
		for _, t := range list {
			if len(out) >= limit {
				return
			}
			title := normalizeArtistKey(t.Title)
			if title == "" {
				continue
			}
			artist := ""
			if len(t.Artists) > 0 {
				artist = normalizeArtistKey(t.Artists[0])
			}
			key := artist + "|" + title
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
		}
	}
	add(primary)
	add(secondary)
	return out
}

// deriveReleasesFromTracks эвристически собирает альбомы и синглы из плоского
// списка треков (запасной путь, когда структурированного каталога нет).
// Треки с одинаковым названием альбома группируются в альбом; одиночные —
// в синглы. Обложка релиза — первая непустая обложка его треков.
func deriveReleasesFromTracks(tracks []domain.Track) (albums []domain.Album, singles []domain.Album) {
	type group struct {
		title    string
		artist   string
		cover    string
		external string
		service  domain.ServiceID
		count    int
	}
	order := make([]string, 0)
	groups := make(map[string]*group)
	for _, t := range tracks {
		album := strings.TrimSpace(t.Album)
		if album == "" {
			continue
		}
		key := normalizeArtistKey(album)
		g, ok := groups[key]
		if !ok {
			artist := ""
			if len(t.Artists) > 0 {
				artist = t.Artists[0]
			}
			g = &group{title: album, artist: artist, service: t.Service}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		if g.cover == "" && t.ArtworkURL != "" {
			g.cover = t.ArtworkURL
		}
		if g.external == "" && t.ExternalURL != "" {
			g.external = t.ExternalURL
		}
	}
	for _, key := range order {
		g := groups[key]
		kind := "album"
		if g.count <= 1 {
			kind = "single"
		} else if g.count <= 4 {
			kind = "ep"
		}
		al := domain.Album{
			Service:     g.service,
			Title:       g.title,
			Artist:      g.artist,
			ArtworkURL:  g.cover,
			Kind:        kind,
			TrackCount:  g.count,
			ExternalURL: g.external,
		}
		if kind == "single" {
			singles = append(singles, al)
		} else {
			albums = append(albums, al)
		}
	}
	return albums, singles
}
