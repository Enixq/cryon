package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/websearch"
)

// Service — адаптер Spotify. Аудио Spotify защищено DRM и не отдаётся
// прямым потоком без их проприетарного SDK, поэтому адаптер работает как
// источник метаданных: при наличии client_id/secret выполняет реальный
// поиск через официальный Web API (client-credentials), треки открываются
// во внешнем приложении Spotify. Без ключей — веб-поиск через Bing.
type Service struct {
	httpClient *http.Client

	// mu защищает как учётные данные (clientID/clientSecret), так и
	// кэшированный access-токен: всё это может меняться из настроек в рантайме.
	mu           sync.Mutex
	clientID     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
}

var _ domain.MusicService = (*Service)(nil)

func New(clientID, clientSecret string) *Service {
	return &Service{
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Service) ID() domain.ServiceID {
	return domain.ServiceSpotify
}

// credentials возвращает текущие client_id/secret под блокировкой.
func (s *Service) credentials() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clientID, s.clientSecret
}

// HasCredentials сообщает, заданы ли client_id и client_secret. Только с ними
// доступен официальный Web API (поиск с метаданными, новинки); иначе адаптер
// живёт на веб-поиске.
func (s *Service) HasCredentials() bool {
	id, secret := s.credentials()
	return id != "" && secret != ""
}

// ValidateCredentials получает service access token. Это минимальная реальная
// проверка пары client_id/client_secret без выполнения пользовательского поиска.
func (s *Service) ValidateCredentials(ctx context.Context) error {
	if !s.HasCredentials() {
		return fmt.Errorf("spotify: client_id и client_secret не заданы")
	}
	_, err := s.ensureToken(ctx)
	return err
}

// SetCredentials обновляет client_id/secret в рантайме (после ввода в
// настройках) и сбрасывает кэш access-токена, чтобы новые ключи применились
// немедленно. Пустые значения возвращают адаптер к режиму веб-поиска.
func (s *Service) SetCredentials(clientID, clientSecret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientID = strings.TrimSpace(clientID)
	s.clientSecret = strings.TrimSpace(clientSecret)
	s.accessToken = ""
	s.tokenExpiry = time.Time{}
}

func (s *Service) Search(ctx context.Context, query string) ([]domain.Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.Track{}, nil
	}

	if s.HasCredentials() {
		tracks, err := s.apiSearch(ctx, query)
		if err == nil {
			return tracks, nil
		}
		logging.L().Warn("spotify: api-поиск не удался, откат к веб-поиску", "err", err)
	}
	return s.webSearch(ctx, query)
}

func (s *Service) apiSearch(ctx context.Context, query string) ([]domain.Track, error) {
	token, err := s.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse("https://api.spotify.com/v1/search")
	q := u.Query()
	q.Set("q", query)
	q.Set("type", "track")
	q.Set("limit", "20")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify: поиск не удался: %s", strings.TrimSpace(string(body)))
	}

	var sr struct {
		Tracks struct {
			Items []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				DurationMs int    `json:"duration_ms"`
				ExternalU  struct {
					Spotify string `json:"spotify"`
				} `json:"external_urls"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}

	out := make([]domain.Track, 0, len(sr.Tracks.Items))
	for _, it := range sr.Tracks.Items {
		if it.ID == "" {
			continue
		}
		artists := make([]string, 0, len(it.Artists))
		for _, a := range it.Artists {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		artwork := ""
		if len(it.Album.Images) > 0 {
			artwork = it.Album.Images[0].URL
		}
		out = append(out, domain.Track{
			ID:         it.ID,
			Service:    domain.ServiceSpotify,
			Title:      it.Name,
			Artists:    artists,
			Album:      it.Album.Name,
			DurationMs: it.DurationMs,
			ArtworkURL: artwork,
			ExternalURL: firstNonEmpty(
				it.ExternalU.Spotify,
				"https://open.spotify.com/track/"+it.ID,
			),
			// Spotify DRM: прямой поток недоступен, играем во внешнем приложении.
			PlayableKind: domain.PlayableExternal,
		})
	}
	return out, nil
}

// LikedTracks loads a listener's saved tracks using an OAuth user access token.
func LikedTracks(ctx context.Context, accessToken string) ([]domain.Track, bool, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	out := []domain.Track{}
	for offset := 0; offset < 10000; offset += 50 {
		u, _ := url.Parse("https://api.spotify.com/v1/me/tracks")
		q := u.Query()
		q.Set("limit", "50")
		q.Set("offset", fmt.Sprint(offset))
		u.RawQuery = q.Encode()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := client.Do(req)
		if err != nil {
			return nil, false, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 401 {
			return nil, true, fmt.Errorf("spotify: access token истёк")
		}
		if resp.StatusCode/100 != 2 {
			return nil, false, fmt.Errorf("spotify liked tracks: %s", strings.TrimSpace(string(body)))
		}
		var data struct {
			Items []struct {
				Track struct {
					ID         string `json:"id"`
					Name       string `json:"name"`
					DurationMs int    `json:"duration_ms"`
					Artists    []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Album struct {
						Name   string `json:"name"`
						Images []struct {
							URL string `json:"url"`
						} `json:"images"`
					} `json:"album"`
				} `json:"track"`
			} `json:"items"`
			Next *string `json:"next"`
		}
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, false, err
		}
		for _, x := range data.Items {
			t := x.Track
			if t.ID == "" {
				continue
			}
			artists := []string{}
			for _, ar := range t.Artists {
				artists = append(artists, ar.Name)
			}
			art := ""
			if len(t.Album.Images) > 0 {
				art = t.Album.Images[0].URL
			}
			out = append(out, domain.Track{ID: t.ID, Service: domain.ServiceSpotify, Title: t.Name, Artists: artists, Album: t.Album.Name, DurationMs: t.DurationMs, ArtworkURL: art, ExternalURL: "https://open.spotify.com/track/" + t.ID, PlayableKind: domain.PlayableExternal})
		}
		if data.Next == nil {
			break
		}
	}
	return out, false, nil
}

func (s *Service) webSearch(ctx context.Context, query string) ([]domain.Track, error) {
	log := logging.L()
	results, err := websearch.FetchBingSearch(ctx, "open.spotify.com/track", query, 20)
	if err != nil {
		log.Warn("spotify: bing search failed", "err", err)
	}

	var tracks []domain.Track
	for _, item := range results {
		if !strings.Contains(item.URL, "open.spotify.com/track/") {
			continue
		}
		if strings.TrimSpace(item.Title) == "" {
			continue
		}
		tracks = append(tracks, domain.Track{
			ID:           item.URL,
			Service:      domain.ServiceSpotify,
			Title:        item.Title,
			Artists:      []string{"Spotify"},
			ExternalURL:  item.URL,
			PlayableKind: domain.PlayableExternal,
		})
	}
	if len(tracks) == 0 {
		tracks = s.parseFallbackSearchPage(ctx, query)
	}
	log.Debug("spotify: found via web fallback", "count", len(tracks))
	return tracks, nil
}

func (s *Service) parseFallbackSearchPage(ctx context.Context, query string) []domain.Track {
	u := "https://open.spotify.com/search/" + url.QueryEscape(query)
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
	return parseSpotifySearchPage(string(body))
}

func parseSpotifySearchPage(htmlBody string) []domain.Track {
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
				if strings.Contains(link, "open.spotify.com/track/") || strings.HasPrefix(link, "/track/") {
					if title := strings.TrimSpace(textContent(n)); title != "" {
						url := link
						if strings.HasPrefix(url, "/") {
							url = "https://open.spotify.com" + url
						}
						out = append(out, domain.Track{
							ID:           url,
							Service:      domain.ServiceSpotify,
							Title:        title,
							Artists:      []string{"Spotify"},
							ExternalURL:  url,
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

func textContent(n *html.Node) string {
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

var _ domain.NewReleaser = (*Service)(nil)

// NewReleases возвращает свежие релизы из каталога Spotify (/browse/new-releases).
// Требует client_id/secret (client-credentials flow); без них — пусто. Треки
// Spotify защищены DRM, поэтому играются во внешнем приложении (external_only),
// как и в поиске.
func (s *Service) NewReleases(ctx context.Context, limit int) ([]domain.Track, error) {
	if !s.HasCredentials() {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 40
	}
	if limit > 50 {
		limit = 50 // предел Spotify на страницу
	}

	token, err := s.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse("https://api.spotify.com/v1/browse/new-releases")
	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify: новинки не удались: %s", strings.TrimSpace(string(body)))
	}

	var nr struct {
		Albums struct {
			Items []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				ExternalU struct {
					Spotify string `json:"spotify"`
				} `json:"external_urls"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(body, &nr); err != nil {
		return nil, err
	}

	// Представляем альбом-новинку одним треком-обложкой (название альбома).
	// Прямого списка треков browse/new-releases не даёт, а разворачивать каждый
	// альбом — десятки лишних запросов; для радара достаточно карточки альбома.
	out := make([]domain.Track, 0, len(nr.Albums.Items))
	for _, al := range nr.Albums.Items {
		if al.ID == "" || al.Name == "" {
			continue
		}
		artists := make([]string, 0, len(al.Artists))
		for _, a := range al.Artists {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		artwork := ""
		if len(al.Images) > 0 {
			artwork = al.Images[0].URL
		}
		out = append(out, domain.Track{
			ID:         al.ID,
			Service:    domain.ServiceSpotify,
			Title:      al.Name,
			Artists:    artists,
			Album:      al.Name,
			ArtworkURL: artwork,
			ExternalURL: firstNonEmpty(
				al.ExternalU.Spotify,
				"https://open.spotify.com/album/"+al.ID,
			),
			PlayableKind: domain.PlayableExternal,
		})
	}
	return out, nil
}

var _ domain.ArtistReleaser = (*Service)(nil)

// ArtistNewReleases возвращает свежие релизы КОНКРЕТНОГО исполнителя из каталога
// Spotify: находим артиста по имени, берём его альбомы и синглы, свежие сверху.
// Требует client_id/secret; без них — пусто. Как и в NewReleases, каждый альбом
// представлен одной карточкой (треки Spotify защищены DRM и играются внешне).
// Используется для засева «Радара новинок» по вкусу (см. app.ListNewReleases).
func (s *Service) ArtistNewReleases(ctx context.Context, artist string, limit int) ([]domain.Track, error) {
	artist = strings.TrimSpace(artist)
	if artist == "" || !s.HasCredentials() {
		return []domain.Track{}, nil
	}
	if limit <= 0 {
		limit = 6
	}
	if limit > 50 {
		limit = 50
	}

	token, err := s.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	artistID, err := s.findArtistID(ctx, token, artist)
	if err != nil {
		return nil, err
	}
	if artistID == "" {
		return []domain.Track{}, nil
	}

	u, _ := url.Parse("https://api.spotify.com/v1/artists/" + artistID + "/albums")
	q := u.Query()
	q.Set("include_groups", "album,single")
	q.Set("limit", fmt.Sprintf("%d", limit))
	u.RawQuery = q.Encode()

	body, err := s.authGet(ctx, token, u.String())
	if err != nil {
		return nil, err
	}
	var ar struct {
		Items []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			ReleaseDate string `json:"release_date"`
			ExternalU   struct {
				Spotify string `json:"spotify"`
			} `json:"external_urls"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, err
	}

	// Свежие релизы сверху: Spotify не гарантирует порядок по дате.
	sort.SliceStable(ar.Items, func(i, j int) bool {
		return ar.Items[i].ReleaseDate > ar.Items[j].ReleaseDate
	})

	out := make([]domain.Track, 0, len(ar.Items))
	for _, al := range ar.Items {
		if al.ID == "" || al.Name == "" {
			continue
		}
		artists := make([]string, 0, len(al.Artists))
		for _, a := range al.Artists {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		artwork := ""
		if len(al.Images) > 0 {
			artwork = al.Images[0].URL
		}
		out = append(out, domain.Track{
			ID:         al.ID,
			Service:    domain.ServiceSpotify,
			Title:      al.Name,
			Artists:    artists,
			Album:      al.Name,
			ArtworkURL: artwork,
			ExternalURL: firstNonEmpty(
				al.ExternalU.Spotify,
				"https://open.spotify.com/album/"+al.ID,
			),
			PlayableKind: domain.PlayableExternal,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// findArtistID ищет артиста по имени и возвращает ID лучшего совпадения. Пустая
// строка (без ошибки) означает «не найден».
func (s *Service) findArtistID(ctx context.Context, token, name string) (string, error) {
	u, _ := url.Parse("https://api.spotify.com/v1/search")
	q := u.Query()
	q.Set("q", name)
	q.Set("type", "artist")
	q.Set("limit", "5")
	u.RawQuery = q.Encode()

	body, err := s.authGet(ctx, token, u.String())
	if err != nil {
		return "", err
	}
	var sr struct {
		Artists struct {
			Items []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"items"`
		} `json:"artists"`
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return "", err
	}
	for _, a := range sr.Artists.Items {
		if strings.EqualFold(strings.TrimSpace(a.Name), name) {
			return a.ID, nil
		}
	}
	if len(sr.Artists.Items) > 0 {
		return sr.Artists.Items[0].ID, nil
	}
	return "", nil
}

// authGet выполняет авторизованный GET к Web API Spotify и возвращает тело
// успешного ответа.
func (s *Service) authGet(ctx context.Context, token, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify: запрос не удался (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// ensureToken получает и кэширует access-токен по client-credentials flow.
func (s *Service) ensureToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		token := s.accessToken
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	clientID, clientSecret := s.credentials()

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("spotify: не удалось получить токен: %s", strings.TrimSpace(string(body)))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("spotify: пустой access-токен")
	}

	s.mu.Lock()
	s.accessToken = tr.AccessToken
	// Обновляем чуть раньше истечения, чтобы не ловить 401 на границе.
	s.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn-30) * time.Second)
	s.mu.Unlock()
	return tr.AccessToken, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
