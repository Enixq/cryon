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
	"Cryon2/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"Cryon2/internal/logging"
)

const (
	apiGet    = "https://lrclib.net/api/get"
	apiSearch = "https://lrclib.net/api/search"
	// userAgent — lrclib просит указывать клиента (их гайдлайны). Ссылка на
	// проект помогает им связаться при проблемах и не блокировать трафик.
	userAgent = "Cryon (https://github.com/Enixq/cryon)"
)

// Result — текст песни для фронтенда. Synced — строки LRC с таймкодами
// ([mm:ss.xx]текст), Plain — обычный текст без таймкодов. Instrumental=true —
// песня инструментальная (текста нет намеренно), это не ошибка.
type Result struct {
	Synced       string `json:"synced"`
	Plain        string `json:"plain"`
	Instrumental bool   `json:"instrumental"`
	// Found — нашёлся ли текст. false — не найден (не путать с инструментальным).
	Found    bool   `json:"found"`
	Provider string `json:"provider,omitempty"`
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
	title = stripArtistPrefix(artist, cleanTitle(title))
	if artist == "" || title == "" {
		return Result{}, nil
	}

	// 1) Точное совпадение по метаданным.
	fallbackCtx, fallbackCancel := context.WithTimeout(ctx, 12*time.Second)
	defer fallbackCancel()
	type reply struct {
		result Result
		err    error
	}
	fallback := make(chan reply, 1)
	go func() {
		result, err := c.GetProvider(fallbackCtx, artist, title, album, durationS, "ovh")
		fallback <- reply{result, err}
	}()
	result, mainErr := c.GetProvider(ctx, artist, title, album, durationS, "lrclib")
	if result.Found {
		return result, nil
	}
	select {
	case other := <-fallback:
		if other.result.Found {
			return other.result, nil
		}
		if mainErr != nil && other.err != nil {
			return Result{}, fmt.Errorf("источники текстов не ответили: %w", mainErr)
		}
		return Result{}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (c *Client) getLyricsOVH(ctx context.Context, artist, title string) (string, error) {
	endpoint := "https://api.lyrics.ovh/v1/" + url.PathEscape(artist) + "/" + url.PathEscape(title)
	var payload struct {
		Lyrics string `json:"lyrics"`
	}
	status, err := c.doJSON(ctx, endpoint, &payload)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", nil
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("lyrics.ovh: HTTP %d", status)
	}
	return strings.TrimSpace(payload.Lyrics), nil
}

func toResult(t apiTrack) Result {
	return Result{
		Provider:     "LRCLIB",
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
	q.Set("artist_name", primaryArtist(artist))
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
func (c *Client) search(ctx context.Context, artist, title string, durationS int) (apiTrack, bool, error) {
	q := url.Values{}
	q.Set("artist_name", primaryArtist(artist))
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
		if domain.MusicKey(cleanTitle(list[i].TrackName)) != domain.MusicKey(title) || domain.MusicKey(primaryArtist(list[i].ArtistName)) != domain.MusicKey(primaryArtist(artist)) {
			continue
		}
		if durationS > 0 && list[i].Duration > 0 && math.Abs(list[i].Duration-float64(durationS)) > math.Max(30, float64(durationS)*0.12) {
			continue
		}
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("lrclib: разбор ответа: %w", err)
	}
	return resp.StatusCode, nil
}

var lyricQualifier = regexp.MustCompile(`(?i)\s*[\[(](?:official(?: music)?(?: video| audio)?|lyrics?|lyric video|audio|video|текст(?: песни)?|клип)[\])]`)

func cleanTitle(title string) string {
	return strings.TrimSpace(lyricQualifier.ReplaceAllString(title, ""))
}

func (c *Client) GetProvider(ctx context.Context, artist, title, album string, durationS int, provider string) (Result, error) {
	title = stripArtistPrefix(artist, cleanTitle(title))
	if provider == "ovh" {
		ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		variants := []string{strings.TrimSpace(artist), primaryArtist(artist)}
		if first, _, ok := strings.Cut(primaryArtist(artist), ", "); ok {
			variants = append(variants, first)
		}
		seen := map[string]bool{}
		var lastErr error
		for _, name := range variants {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			attempt, stop := context.WithTimeout(ctx, 4*time.Second)
			text, err := c.getLyricsOVH(attempt, name, title)
			stop()
			if text != "" {
				return Result{Plain: text, Found: true, Provider: "lyrics.ovh"}, nil
			}
			if err != nil {
				lastErr = err
			}
		}
		return Result{Provider: "lyrics.ovh"}, lastErr
	}
	if provider == "lrclib" {
		ctx, cancel := context.WithTimeout(ctx, 14*time.Second)
		defer cancel()
		exactCtx, exactCancel := context.WithTimeout(ctx, 5*time.Second)
		defer exactCancel()
		if t, ok, err := c.getExact(exactCtx, artist, title, album, durationS); err == nil && ok && toResult(t).Found {
			return toResult(t), nil
		}
		searchCtx, searchCancel := context.WithTimeout(ctx, 8*time.Second)
		defer searchCancel()
		t, ok, err := c.search(searchCtx, artist, title, durationS)
		if err != nil {
			return Result{}, err
		}
		if ok {
			return toResult(t), nil
		}
		return Result{Provider: "LRCLIB"}, nil
	}
	return c.Get(ctx, artist, title, album, durationS)
}

var artistFeature = regexp.MustCompile(`(?i)\s+(?:feat\.?|ft\.?|featuring)\s+`)

func primaryArtist(artist string) string { return strings.TrimSpace(artistFeature.Split(artist, 2)[0]) }
func stripArtistPrefix(artist, title string) string {
	for _, separator := range []string{" - ", " \u2014 ", " \u2013 "} {
		if prefix, rest, ok := strings.Cut(title, separator); ok && domain.MusicKey(prefix) == domain.MusicKey(primaryArtist(artist)) {
			return strings.TrimSpace(rest)
		}
	}
	return title
}
