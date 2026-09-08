package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"Cryon2/internal/logging"
)

// clientIDTTL — как долго переиспользуем автоматически найденный client_id
// перед повторным обнаружением. SoundCloud меняет его редко.
const clientIDTTL = 6 * time.Hour

// clientIDPattern вытаскивает client_id из JS-бандлов SoundCloud.
var clientIDPattern = regexp.MustCompile(`client_id\s*[:=]\s*"([0-9a-zA-Z]{20,})"`)

// mobileClientIDPattern вытаскивает client_id прямо из HTML мобильного сайта:
// m.soundcloud.com — Next.js-приложение и печатает `"clientId":"…"` в
// __NEXT_DATA__, поэтому ключ достаётся одним запросом, без обхода бандлов.
var mobileClientIDPattern = regexp.MustCompile(`"client_?[iI]d"\s*:\s*"([0-9a-zA-Z]{20,})"`)

// scriptSrcPattern находит ссылки на JS-бандлы на главной странице.
var scriptSrcPattern = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)

// StreamInfo — разрешённый прямой аудиопоток трека SoundCloud.
type StreamInfo struct {
	URL        string
	MimeType   string
	DurationMs int
	ArtworkURL string
}

// ensureClientID возвращает рабочий client_id: заданный в конфиге, либо
// автоматически найденный на страницах SoundCloud (с кэшем на clientIDTTL).
// Благодаря этому SoundCloud играет и без ключа в переменных окружения.
func (s *Service) ensureClientID(ctx context.Context) (string, error) {
	if id := s.configuredClientID(); id != "" {
		return id, nil
	}

	s.mu.Lock()
	if s.discoveredID != "" && time.Since(s.discoveredAt) < clientIDTTL {
		id := s.discoveredID
		s.mu.Unlock()
		return id, nil
	}
	s.mu.Unlock()

	id, err := s.discoverClientID(ctx)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.discoveredID = id
	s.discoveredAt = time.Now()
	s.mu.Unlock()
	logging.L().Debug("soundcloud: client_id обнаружен автоматически")
	return id, nil
}

// refreshClientID принудительно переоткрывает client_id со страниц SoundCloud,
// минуя кэш и сохранённый вручную ключ (он мог устареть). Свежий ключ кладётся
// в кэш автоопределения, поэтому последующие вызовы ensureClientID его увидят.
// Используется как самовосстановление, когда api-v2 отвергает текущий ключ.
func (s *Service) refreshClientID(ctx context.Context) (string, error) {
	id, err := s.discoverClientID(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.discoveredID = id
	s.discoveredAt = time.Now()
	s.mu.Unlock()
	logging.L().Debug("soundcloud: client_id переоткрыт после отказа api")
	return id, nil
}

// discoverClientID добывает client_id со страниц SoundCloud.
//
// Сначала пробуем m.soundcloud.com: мобильный сайт печатает `"clientId":"…"`
// прямо в HTML, это один запрос вместо обхода бандлов. И, что важнее, основной
// домен soundcloud.com (как и часть его CDN-хостов) в ряде сетей закрыт DPI —
// запрос к нему отваливается по таймауту, из-за чего client_id не находился
// НИКОГДА, api-v2 не использовался вовсе и поиск молча деградировал в Bing: без
// обложек, без длительностей и с воспроизведением через внешний сайт. При этом
// сам api-v2.soundcloud.com отвечает — не хватало только ключа.
func (s *Service) discoverClientID(ctx context.Context) (string, error) {
	if html, err := s.fetchText(ctx, "https://m.soundcloud.com/"); err == nil {
		if m := mobileClientIDPattern.FindStringSubmatch(html); m != nil {
			return m[1], nil
		}
	} else {
		logging.L().Debug("soundcloud: мобильный сайт недоступен", "err", err)
	}

	home, err := s.fetchText(ctx, "https://soundcloud.com/")
	if err != nil {
		return "", fmt.Errorf("soundcloud: не удалось загрузить главную: %w", err)
	}

	// Бандлы перечислены в конце документа; проверяем в обратном порядке,
	// т.к. client_id чаще всего в последнем из них.
	matches := scriptSrcPattern.FindAllStringSubmatch(home, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		src := matches[i][1]
		if !strings.HasPrefix(src, "http") || !strings.Contains(src, "sndcdn.com") {
			continue
		}
		js, ferr := s.fetchText(ctx, src)
		if ferr != nil {
			continue
		}
		if m := clientIDPattern.FindStringSubmatch(js); m != nil {
			return m[1], nil
		}
	}
	return "", fmt.Errorf("soundcloud: client_id не найден на страницах")
}

// ResolveStream разрешает прямой аудиопоток трека по его ссылке или
// числовому id. Возвращает прогрессивный (mp3) поток, пригодный для
// HTML5 <audio> и mpv.
func (s *Service) ResolveStream(ctx context.Context, trackRef string) (*StreamInfo, error) {
	clientID, err := s.ensureClientID(ctx)
	if err != nil {
		return nil, err
	}

	track, err := s.fetchTrack(ctx, trackRef, clientID)
	if err != nil {
		return nil, err
	}

	// Выбираем прогрессивный поток (обычный HTTP mp3). HLS требует доп.
	// проигрывателя, поэтому используем его только как запасной вариант.
	var progressive, hls *transcoding
	for i := range track.Media.Transcodings {
		t := &track.Media.Transcodings[i]
		switch t.Format.Protocol {
		case "progressive":
			if progressive == nil {
				progressive = t
			}
		case "hls":
			if hls == nil {
				hls = t
			}
		}
	}

	chosen := progressive
	if chosen == nil {
		chosen = hls
	}
	if chosen == nil {
		return nil, fmt.Errorf("soundcloud: у трека нет доступных потоков")
	}

	streamURL, err := s.resolveTranscoding(ctx, chosen.URL, clientID)
	if err != nil {
		return nil, err
	}

	return &StreamInfo{
		URL:        streamURL,
		MimeType:   chosen.Format.MimeType,
		DurationMs: track.DurationMs,
		ArtworkURL: normalizeArtwork(track.ArtworkURL, s.ensureArtworkHost(ctx)),
	}, nil
}

// artworkHosts — взаимозаменяемые зеркала CDN обложек SoundCloud: по одному и
// тому же пути они отдают один и тот же файл. Порядок = приоритет, i1 первый,
// т.к. именно его подставляет сам api-v2.
var artworkHosts = []string{"i1.sndcdn.com", "i2.sndcdn.com", "i3.sndcdn.com", "i4.sndcdn.com"}

// artworkHostRe находит хост обложки в ссылке, чтобы подменить его на зеркало.
var artworkHostRe = regexp.MustCompile(`^https?://i[1-4]\.sndcdn\.com`)

// ensureArtworkHost выбирает первое ДОСТУПНОЕ зеркало CDN обложек и запоминает
// выбор на весь процесс. Нужно потому, что часть хостов (у пользователя — i1,
// i3 и i4) закрыта DPI: ссылка из api-v2 указывает на i1, картинка не грузится
// и в выдаче остаются серые заглушки, хотя i2 отдаёт тот же файл. Проба
// одноразовая и с коротким таймаутом, чтобы не задерживать поиск.
func (s *Service) ensureArtworkHost(ctx context.Context) string {
	s.artworkOnce.Do(func() {
		// Отдельный клиент с коротким таймаутом: заблокированный хост не должен
		// съедать 15 с общего таймаута. Контекст поиска намеренно не используем —
		// его отмена не должна «застывать» в кэше единственной пробы.
		probe := &http.Client{Timeout: 3 * time.Second}
		for _, host := range artworkHosts {
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+host+"/", nil)
			if err != nil {
				continue
			}
			resp, err := probe.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			// Любой HTTP-ответ (даже 404 на корень) означает, что хост доступен.
			s.artworkHost = host
			if host != artworkHosts[0] {
				logging.L().Info("soundcloud: обложки идут через зеркало CDN", "host", host)
			}
			return
		}
		logging.L().Warn("soundcloud: ни одно зеркало CDN обложек не ответило")
		s.artworkHost = artworkHosts[0]
	})
	return s.artworkHost
}

// normalizeArtwork повышает разрешение обложки SoundCloud (api отдаёт маленький
// large.jpg ~100px — заменяем на t500x500) и переводит её на доступное зеркало
// CDN. host пустой — хост оставляем как есть.
func normalizeArtwork(artworkURL, host string) string {
	if artworkURL == "" {
		return ""
	}
	out := strings.Replace(artworkURL, "-large.", "-t500x500.", 1)
	if host != "" {
		out = artworkHostRe.ReplaceAllString(out, "https://"+host)
	}
	return out
}

type transcoding struct {
	URL    string `json:"url"`
	Format struct {
		Protocol string `json:"protocol"`
		MimeType string `json:"mime_type"`
	} `json:"format"`
}

type scTrack struct {
	ID         int64  `json:"id"`
	DurationMs int    `json:"duration"`
	ArtworkURL string `json:"artwork_url"`
	Media      struct {
		Transcodings []transcoding `json:"transcodings"`
	} `json:"media"`
}

// fetchTrack получает метаданные трека: по числовому id через /tracks/{id},
// по ссылке — через /resolve.
func (s *Service) fetchTrack(ctx context.Context, trackRef, clientID string) (*scTrack, error) {
	var endpoint string
	if isNumericID(trackRef) {
		endpoint = "https://api-v2.soundcloud.com/tracks/" + url.PathEscape(trackRef) +
			"?client_id=" + url.QueryEscape(clientID)
	} else {
		endpoint = "https://api-v2.soundcloud.com/resolve?url=" +
			url.QueryEscape(trackRef) + "&client_id=" + url.QueryEscape(clientID)
	}

	body, err := s.fetchText(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("soundcloud: не удалось получить трек: %w", err)
	}

	var track scTrack
	if err := json.Unmarshal([]byte(body), &track); err != nil {
		return nil, fmt.Errorf("soundcloud: не удалось разобрать трек: %w", err)
	}
	return &track, nil
}

// resolveTranscoding обращается к URL транскодинга и получает финальную
// ссылку на аудиопоток.
func (s *Service) resolveTranscoding(ctx context.Context, transcodingURL, clientID string) (string, error) {
	sep := "?"
	if strings.Contains(transcodingURL, "?") {
		sep = "&"
	}
	endpoint := transcodingURL + sep + "client_id=" + url.QueryEscape(clientID)

	body, err := s.fetchText(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("soundcloud: не удалось разрешить поток: %w", err)
	}

	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return "", fmt.Errorf("soundcloud: не удалось разобрать поток: %w", err)
	}
	if resp.URL == "" {
		return "", fmt.Errorf("soundcloud: пустая ссылка на поток")
	}
	return resp.URL, nil
}

// fetchText выполняет GET и возвращает тело как строку.
func (s *Service) fetchText(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("статус %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

// isNumericID сообщает, состоит ли строка только из цифр (id трека SoundCloud).
func isNumericID(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
