package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/websearch"
)

// Service — адаптер SoundCloud. Использует официальный api-v2:
// client_id берётся из конфига либо обнаруживается автоматически
// на страницах SoundCloud. Если получить client_id не удалось —
// поиск деградирует до веб-поиска через Bing.
type Service struct {
	httpClient *http.Client

	// Все изменяемые поля под mu: заданный client_id может обновляться из
	// настроек в рантайме, а автоматически найденный — из разных горутин.
	mu           sync.Mutex
	clientID     string
	discoveredID string
	discoveredAt time.Time

	// Доступное зеркало CDN обложек выбирается один раз за процесс
	// (см. ensureArtworkHost) — под своей sync.Once, а не под mu.
	artworkOnce sync.Once
	artworkHost string
}

var _ domain.MusicService = (*Service)(nil)

func New(clientID string) *Service {
	return &Service{
		clientID:   strings.TrimSpace(clientID),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Service) ID() domain.ServiceID {
	return domain.ServiceSoundCloud
}

// configuredClientID возвращает заданный вручную client_id под блокировкой.
func (s *Service) configuredClientID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientID
}

// HasClientID сообщает, задан ли client_id вручную. Без него SoundCloud всё
// равно работает: client_id определяется автоматически со страниц сервиса, —
// поэтому статус источника это не роняет, но ручной ключ надёжнее.
func (s *Service) HasClientID() bool {
	return s.configuredClientID() != ""
}

// SetClientID обновляет заданный вручную client_id в рантайме (после ввода в
// настройках) и сбрасывает автоматически найденный, чтобы новый ключ применился
// сразу. Пустая строка возвращает адаптер к автоопределению client_id.
func (s *Service) SetClientID(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientID = strings.TrimSpace(clientID)
	s.discoveredID = ""
	s.discoveredAt = time.Time{}
}

func (s *Service) Search(ctx context.Context, query string) ([]domain.Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.Track{}, nil
	}

	// Пробуем api-v2: client_id либо задан, либо обнаруживается автоматически.
	// Он даёт числовые id и метаданные, а треки становятся воспроизводимыми
	// напрямую через ResolveStream. При неудаче — веб-поиск через Bing.
	if clientID, err := s.ensureClientID(ctx); err == nil {
		tracks, aerr := s.apiSearch(ctx, query, clientID)
		if aerr == nil {
			return tracks, nil
		}
		// client_id мог устареть (SoundCloud их ротирует) — сохранённый вручную
		// или найденный ранее ключ отвергается, и раньше поиск молча падал в
		// пустой Bing-фолбэк. Принудительно переоткрываем свежий client_id со
		// страниц сервиса и повторяем — так SoundCloud сам восстанавливается.
		if fresh, ferr := s.refreshClientID(ctx); ferr == nil && fresh != "" && fresh != clientID {
			if tracks, aerr2 := s.apiSearch(ctx, query, fresh); aerr2 == nil {
				return tracks, nil
			} else {
				aerr = aerr2
			}
		}
		logging.L().Warn("soundcloud: api-поиск не удался, откат к веб-поиску", "err", aerr)
	}
	return s.webSearch(ctx, query)
}

func (s *Service) apiSearch(ctx context.Context, query, clientID string) ([]domain.Track, error) {
	u, _ := url.Parse("https://api-v2.soundcloud.com/search/tracks")
	q := u.Query()
	q.Set("q", query)
	q.Set("client_id", clientID)
	q.Set("limit", "20")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("поиск SoundCloud не удался: %s", strings.TrimSpace(string(body)))
	}

	var sr struct {
		Collection []scFullTrack `json:"collection"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}

	out := make([]domain.Track, 0, len(sr.Collection))
	artworkHost := s.ensureArtworkHost(ctx)
	for _, it := range sr.Collection {
		if it.ID == 0 {
			continue
		}
		// Числовой id + client_id позволяют разрешить прямой поток, поэтому трек
		// воспроизводится напрямую (см. ResolveStream). Общий маппер — в album.go.
		out = append(out, it.toDomain("", artworkHost))
	}
	return out, nil
}

func (s *Service) webSearch(ctx context.Context, query string) ([]domain.Track, error) {
	log := logging.L()
	results, err := websearch.FetchBingSearch(ctx, "soundcloud.com", query, 20)
	if err != nil {
		// Возвращаем ошибку, а не пустой успех: это последний рубеж (api-v2 уже
		// не сработал), и если Bing тоже упал — источник реально недоступен.
		// Прежде здесь был `nil`, и health-check считал SoundCloud «зелёным»,
		// хотя не работало ничего. Fan-out поиска ошибку одного источника
		// переживает — просто исключает его из выдачи, оставляя остальные.
		log.Warn("soundcloud: bing search failed", "err", err)
		return nil, err
	}

	var tracks []domain.Track
	for _, item := range results {
		if !strings.Contains(item.URL, "soundcloud.com/") {
			continue
		}
		// Отсекаем служебные страницы, оставляя только треки.
		if strings.Contains(item.URL, "/search?") ||
			strings.Contains(item.URL, "/charts") ||
			strings.Contains(item.URL, "/discover") {
			continue
		}
		if item.URL == "" || strings.TrimSpace(item.Title) == "" {
			continue
		}
		// Исполнителя берём из slug ссылки (soundcloud.com/<artist>/<track>),
		// иначе на странице артиста все треки схлопывались в фейкового «SoundCloud».
		artist := artistFromPermalink(item.URL)
		if artist == "" {
			artist = "SoundCloud"
		}
		tracks = append(tracks, domain.Track{
			ID:           item.URL,
			Service:      domain.ServiceSoundCloud,
			Title:        cleanSoundCloudTitle(item.Title),
			Artists:      []string{artist},
			ExternalURL:  item.URL,
			PlayableKind: domain.PlayableEmbeddedWeb,
		})
	}

	log.Debug("soundcloud: found via bing", "count", len(tracks))
	return tracks, nil
}

// artistFromPermalink достаёт имя исполнителя из ссылки вида
// soundcloud.com/<artist-slug>/<track-slug>. Возвращает пусто, если ссылка не
// на трек (например, только на профиль или без slug трека).
func artistFromPermalink(rawURL string) string {
	idx := strings.Index(rawURL, "soundcloud.com/")
	if idx == -1 {
		return ""
	}
	path := rawURL[idx+len("soundcloud.com/"):]
	if q := strings.IndexAny(path, "?#"); q != -1 {
		path = path[:q]
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Нужен и артист, и slug трека — иначе это профиль/раздел, а не трек.
	if len(parts) < 2 || parts[0] == "" {
		return ""
	}
	slug := strings.NewReplacer("-", " ", "_", " ").Replace(parts[0])
	return titleCase(strings.TrimSpace(slug))
}

// titleCase делает первую букву каждого слова заглавной (без устаревшего
// strings.Title и без сюрпризов с Unicode-словоразделами).
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// cleanSoundCloudTitle убирает типовые хвосты заголовков из выдачи Bing
// («… | Free Listening on SoundCloud», «… by Artist»), оставляя название трека.
func cleanSoundCloudTitle(title string) string {
	title = strings.TrimSpace(title)
	if i := strings.Index(title, " | "); i != -1 {
		title = strings.TrimSpace(title[:i])
	}
	return title
}
