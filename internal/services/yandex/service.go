package yandex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/websearch"
)

// apiBase — точка входа неофициального, но стабильного API Yandex Music
// (тот же, что использует официальное приложение и MarshalX/yandex-music-api).
const apiBase = "https://api.music.yandex.net"

// Service — адаптер Yandex Music. При наличии OAuth-токена выполняет
// реальный поиск и разрешает прямой mp3-поток. Без токена деградирует
// до веб-поиска через Bing (треки открываются во внешнем сервисе).
type Service struct {
	mu         sync.RWMutex
	token      string
	httpClient *http.Client
}

var _ domain.MusicService = (*Service)(nil)

func New(token string) *Service {
	return &Service{
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// currentToken возвращает актуальный токен под блокировкой чтения.
func (s *Service) currentToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *Service) ID() domain.ServiceID {
	return domain.ServiceYandex
}

// HasToken сообщает, доступен ли реальный API (а не только веб-поиск).
func (s *Service) HasToken() bool {
	return s.currentToken() != ""
}

// ValidateToken проверяет OAuth-токен коротким запросом к API. Метод не
// меняет состояние адаптера: его можно безопасно использовать для статуса UI.
func (s *Service) ValidateToken(ctx context.Context) error {
	if s.currentToken() == "" {
		return fmt.Errorf("yandex: OAuth-токен не задан")
	}
	// users/account/settings больше не поддерживается web API и возвращает
	// 404 даже для валидного токена. account/status — актуальная точка входа
	// и одновременно содержит UID, нужный для личной медиатеки.
	_, err := s.apiGet(ctx, apiBase+"/account/status")
	return err
}

// SetToken обновляет OAuth-токен в рантайме (например, после ввода в
// настройках). Пустая строка возвращает адаптер к режиму веб-поиска.
func (s *Service) SetToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = strings.TrimSpace(token)
}

func (s *Service) Search(ctx context.Context, query string) ([]domain.Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.Track{}, nil
	}

	if s.currentToken() != "" {
		tracks, err := s.apiSearch(ctx, query)
		if err == nil {
			return tracks, nil
		}
		logging.L().Warn("yandex: api-поиск не удался, откат к веб-поиску", "err", err)
	}
	return s.webSearch(ctx, query)
}

// LikedTracks возвращает любимые треки текущего аккаунта Yandex Music.
// Метод намеренно требует OAuth-токен: без авторизации API не позволяет
// прочитать личную коллекцию. Используется единым импортом в Cryon.
func (s *Service) LikedTracks(ctx context.Context) ([]domain.Track, error) {
	if s.currentToken() == "" {
		return nil, fmt.Errorf("yandex: для импорта избранного требуется OAuth-токен")
	}

	body, err := s.apiGet(ctx, apiBase+"/account/status")
	if err != nil {
		return nil, fmt.Errorf("yandex: не удалось получить аккаунт: %w", err)
	}
	var account struct {
		Result struct {
			Account struct {
				UID json.Number `json:"uid"`
			} `json:"account"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &account); err != nil || account.Result.Account.UID.String() == "" {
		if err != nil {
			return nil, fmt.Errorf("yandex: не удалось разобрать аккаунт: %w", err)
		}
		return nil, fmt.Errorf("yandex: API не вернул идентификатор аккаунта")
	}

	body, err = s.apiGet(ctx, apiBase+"/users/"+url.PathEscape(account.Result.Account.UID.String())+"/likes/tracks")
	if err != nil {
		return nil, fmt.Errorf("yandex: не удалось получить лайки: %w", err)
	}
	var likes struct {
		Result struct {
			Library struct {
				Tracks []struct {
					ID json.Number `json:"id"`
				} `json:"tracks"`
			} `json:"library"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &likes); err != nil {
		return nil, fmt.Errorf("yandex: не удалось разобрать лайки: %w", err)
	}

	ids := make([]string, 0, len(likes.Result.Library.Tracks))
	for _, track := range likes.Result.Library.Tracks {
		if id := track.ID.String(); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []domain.Track{}, nil
	}

	// API принимает пачки id; ограничиваем размер, чтобы URL оставался разумным.
	out := make([]domain.Track, 0, len(ids))
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		u := apiBase + "/tracks?track-ids=" + url.QueryEscape(strings.Join(ids[start:end], ","))
		tracksBody, err := s.apiGet(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("yandex: не удалось получить данные любимых треков: %w", err)
		}
		var tracks struct {
			Result []yaTrack `json:"result"`
		}
		if err := json.Unmarshal(tracksBody, &tracks); err != nil {
			return nil, fmt.Errorf("yandex: не удалось разобрать любимые треки: %w", err)
		}
		for _, track := range tracks.Result {
			out = append(out, track.toDomain())
		}
	}
	return out, nil
}

func (s *Service) apiSearch(ctx context.Context, query string) ([]domain.Track, error) {
	u, _ := url.Parse(apiBase + "/search")
	q := u.Query()
	q.Set("text", query)
	q.Set("type", "track")
	q.Set("page", "0")
	q.Set("nocorrect", "false")
	u.RawQuery = q.Encode()

	body, err := s.apiGet(ctx, u.String())
	if err != nil {
		return nil, err
	}

	var sr struct {
		Result struct {
			Tracks struct {
				Results []yaTrack `json:"results"`
			} `json:"tracks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("yandex: не удалось разобрать ответ поиска: %w", err)
	}

	out := make([]domain.Track, 0, len(sr.Result.Tracks.Results))
	for _, t := range sr.Result.Tracks.Results {
		out = append(out, t.toDomain())
	}
	return out, nil
}

func (s *Service) webSearch(ctx context.Context, query string) ([]domain.Track, error) {
	log := logging.L()
	results, err := websearch.FetchBingSearch(ctx, "music.yandex.ru/album", query, 20)
	if err != nil {
		log.Warn("yandex: bing search failed", "err", err)
	}

	var tracks []domain.Track
	for _, item := range results {
		if !strings.Contains(item.URL, "music.yandex.ru/") {
			continue
		}
		if strings.TrimSpace(item.Title) == "" {
			continue
		}
		tracks = append(tracks, domain.Track{
			ID:           item.URL,
			Service:      domain.ServiceYandex,
			Title:        item.Title,
			Artists:      []string{"Yandex Music"},
			ExternalURL:  item.URL,
			PlayableKind: domain.PlayableExternal,
		})
	}
	if len(tracks) == 0 {
		tracks = s.parseFallbackSearchPage(ctx, query)
	}
	log.Debug("yandex: found via web fallback", "count", len(tracks))
	return tracks, nil
}

func (s *Service) parseFallbackSearchPage(ctx context.Context, query string) []domain.Track {
	u := "https://music.yandex.ru/search?text=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	return parseYandexSearchPage(string(body))
}

func parseYandexSearchPage(htmlBody string) []domain.Track {
	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return nil
	}

	out := make([]domain.Track, 0)
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key != "href" {
					continue
				}
				link := strings.TrimSpace(attr.Val)
				if strings.Contains(link, "music.yandex.ru/track/") ||
					strings.Contains(link, "music.yandex.ru/album/") ||
					strings.HasPrefix(link, "/album/") ||
					strings.HasPrefix(link, "/track/") {
					if title := strings.TrimSpace(extractNodeText(n)); title != "" {
						if strings.HasPrefix(link, "/") {
							link = "https://music.yandex.ru" + link
						}
						out = append(out, domain.Track{
							ID:           link,
							Service:      domain.ServiceYandex,
							Title:        title,
							Artists:      []string{"Yandex Music"},
							ExternalURL:  link,
							PlayableKind: domain.PlayableExternal,
						})
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	return out
}

func extractNodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// yaTrack — трек из ответа Yandex Music API (нужные поля).
type yaTrack struct {
	ID         json.Number `json:"id"`
	Title      string      `json:"title"`
	DurationMs int         `json:"durationMs"`
	CoverURI   string      `json:"coverUri"`
	Available  bool        `json:"available"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Albums []struct {
		ID    json.Number `json:"id"`
		Title string      `json:"title"`
	} `json:"albums"`
}

func (t yaTrack) toDomain() domain.Track {
	artists := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if a.Name != "" {
			artists = append(artists, a.Name)
		}
	}

	album := ""
	if len(t.Albums) > 0 {
		album = t.Albums[0].Title
	}

	// ID трека для стрима кодируем как "trackID:albumID" — download-info
	// требует оба (у трека может быть несколько альбомов).
	rawID := t.ID.String()
	if len(t.Albums) > 0 {
		rawID = t.ID.String() + ":" + t.Albums[0].ID.String()
	}

	kind := domain.PlayableStream
	if !t.Available {
		kind = domain.PlayableExternal
	}

	return domain.Track{
		ID:           rawID,
		Service:      domain.ServiceYandex,
		Title:        t.Title,
		Artists:      artists,
		Album:        album,
		DurationMs:   t.DurationMs,
		ArtworkURL:   normalizeCover(t.CoverURI),
		ExternalURL:  externalURL(t.ID.String(), t.Albums),
		PlayableKind: kind,
	}
}

// normalizeCover разворачивает шаблон обложки Yandex (содержит "%%")
// в конкретный URL размером 400x400.
func normalizeCover(coverURI string) string {
	if coverURI == "" {
		return ""
	}
	return "https://" + strings.Replace(coverURI, "%%", "400x400", 1)
}

func externalURL(trackID string, albums []struct {
	ID    json.Number `json:"id"`
	Title string      `json:"title"`
}) string {
	if len(albums) > 0 {
		return fmt.Sprintf("https://music.yandex.ru/album/%s/track/%s", albums[0].ID.String(), trackID)
	}
	return "https://music.yandex.ru/track/" + trackID
}

var _ domain.NewReleaser = (*Service)(nil)

// NewReleases возвращает свежие релизы Yandex Music. Доступно только с
// OAuth-токеном: сначала берём список ID новых альбомов (/landing3/new-releases),
// затем догружаем треки ограниченного числа альбомов. Без токена — пусто
// (веб-поиск новинки не отдаёт).
func (s *Service) NewReleases(ctx context.Context, limit int) ([]domain.Track, error) {
	if s.currentToken() == "" {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 40
	}

	body, err := s.apiGet(ctx, apiBase+"/landing3/new-releases")
	if err != nil {
		return nil, err
	}
	var lr struct {
		Result struct {
			NewReleases []json.Number `json:"newReleases"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, fmt.Errorf("yandex: не удалось разобрать новинки: %w", err)
	}

	// Ограничиваем число альбомов, чтобы не делать десятки запросов: с запасом,
	// т.к. в альбоме несколько треков, а нам нужно ~limit штук.
	albumIDs := lr.Result.NewReleases
	maxAlbums := limit/2 + 4
	if len(albumIDs) > maxAlbums {
		albumIDs = albumIDs[:maxAlbums]
	}

	out := make([]domain.Track, 0, limit)
	for _, id := range albumIDs {
		if len(out) >= limit {
			break
		}
		tracks, err := s.albumTracks(ctx, id.String())
		if err != nil {
			logging.L().Debug("yandex: не удалось загрузить альбом-новинку", "album", id.String(), "err", err)
			continue
		}
		// Один-два трека с альбома — чтобы радар был разнообразным по артистам.
		for i, t := range tracks {
			if i >= 2 || len(out) >= limit {
				break
			}
			out = append(out, t)
		}
	}
	return out, nil
}

var _ domain.ArtistReleaser = (*Service)(nil)

// ArtistNewReleases возвращает свежие релизы КОНКРЕТНОГО исполнителя. Доступно
// только с OAuth-токеном: находим артиста по имени, берём его альбомы,
// отсортированные по году (свежие сверху), и догружаем треки нескольких верхних
// альбомов. Без токена или если артист не найден — пусто. Используется для
// засева «Радара новинок» по вкусу (см. app.ListNewReleases).
func (s *Service) ArtistNewReleases(ctx context.Context, artist string, limit int) ([]domain.Track, error) {
	artist = strings.TrimSpace(artist)
	if artist == "" || s.currentToken() == "" {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 6
	}

	artistID, err := s.findArtistID(ctx, artist)
	if err != nil {
		return nil, err
	}
	if artistID == "" {
		return []domain.Track{}, nil
	}

	albumIDs, err := s.artistAlbums(ctx, artistID)
	if err != nil {
		return nil, err
	}

	out := make([]domain.Track, 0, limit)
	// Несколько верхних (самых свежих) альбомов; по паре треков с каждого, чтобы
	// засев был разнообразным и не делал десятки запросов.
	maxAlbums := limit/2 + 2
	for i, id := range albumIDs {
		if i >= maxAlbums || len(out) >= limit {
			break
		}
		tracks, err := s.albumTracks(ctx, id)
		if err != nil {
			logging.L().Debug("yandex: не удалось загрузить альбом артиста", "album", id, "err", err)
			continue
		}
		for j, t := range tracks {
			if j >= 2 || len(out) >= limit {
				break
			}
			out = append(out, t)
		}
	}
	return out, nil
}

var _ domain.ArtistBrowser = (*Service)(nil)
var _ domain.AlbumResolver = (*Service)(nil)

// GetArtist собирает каталог исполнителя: фото, популярные треки и релизы
// (альбомы/синглы). Использует один запрос /artists/{id}/brief-info — тот же,
// что и официальное приложение, — поэтому за один заход приходят и фото, и
// топ-треки, и список релизов с обложками и годами. Требует OAuth-токен; без
// него или если исполнитель не найден — пустой ArtistInfo без ошибки, чтобы
// агрегатор мог собрать страницу из мультипоиска.
func (s *Service) GetArtist(ctx context.Context, name string) (domain.ArtistInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" || s.currentToken() == "" {
		return domain.ArtistInfo{}, nil
	}

	artistID, err := s.findArtistID(ctx, name)
	if err != nil {
		return domain.ArtistInfo{}, err
	}
	if artistID == "" {
		return domain.ArtistInfo{}, nil
	}

	body, err := s.apiGet(ctx, apiBase+"/artists/"+artistID+"/brief-info")
	if err != nil {
		return domain.ArtistInfo{}, err
	}

	var br struct {
		Result struct {
			Artist struct {
				Name    string `json:"name"`
				Cover   struct {
					URI string `json:"uri"`
				} `json:"cover"`
				OgImage string `json:"ogImage"`
			} `json:"artist"`
			PopularTracks []yaTrack `json:"popularTracks"`
			Albums        []yaAlbum `json:"albums"`
			AlsoAlbums    []yaAlbum `json:"alsoAlbums"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &br); err != nil {
		return domain.ArtistInfo{}, fmt.Errorf("yandex: не удалось разобрать каталог артиста: %w", err)
	}

	info := domain.ArtistInfo{
		Name:    firstNonEmpty(br.Result.Artist.Name, name),
		Service: domain.ServiceYandex,
	}
	// Фото: cover.uri надёжнее ogImage; оба — шаблоны с "%%".
	if uri := br.Result.Artist.Cover.URI; uri != "" {
		info.ArtworkURL = normalizeCover(uri)
	} else if og := br.Result.Artist.OgImage; og != "" {
		info.ArtworkURL = normalizeCover(og)
	}

	for _, t := range br.Result.PopularTracks {
		info.TopTracks = append(info.TopTracks, t.toDomain())
	}

	// Собственная дискография. brief-info отдаёт лишь верхушку списка релизов,
	// поэтому полный список тянем отдельно через /direct-albums (все страницы).
	// Если добор не удался — деградируем к тому, что дал brief-info.
	ownAlbums := br.Result.Albums
	if full, err := s.artistDirectAlbums(ctx, artistID); err == nil && len(full) > 0 {
		ownAlbums = full
	}
	for _, al := range ownAlbums {
		album := al.toDomain()
		if album.Kind == "single" || album.Kind == "ep" {
			info.Singles = append(info.Singles, album)
		} else {
			info.Albums = append(info.Albums, album)
		}
	}
	// AlsoAlbums — релизы, где артист лишь участвует (совместки, сборники
	// других исполнителей). В свою дискографию их мешать нельзя, поэтому
	// выносим в отдельный блок «Встречается в».
	for _, al := range br.Result.AlsoAlbums {
		info.AppearsOn = append(info.AppearsOn, al.toDomain())
	}
	return info, nil
}

// AlbumTracks — публичная обёртка над albumTracks для интерфейса ArtistBrowser.
func (s *Service) AlbumTracks(ctx context.Context, albumID string) ([]domain.Track, error) {
	if s.currentToken() == "" {
		return []domain.Track{}, nil
	}
	return s.albumTracks(ctx, albumID)
}

// ResolveAlbum ищет релиз по «исполнитель + название» и возвращает его полный
// трек-лист. Использует поиск type=album, выбирает лучшее совпадение по
// названию (и, если возможно, по исполнителю), затем тянет треки альбома. Так
// альбом, открытый со страницы поиска (без ID каталога), дополняется до
// настоящего релиза со всеми треками. Реализует domain.AlbumResolver.
func (s *Service) ResolveAlbum(ctx context.Context, artist, title string) ([]domain.Track, error) {
	title = strings.TrimSpace(title)
	if title == "" || s.currentToken() == "" {
		return []domain.Track{}, nil
	}
	albumID, err := s.findAlbumID(ctx, artist, title)
	if err != nil {
		return nil, err
	}
	if albumID == "" {
		return []domain.Track{}, nil
	}
	return s.albumTracks(ctx, albumID)
}

// findAlbumID ищет альбом по названию (при наличии — с уточнением исполнителя)
// и возвращает ID лучшего совпадения. Пустая строка без ошибки — «не найден».
func (s *Service) findAlbumID(ctx context.Context, artist, title string) (string, error) {
	query := strings.TrimSpace(title)
	if a := strings.TrimSpace(artist); a != "" {
		query = a + " " + title
	}
	u, _ := url.Parse(apiBase + "/search")
	q := u.Query()
	q.Set("text", query)
	q.Set("type", "album")
	q.Set("page", "0")
	q.Set("nocorrect", "false")
	u.RawQuery = q.Encode()

	body, err := s.apiGet(ctx, u.String())
	if err != nil {
		return "", err
	}
	var sr struct {
		Result struct {
			Albums struct {
				Results []yaAlbum `json:"results"`
			} `json:"albums"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return "", fmt.Errorf("yandex: не удалось разобрать поиск альбома: %w", err)
	}

	results := sr.Result.Albums.Results
	wantTitle := normalizeMatchKey(title)
	wantArtist := normalizeMatchKey(artist)
	// Сначала ищем точное совпадение названия (и исполнителя, если задан) —
	// у распространённых названий сверху может оказаться чужой релиз.
	for _, al := range results {
		if normalizeMatchKey(al.Title) != wantTitle {
			continue
		}
		if wantArtist == "" || albumHasArtist(al, wantArtist) {
			return al.ID.String(), nil
		}
	}
	// Иначе — первое совпадение по названию без учёта исполнителя.
	for _, al := range results {
		if normalizeMatchKey(al.Title) == wantTitle {
			return al.ID.String(), nil
		}
	}
	if len(results) > 0 {
		return results[0].ID.String(), nil
	}
	return "", nil
}

// normalizeMatchKey приводит строку к сравнимому виду (нижний регистр,
// схлопнутые пробелы) для нестрогого сопоставления названий/исполнителей.
func normalizeMatchKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// albumHasArtist сообщает, есть ли среди исполнителей альбома совпадающий с want
// (нестрого — одно имя содержит другое).
func albumHasArtist(al yaAlbum, want string) bool {
	for _, a := range al.Artists {
		key := normalizeMatchKey(a.Name)
		if key == "" {
			continue
		}
		if key == want || strings.Contains(key, want) || strings.Contains(want, key) {
			return true
		}
	}
	return false
}

// yaAlbum — релиз из ответа Yandex Music API (нужные поля).
type yaAlbum struct {
	ID         json.Number `json:"id"`
	Title      string      `json:"title"`
	Year       int         `json:"year"`
	CoverURI   string      `json:"coverUri"`
	Type       string      `json:"type"`
	MetaType   string      `json:"metaType"`
	TrackCount int         `json:"trackCount"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
}

func (a yaAlbum) toDomain() domain.Album {
	// Yandex помечает синглы type/metaType == "single"; EP отдельного типа не
	// имеет — трактуем малый релиз (<= 4 треков) без пометки альбома как EP.
	kind := "album"
	switch {
	case strings.EqualFold(a.Type, "single") || strings.EqualFold(a.MetaType, "single"):
		kind = "single"
	case a.TrackCount > 0 && a.TrackCount <= 4:
		kind = "ep"
	}
	artist := ""
	if len(a.Artists) > 0 {
		artist = a.Artists[0].Name
	}
	return domain.Album{
		ID:          a.ID.String(),
		Service:     domain.ServiceYandex,
		Title:       a.Title,
		Artist:      artist,
		Year:        a.Year,
		ArtworkURL:  normalizeCover(a.CoverURI),
		Kind:        kind,
		TrackCount:  a.TrackCount,
		ExternalURL: "https://music.yandex.ru/album/" + a.ID.String(),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// findArtistID ищет артиста по имени и возвращает ID лучшего совпадения. Пустая
// строка (без ошибки) означает «не найден».
func (s *Service) findArtistID(ctx context.Context, name string) (string, error) {
	u, _ := url.Parse(apiBase + "/search")
	q := u.Query()
	q.Set("text", name)
	q.Set("type", "artist")
	q.Set("page", "0")
	q.Set("nocorrect", "false")
	u.RawQuery = q.Encode()

	body, err := s.apiGet(ctx, u.String())
	if err != nil {
		return "", err
	}
	var sr struct {
		Result struct {
			Artists struct {
				Results []struct {
					ID   json.Number `json:"id"`
					Name string      `json:"name"`
				} `json:"results"`
			} `json:"artists"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return "", fmt.Errorf("yandex: не удалось разобрать поиск артиста: %w", err)
	}

	results := sr.Result.Artists.Results
	// Точное совпадение имени важнее позиции в выдаче: у распространённых имён
	// сверху может оказаться тёзка. Иначе берём первого — поиск ранжирует по
	// релевантности.
	for _, a := range results {
		if strings.EqualFold(strings.TrimSpace(a.Name), name) {
			return a.ID.String(), nil
		}
	}
	if len(results) > 0 {
		return results[0].ID.String(), nil
	}
	return "", nil
}

// artistDirectAlbums возвращает ПОЛНУЮ дискографию исполнителя (свои релизы),
// самые свежие сверху, с полными метаданными (название, год, обложка, вид).
// brief-info отдаёт лишь верхушку списка (обычно ~7 релизов), из-за чего у
// плодовитых артистов пропадала бо́льшая часть альбомов; здесь листаем
// /direct-albums постранично, пока сервер отдаёт релизы.
func (s *Service) artistDirectAlbums(ctx context.Context, artistID string) ([]yaAlbum, error) {
	const pageSize = 100
	const maxPages = 20 // предохранитель от бесконечного листания
	seen := make(map[string]bool)
	out := make([]yaAlbum, 0, pageSize)
	for page := 0; page < maxPages; page++ {
		u, _ := url.Parse(apiBase + "/artists/" + artistID + "/direct-albums")
		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		q.Set("page-size", strconv.Itoa(pageSize))
		q.Set("sort-by", "year")
		q.Set("sort-order", "desc")
		u.RawQuery = q.Encode()

		body, err := s.apiGet(ctx, u.String())
		if err != nil {
			return out, err
		}
		var ar struct {
			Result struct {
				Albums []yaAlbum `json:"albums"`
			} `json:"result"`
		}
		if err := json.Unmarshal(body, &ar); err != nil {
			return out, fmt.Errorf("yandex: не удалось разобрать альбомы артиста: %w", err)
		}
		added := 0
		for _, al := range ar.Result.Albums {
			id := al.ID.String()
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, al)
			added++
		}
		// Останавливаемся, когда страница пуста или не принесла новых релизов.
		// Не полагаемся на «меньше page-size»: сервер вправе урезать размер
		// страницы, и по такому признаку часть дискографии терялась бы.
		if len(ar.Result.Albums) == 0 || added == 0 {
			break
		}
	}
	return out, nil
}

// artistAlbums возвращает ID альбомов исполнителя, самые свежие сверху.
func (s *Service) artistAlbums(ctx context.Context, artistID string) ([]string, error) {
	u, _ := url.Parse(apiBase + "/artists/" + artistID + "/direct-albums")
	q := u.Query()
	q.Set("page", "0")
	q.Set("page-size", "10")
	q.Set("sort-by", "year")
	q.Set("sort-order", "desc")
	u.RawQuery = q.Encode()

	body, err := s.apiGet(ctx, u.String())
	if err != nil {
		return nil, err
	}
	var ar struct {
		Result struct {
			Albums []struct {
				ID json.Number `json:"id"`
			} `json:"albums"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("yandex: не удалось разобрать альбомы артиста: %w", err)
	}
	ids := make([]string, 0, len(ar.Result.Albums))
	for _, al := range ar.Result.Albums {
		ids = append(ids, al.ID.String())
	}
	return ids, nil
}

// albumTracks загружает треки альбома по ID (первый «том»).
func (s *Service) albumTracks(ctx context.Context, albumID string) ([]domain.Track, error) {
	body, err := s.apiGet(ctx, apiBase+"/albums/"+albumID+"/with-tracks")
	if err != nil {
		return nil, err
	}
	var ar struct {
		Result struct {
			Volumes [][]yaTrack `json:"volumes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("yandex: не удалось разобрать альбом: %w", err)
	}
	out := make([]domain.Track, 0)
	for _, vol := range ar.Result.Volumes {
		for _, t := range vol {
			out = append(out, t.toDomain())
		}
	}
	return out, nil
}

// apiGet выполняет авторизованный GET к API Yandex Music.
func (s *Service) apiGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+s.currentToken())
	req.Header.Set("User-Agent", "Yandex-Music-API")
	req.Header.Set("X-Yandex-Music-Client", "WindowsPhone/3.20")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("yandex: статус %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
