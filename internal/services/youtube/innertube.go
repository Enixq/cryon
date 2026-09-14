package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"Cryon2/internal/domain"
)

// InnerTube — приватный, но публично-статический JSON-API (youtubei/v1),
// которым пользуются сами клиенты YouTube. Ключ ниже НЕ секрет: он зашит в
// веб-клиент YouTube, не привязан к аккаунту и не открывает доступ к приватным
// данным, поэтому держать его в открытой сборке допустимо (политика «секреты не
// в бинарник» не нарушается). В отличие от парсинга HTML-страницы, InnerTube
// отдаёт структурированный JSON, поэтому это самый надёжный бесключевой путь
// поиска и работает на Android (чистый net/http, без внешнего yt-dlp).
const innertubeWebKey = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"

const (
	innertubeWebClientName    = "WEB"
	innertubeWebClientVersion = "2.20240826.01.00"
	innertubeWebUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// innertubeSearchParamsVideos фильтрует выдачу до одних видео (эквивалент
// sp=EgIQAQ== в /results). Без него в результаты лезут каналы и плейлисты.
const innertubeSearchParamsVideos = "EgIQAQ=="

type innertubeContext struct {
	Client innertubeClient `json:"client"`
}

type innertubeClient struct {
	ClientName    string `json:"clientName"`
	ClientVersion string `json:"clientVersion"`
	HL            string `json:"hl"`
	GL            string `json:"gl"`
}

type innertubeSearchRequest struct {
	Context innertubeContext `json:"context"`
	Query   string           `json:"query"`
	Params  string           `json:"params,omitempty"`
}

// searchByInnerTube ищет треки через youtubei/v1/search. Ответ — структурный
// JSON, который мы разбираем тем же рекурсивным сборщиком videoRenderer, что и
// HTML-страницу (collectYouTubeVideoRenderers): разбор один и тот же и уже
// покрыт тестами. Пустой результат или ошибка приводят к штатному откату на
// следующий путь в Search (страница → Bing → yt-dlp), поэтому этот метод
// безопасен как дополнительный слой и не может ухудшить работающие пути.
func (s *Service) searchByInnerTube(ctx context.Context, query string) ([]domain.Track, error) {
	reqBody := innertubeSearchRequest{
		Context: innertubeContext{
			Client: innertubeClient{
				ClientName:    innertubeWebClientName,
				ClientVersion: innertubeWebClientVersion,
				HL:            "en",
				GL:            "US",
			},
		},
		Query:  query,
		Params: innertubeSearchParamsVideos,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	endpoint := "https://www.youtube.com/youtubei/v1/search?key=" + innertubeWebKey + "&prettyPrint=false"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", innertubeWebUserAgent)
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("X-Youtube-Client-Name", "1")
	req.Header.Set("X-Youtube-Client-Version", innertubeWebClientVersion)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("youtube innertube search returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}

	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("decode youtube innertube search: %w", err)
	}
	tracks := make([]domain.Track, 0, 20)
	seen := make(map[string]bool)
	collectYouTubeVideoRenderers(data, &tracks, seen)
	return tracks, nil
}
