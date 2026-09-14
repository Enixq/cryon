// Package deezer — тонкий клиент к публичному REST API Deezer.
//
// Используется движком рекомендаций (internal/recommendations) как БЕСКЛЮЧЕВОЙ
// граф похожести: Deezer знает артистов и их «похожих» без ключа и регистрации,
// поэтому рекомендации работают «из коробки», когда ключ Last.fm не задан.
//
// ВАЖНО: Deezer здесь — только ГРАФ (кто на кого похож, топ-треки, чарт). Аудио
// из Deezer не берётся (публичный API отдаёт лишь 30-сек превью); звук по-
// прежнему приходит из YouTube/SoundCloud/Yandex/локальных файлов через обычный
// поиск. Возвращаемые названия «исполнитель+трек» движок ищет в реальных
// проигрываемых источниках.
//
// Реализовано прямыми HTTP-вызовами (как lastfm/yandex/spotify-адаптеры), без
// внешней зависимости-обёртки: API простой и без авторизации.
package deezer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"Cryon2/internal/logging"
)

// apiBase — точка входа публичного API Deezer (формат JSON, без ключа).
const apiBase = "https://api.deezer.com"

// Client — клиент Deezer. Ключа нет: сервис всегда «доступен», поэтому граф
// рекомендаций работает без какой-либо настройки пользователем.
type Client struct {
	httpClient *http.Client
	// baseURL — точка входа API. Обычно apiBase; переопределяется в тестах, чтобы
	// прогонять разбор ответов через httptest-сервер без сети.
	baseURL string
}

// New создаёт бесключевой клиент Deezer.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 12 * time.Second},
		baseURL:    apiBase,
	}
}

// Available сообщает, что клиент готов к работе. Для Deezer это всегда true
// (ключ не нужен) — сохранено методом ради симметрии с lastfm.Client, чтобы
// движок обращался к обоим графам одинаково.
func (c *Client) Available() bool {
	return c != nil
}

// Artist — похожий/чартовый артист. Match — синтетическая близость 0..1: Deezer
// отдаёт related как РАНЖИРОВАННЫЙ список без числовой оценки, поэтому близость
// выводим из позиции (верхние — ближе), чтобы движок взвешивал кандидатов так же,
// как по Last.fm-полю match.
type Artist struct {
	ID    int64
	Name  string
	Match float64
}

// Track — трек из топа артиста или чарта. Match — та же синтетическая близость
// по рангу (см. Artist).
type Track struct {
	Title  string
	Artist string
	Match  float64
}

// SimilarArtists возвращает похожих артистов по имени: сначала находит артиста
// (/search/artist), затем берёт его related (/artist/{id}/related). Пустой
// результат без ошибки — если артист не найден.
func (c *Client) SimilarArtists(ctx context.Context, artist string, limit int) ([]Artist, error) {
	artist = strings.TrimSpace(artist)
	if !c.Available() || artist == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	id, ok, err := c.searchArtistID(ctx, artist)
	if err != nil || !ok {
		return nil, err
	}
	return c.relatedArtists(ctx, id, limit)
}

// TopArtistTracks возвращает популярные треки артиста по имени: /search/artist →
// /artist/{id}/top. Это источник конкретных названий для последующего поиска в
// проигрываемых источниках.
func (c *Client) TopArtistTracks(ctx context.Context, artist string, limit int) ([]Track, error) {
	artist = strings.TrimSpace(artist)
	if !c.Available() || artist == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	id, ok, err := c.searchArtistID(ctx, artist)
	if err != nil || !ok {
		return nil, err
	}
	return c.topTracks(ctx, id, artist, limit)
}

// searchArtistID находит ID артиста по имени (первый и наиболее релевантный
// результат /search/artist). ok=false без ошибки — если ничего не найдено.
func (c *Client) searchArtistID(ctx context.Context, name string) (int64, bool, error) {
	var payload struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, "/search/artist", url.Values{
		"q":     {name},
		"limit": {"1"},
	}, &payload); err != nil {
		return 0, false, err
	}
	if payload.Error != nil {
		return 0, false, payload.Error.err()
	}
	if len(payload.Data) == 0 || payload.Data[0].ID == 0 {
		return 0, false, nil
	}
	return payload.Data[0].ID, true, nil
}

// relatedArtists возвращает похожих артистов по ID. Match выводится из ранга.
func (c *Client) relatedArtists(ctx context.Context, id int64, limit int) ([]Artist, error) {
	var payload struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, "/artist/"+strconv.FormatInt(id, 10)+"/related", url.Values{
		"limit": {strconv.Itoa(limit)},
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		return nil, payload.Error.err()
	}
	out := make([]Artist, 0, len(payload.Data))
	n := len(payload.Data)
	for i, a := range payload.Data {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		out = append(out, Artist{ID: a.ID, Name: name, Match: rankMatch(i, n)})
	}
	return out, nil
}

// topTracks возвращает топ-треки артиста по ID. artistFallback подставляется,
// если в ответе имя артиста пустое.
func (c *Client) topTracks(ctx context.Context, id int64, artistFallback string, limit int) ([]Track, error) {
	var payload struct {
		Data []struct {
			Title  string `json:"title"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"data"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, "/artist/"+strconv.FormatInt(id, 10)+"/top", url.Values{
		"limit": {strconv.Itoa(limit)},
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		return nil, payload.Error.err()
	}
	out := make([]Track, 0, len(payload.Data))
	n := len(payload.Data)
	for i, t := range payload.Data {
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		name := strings.TrimSpace(t.Artist.Name)
		if name == "" {
			name = artistFallback
		}
		out = append(out, Track{Title: title, Artist: name, Match: rankMatch(i, n)})
	}
	return out, nil
}

// ChartTracks возвращает глобальный чарт треков — «якорь» для холодного старта,
// когда о вкусе пользователя ещё ничего не известно (ни библиотеки, ни истории).
func (c *Client) ChartTracks(ctx context.Context, limit int) ([]Track, error) {
	if !c.Available() {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	var payload struct {
		Tracks struct {
			Data []struct {
				Title  string `json:"title"`
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"data"`
		} `json:"tracks"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, "/chart", url.Values{"limit": {strconv.Itoa(limit)}}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		return nil, payload.Error.err()
	}
	out := make([]Track, 0, len(payload.Tracks.Data))
	n := len(payload.Tracks.Data)
	for i, t := range payload.Tracks.Data {
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		out = append(out, Track{Title: title, Artist: strings.TrimSpace(t.Artist.Name), Match: rankMatch(i, n)})
	}
	return out, nil
}

// ChartArtists возвращает глобальный чарт артистов (тоже для холодного старта:
// из них можно строить обычный similar-обход).
func (c *Client) ChartArtists(ctx context.Context, limit int) ([]Artist, error) {
	if !c.Available() {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	var payload struct {
		Artists struct {
			Data []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
		} `json:"artists"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, "/chart", url.Values{"limit": {strconv.Itoa(limit)}}, &payload); err != nil {
		return nil, err
	}
	if payload.Error != nil {
		return nil, payload.Error.err()
	}
	out := make([]Artist, 0, len(payload.Artists.Data))
	n := len(payload.Artists.Data)
	for i, a := range payload.Artists.Data {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		out = append(out, Artist{ID: a.ID, Name: name, Match: rankMatch(i, n)})
	}
	return out, nil
}

// deezerError — тело ошибки Deezer (top-level "error"). Deezer отвечает 200 OK
// даже на ошибку, а признак кладёт сюда.
type deezerError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *deezerError) err() error {
	return fmt.Errorf("deezer: %s (%s, код %d)", e.Message, e.Type, e.Code)
}

// rankMatch выводит синтетическую близость 0..1 из позиции в ранжированном
// списке длиной n: верхний элемент ≈1.0, дальше линейно затухает. Так вес,
// который движок считает как (match+0.1), убывает по рангу — как если бы Deezer
// отдавал числовую близость.
func rankMatch(i, n int) float64 {
	if n <= 1 {
		return 1.0
	}
	m := float64(n-i) / float64(n)
	if m < 0 {
		return 0
	}
	return m
}

// get выполняет GET к API Deezer и декодирует JSON в dst. path — путь от baseURL
// (например "/artist/27/related"), query — параметры запроса.
func (c *Client) get(ctx context.Context, path string, query url.Values, dst any) error {
	base := c.baseURL
	if base == "" {
		base = apiBase
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		return fmt.Errorf("deezer: статус %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		logging.L().Debug("deezer: не удалось разобрать ответ", "err", err)
		return err
	}
	return nil
}
