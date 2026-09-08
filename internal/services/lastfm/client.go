// Package lastfm — тонкий клиент к публичному REST API Last.fm.
//
// Используется движком рекомендаций (internal/recommendations): Last.fm знает
// артистов во всех источниках, поэтому «похожие артисты/треки» работают поверх
// мульти-сервисной библиотеки без обучения собственной модели.
//
// Реализовано прямыми HTTP-вызовами (как yandex/spotify-адаптеры), без внешней
// зависимости-обёртки: API простой, а лишний модуль тянуть незачем.
package lastfm

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

	"Cryon2/internal/logging"
)

// apiBase — точка входа публичного API Last.fm (формат JSON).
const apiBase = "https://ws.audioscrobbler.com/2.0/"

// Client — клиент Last.fm. Без API-ключа считается недоступным: вызывающая
// сторона (движок рекомендаций) переходит в оффлайн-режим. Ключ можно задать
// в рантайме (SetAPIKey) — движок рекомендаций читается конкурентно, поэтому
// доступ к ключу защищён мьютексом.
type Client struct {
	mu         sync.RWMutex
	apiKey     string
	httpClient *http.Client
}

// New создаёт клиент с заданным API-ключом (может быть пустым).
func New(apiKey string) *Client {
	return &Client{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 12 * time.Second},
	}
}

// SetAPIKey задаёт ключ в рантайме (из настроек). Пустая строка отключает
// онлайн-движок.
func (c *Client) SetAPIKey(apiKey string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.apiKey = strings.TrimSpace(apiKey)
	c.mu.Unlock()
}

// key возвращает текущий ключ под блокировкой чтения.
func (c *Client) key() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiKey
}

// Available сообщает, задан ли API-ключ (иначе запросы делать бессмысленно).
func (c *Client) Available() bool {
	return c != nil && c.key() != ""
}

// Artist — похожий артист из ответа Last.fm.
type Artist struct {
	Name  string
	Match float64 // близость 0..1 (Last.fm отдаёт строкой)
}

// Track — похожий трек из ответа Last.fm.
type Track struct {
	Name   string
	Artist string
	Match  float64
}

// Tag — жанровый/настроенческий тег артиста из ответа Last.fm. Count — «сила»
// применения тега (0..100 по данным Last.fm): насколько сообщество считает тег
// характерным для артиста. Служит компонентой вектора тегов в контентной
// похожести (см. internal/recommendations/content.go).
type Tag struct {
	Name  string
	Count int
}

// SimilarArtists возвращает похожих артистов, отсортированных по близости.
// Пустой результат без ошибки — если ключа нет или артист не найден.
func (c *Client) SimilarArtists(ctx context.Context, artist string, limit int) ([]Artist, error) {
	artist = strings.TrimSpace(artist)
	if !c.Available() || artist == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	var payload struct {
		SimilarArtists struct {
			Artist []struct {
				Name  string `json:"name"`
				Match string `json:"match"`
			} `json:"artist"`
		} `json:"similarartists"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := c.get(ctx, map[string]string{
		"method": "artist.getsimilar",
		"artist": artist,
		"limit":  fmt.Sprintf("%d", limit),
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm: %s (код %d)", payload.Message, payload.Error)
	}

	out := make([]Artist, 0, len(payload.SimilarArtists.Artist))
	for _, a := range payload.SimilarArtists.Artist {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		out = append(out, Artist{Name: name, Match: parseMatch(a.Match)})
	}
	return out, nil
}

// SimilarTracks возвращает похожие треки по паре артист+название.
func (c *Client) SimilarTracks(ctx context.Context, artist, track string, limit int) ([]Track, error) {
	artist = strings.TrimSpace(artist)
	track = strings.TrimSpace(track)
	if !c.Available() || artist == "" || track == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	var payload struct {
		SimilarTracks struct {
			Track []struct {
				Name   string `json:"name"`
				Match  string `json:"match"`
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"track"`
		} `json:"similartracks"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := c.get(ctx, map[string]string{
		"method": "track.getsimilar",
		"artist": artist,
		"track":  track,
		"limit":  fmt.Sprintf("%d", limit),
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm: %s (код %d)", payload.Message, payload.Error)
	}

	out := make([]Track, 0, len(payload.SimilarTracks.Track))
	for _, t := range payload.SimilarTracks.Track {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		out = append(out, Track{
			Name:   name,
			Artist: strings.TrimSpace(t.Artist.Name),
			Match:  parseMatch(t.Match),
		})
	}
	return out, nil
}

// TopArtistTracks возвращает популярные треки артиста — полезно как источник
// конкретных названий для последующего поиска в музыкальных сервисах.
func (c *Client) TopArtistTracks(ctx context.Context, artist string, limit int) ([]Track, error) {
	artist = strings.TrimSpace(artist)
	if !c.Available() || artist == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	var payload struct {
		TopTracks struct {
			Track []struct {
				Name   string `json:"name"`
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"track"`
		} `json:"toptracks"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := c.get(ctx, map[string]string{
		"method": "artist.gettoptracks",
		"artist": artist,
		"limit":  fmt.Sprintf("%d", limit),
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm: %s (код %d)", payload.Message, payload.Error)
	}

	out := make([]Track, 0, len(payload.TopTracks.Track))
	for _, t := range payload.TopTracks.Track {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		artistName := strings.TrimSpace(t.Artist.Name)
		if artistName == "" {
			artistName = artist
		}
		out = append(out, Track{Name: name, Artist: artistName})
	}
	return out, nil
}

// ArtistTags возвращает топовые теги артиста (жанры/настроения) с их «силой»
// (count 0..100). Используется контентной похожестью движка рекомендаций:
// артист представляется вектором тегов, а близость к профилю вкуса считается
// косинусом. Пустой результат без ошибки — если ключа нет или тегов нет.
func (c *Client) ArtistTags(ctx context.Context, artist string, limit int) ([]Tag, error) {
	artist = strings.TrimSpace(artist)
	if !c.Available() || artist == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	var payload struct {
		TopTags struct {
			Tag []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"tag"`
		} `json:"toptags"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := c.get(ctx, map[string]string{
		"method": "artist.gettoptags",
		"artist": artist,
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm: %s (код %d)", payload.Message, payload.Error)
	}

	out := make([]Tag, 0, len(payload.TopTags.Tag))
	for _, t := range payload.TopTags.Tag {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		out = append(out, Tag{Name: name, Count: t.Count})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// get выполняет GET к API Last.fm и декодирует JSON в dst.
func (c *Client) get(ctx context.Context, params map[string]string, dst any) error {
	q := url.Values{}
	q.Set("api_key", c.key())
	q.Set("format", "json")
	for k, v := range params {
		q.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Cryon2/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("lastfm: статус %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		logging.L().Debug("lastfm: не удалось разобрать ответ", "err", err)
		return err
	}
	return nil
}

// parseMatch разбирает строковое значение близости Last.fm в float 0..1.
func parseMatch(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return 0
	}
	return v
}
