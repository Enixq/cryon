// Package lyrics — клиент к публичному API lrclib.net: тексты песен, в том
// числе синхронизированные (формат LRC с таймкодами). API бесплатный и не
// требует ключа, поэтому тексты работают из коробки, без настройки источника.
//
// Реализовано прямыми HTTP-вызовами (как lastfm/yandex-адаптеры). Сначала
// пробуем точное совпадение (/api/get по артисту, треку, альбому и
// длительности), при промахе — поиск (/api/search) с выбором ближайшего
// результата.
package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"Cryon2/internal/logging"
)

const (
	apiGet    = "https://lrclib.net/api/get"
	apiSearch = "https://lrclib.net/api/search"
	// userAgent — lrclib просит указывать клиента (их гайдлайны). Ссылка на
	// проект помогает им связаться при проблемах и не блокировать трафик.
	userAgent = "Cryon2 (https://github.com/cryon/cryon2)"
)

// Result — текст песни для фронтенда. Synced — строки LRC с таймкодами
// ([mm:ss.xx]текст), Plain — обычный текст без таймкодов. Instrumental=true —
// песня инструментальная (текста нет намеренно), это не ошибка.
type Result struct {
	Synced       string `json:"synced"`
	Plain        string `json:"plain"`
	Instrumental bool   `json:"instrumental"`
	// Found — нашёлся ли текст. false — не найден (не путать с инструментальным).
	Found bool `json:"found"`
}

// apiTrack — ответ lrclib для одного трека.
type apiTrack struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// Client — клиент lrclib. Без состояния/ключа; хранит лишь http.Client.
type Client struct {
	httpClient *http.Client
}

// New создаёт клиент с разумным таймаутом.
func New() *Client {
	return &Client{httpClient: &http.Client{Timeout: 12 * time.Second}}
}

// Get возвращает текст песни. artist/title обязательны; album и durationS —
// уточняющие (durationS ≤ 0 игнорируется). Ошибка — только при сетевом сбое;
// «не найдено» отдаётся как Result{Found:false}, а не ошибкой.
func (c *Client) Get(ctx context.Context, artist, title, album string, durationS int) (Result, error) {
	artist = strings.TrimSpace(artist)
	title = strings.TrimSpace(title)
	if artist == "" || title == "" {
		return Result{}, nil
	}

	// 1) Точное совпадение по метаданным.
	if t, ok, err := c.getExact(ctx, artist, title, album, durationS); err != nil {
		return Result{}, err
	} else if ok {
		return toResult(t), nil
	}

	// 2) Фолбэк: поиск и ближайший результат.
	t, ok, err := c.search(ctx, artist, title)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{Found: false}, nil
	}
	return toResult(t), nil
}

func toResult(t apiTrack) Result {
	return Result{
		Synced:       t.SyncedLyrics,
		Plain:        t.PlainLyrics,
		Instrumental: t.Instrumental,
		Found:        t.Instrumental || t.SyncedLyrics != "" || t.PlainLyrics != "",
	}
}

// getExact запрашивает /api/get. Возвращает (трек, найдено, ошибка). 404 — это
// «не найдено» (ok=false), а не ошибка.
func (c *Client) getExact(ctx context.Context, artist, title, album string, durationS int) (apiTrack, bool, error) {
	q := url.Values{}
	q.Set("artist_name", artist)
	q.Set("track_name", title)
	if strings.TrimSpace(album) != "" {
		q.Set("album_name", album)
	}
	if durationS > 0 {
		q.Set("duration", fmt.Sprintf("%d", durationS))
	}
	var t apiTrack
	status, err := c.doJSON(ctx, apiGet+"?"+q.Encode(), &t)
	if err != nil {
		return apiTrack{}, false, err
	}
	if status == http.StatusNotFound {
		return apiTrack{}, false, nil
	}
	if status != http.StatusOK {
		return apiTrack{}, false, fmt.Errorf("lrclib get: статус %d", status)
	}
	return t, true, nil
}

// search запрашивает /api/search и берёт первый результат, у которого есть
// хоть какой-то текст (или он инструментальный).
func (c *Client) search(ctx context.Context, artist, title string) (apiTrack, bool, error) {
	q := url.Values{}
	q.Set("artist_name", artist)
	q.Set("track_name", title)
	var list []apiTrack
	status, err := c.doJSON(ctx, apiSearch+"?"+q.Encode(), &list)
	if err != nil {
		return apiTrack{}, false, err
	}
	if status != http.StatusOK {
		return apiTrack{}, false, nil
	}
	// Приоритет — синхронизированный текст, затем обычный.
	var plainMatch *apiTrack
	for i := range list {
		if list[i].SyncedLyrics != "" {
			return list[i], true, nil
		}
		if plainMatch == nil && (list[i].PlainLyrics != "" || list[i].Instrumental) {
			plainMatch = &list[i]
		}
	}
	if plainMatch != nil {
		return *plainMatch, true, nil
	}
	return apiTrack{}, false, nil
}

// doJSON выполняет GET и декодирует тело в out (если статус 200). Возвращает
// HTTP-статус, чтобы вызывающий отличал 404 от сетевой ошибки.
func (c *Client) doJSON(ctx context.Context, endpoint string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.L().Debug("lrclib запрос не удался", "err", err)
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Тело у не-200 нам не нужно — но прочитаем и отбросим, чтобы соединение
		// переиспользовалось.
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("lrclib: разбор ответа: %w", err)
	}
	return resp.StatusCode, nil
}
