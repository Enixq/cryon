package audiofetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/kkdai/youtube/v2"

	"Cryon2/internal/domain"
)

// AudioStream — прямой аудиопоток трека и его метаданные.
type AudioStream struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Quality  string `json:"quality"`
	Duration int64  `json:"duration"`
}

// ytClient переиспользуется между вызовами: youtube.Client безопасен для
// конкурентного использования, и один экземпляр не пересоздаёт http-клиент на
// каждый трек (а также переиспользует внутреннее состояние библиотеки).
//
// HTTPClient задан явно с КОНЕЧНЫМИ таймаутами. По умолчанию kkdai/youtube
// берёт http.DefaultClient (Timeout: 0) — из-за этого зависший запрос к
// InnerTube/странице YouTube (медленный TLS под VPN, троттлинг, обрыв сети)
// висел вечно и держал горутину, а с ней — singleflight в localserver (все
// ждущие Range-запросы плеера). Это и была одна из причин «музыка не всегда
// включается»: плеер бесконечно буферизовал без ошибки. Бюджеты разнесены по
// фазам под медленный VPN, но общий предел конечен.
var ytClient = youtube.Client{HTTPClient: newYouTubeMetadataClient()}

func newYouTubeMetadataClient() *http.Client {
	return &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   12 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 2 * time.Second,
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

const minimumAudioBitrate = 96000

type audioStreamAttempt struct {
	name    string
	timeout time.Duration
	resolve func(context.Context) (*AudioStream, error)
}

type audioStreamResult struct {
	name   string
	stream *AudioStream
	err    error
}

// raceAudioStreams returns the first usable stream from independent resolvers.
// A blocked YouTube endpoint must not prevent another supported client from
// resolving the same video.
func raceAudioStreams(ctx context.Context, attempts []audioStreamAttempt) (*AudioStream, error) {
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan audioStreamResult, len(attempts))
	for _, attempt := range attempts {
		go func(attempt audioStreamAttempt) {
			attemptCtx, stop := context.WithTimeout(raceCtx, attempt.timeout)
			defer stop()
			stream, err := attempt.resolve(attemptCtx)
			results <- audioStreamResult{name: attempt.name, stream: stream, err: err}
		}(attempt)
	}

	failures := make([]string, 0, len(attempts))
	for range attempts {
		var result audioStreamResult
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result = <-results:
		}
		if result.err == nil && result.stream != nil && result.stream.URL != "" {
			return result.stream, nil
		}
		if result.err != nil {
			failures = append(failures, result.name+": "+result.err.Error())
		}
	}
	if len(failures) == 0 {
		return nil, fmt.Errorf("аудиопоток YouTube не найден")
	}
	return nil, fmt.Errorf("аудиопоток YouTube не получен (%s)", strings.Join(failures, "; "))
}

// GetYouTubeAudioStream извлекает прямой аудиопоток видео YouTube по его ID.
// Работает без API-ключа. Первичная попытка (kkdai/youtube) ограничена
// собственным под-дедлайном: даже если она зависнет или упадёт, у резолвера
// останется бюджет на бесключевые фолбэки (InnerTube → Piped → yt-dlp) в
// пределах общего дедлайна, который ставит вызывающий код (core.GetAudioStream).
func GetYouTubeAudioStream(ctx context.Context, videoID string) (*AudioStream, error) {
	stream, primaryErr := raceAudioStreams(ctx, []audioStreamAttempt{
		{
			name:    "metadata",
			timeout: 18 * time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamPrimary(ctx, videoID)
			},
		},
		{
			name:    "innertube",
			timeout: 18 * time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamWithInnerTube(ctx, videoID)
			},
		},
	})
	if primaryErr == nil {
		return stream, nil
	}

	fallback, fallbackErr := raceAudioStreams(ctx, []audioStreamAttempt{
		{
			name:    "piped",
			timeout: 12 * time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamWithPiped(ctx, videoID)
			},
		},
		{
			name:    "yt-dlp",
			timeout: 20 * time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamWithYtDlp(ctx, videoID)
			},
		},
	})
	if fallbackErr == nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("не удалось получить аудиопоток YouTube: основной путь: %v; резервный путь: %w", primaryErr, fallbackErr)
}

// getYouTubeAudioStreamPrimary — первичный путь через kkdai/youtube (клиент
// ANDROID_VR по умолчанию: InnerTube-плеер, прямые URL без расшифровки сигнатур).
// Никаких фолбэков внутри не делает — ими управляет GetYouTubeAudioStream, чтобы
// цепочка была единой и предсказуемой по времени.
func getYouTubeAudioStreamPrimary(ctx context.Context, videoID string) (*AudioStream, error) {
	video, err := ytClient.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить данные видео YouTube: %w", err)
	}

	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		return nil, fmt.Errorf("подходящих аудиоформатов не нашлось")
	}

	// Предпочитаем аудио-only поток (mimeType "audio/..."): он в разы легче
	// комбинированного видео+аудио, поэтому запускается быстрее. Среди аудио
	// берём вариант с наибольшим битрейтом. Если аудио-only нет — первый доступный.
	format := formats[0]
	var bestAudio *youtube.Format
	for i := range formats {
		if !strings.HasPrefix(formats[i].MimeType, "audio/") {
			continue
		}
		if formats[i].Bitrate < minimumAudioBitrate {
			continue
		}
		if bestAudio == nil || formats[i].Bitrate > bestAudio.Bitrate {
			bestAudio = &formats[i]
		}
	}
	if bestAudio != nil {
		format = *bestAudio
	} else if format.Bitrate < minimumAudioBitrate {
		return nil, fmt.Errorf("качество аудиопотока YouTube ниже 96 кбит/с")
	}

	streamURL, err := ytClient.GetStreamURLContext(ctx, video, &format)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить URL аудиопотока YouTube: %w", err)
	}

	return &AudioStream{
		URL:      streamURL,
		Format:   format.AudioQuality,
		Quality:  fmt.Sprintf("%dbps", format.Bitrate),
		Duration: int64(video.Duration.Seconds()),
	}, nil
}

// ParseTrackFromURL определяет источник и ID трека по ссылке.
func ParseTrackFromURL(rawURL string) (domain.ServiceID, string, error) {
	if strings.Contains(rawURL, "youtube.com") || strings.Contains(rawURL, "youtu.be") {
		if strings.Contains(rawURL, "youtu.be/") {
			parts := strings.Split(rawURL, "youtu.be/")
			id := strings.Split(parts[1], "?")[0]
			return domain.ServiceYouTube, id, nil
		}
		parts := strings.Split(rawURL, "v=")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("некорректная ссылка YouTube")
		}
		videoID := strings.Split(parts[1], "&")[0]
		return domain.ServiceYouTube, videoID, nil
	}

	if strings.Contains(rawURL, "spotify.com/track") {
		parts := strings.Split(rawURL, "/track/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("некорректная ссылка Spotify")
		}
		trackID := strings.Split(parts[1], "?")[0]
		return domain.ServiceSpotify, trackID, nil
	}

	if strings.Contains(rawURL, "soundcloud.com") {
		return domain.ServiceSoundCloud, rawURL, nil
	}

	return "", "", fmt.Errorf("неподдерживаемая ссылка")
}

type pipedAudioStream struct {
	URL      string `json:"url"`
	Bitrate  int    `json:"bitrate"`
	MimeType string `json:"mimeType"`
}

type pipedStreamsResponse struct {
	AudioStreams []pipedAudioStream `json:"audioStreams"`
	Duration     int64              `json:"duration"`
}

func getYouTubeAudioStreamFallback(ctx context.Context, videoID string) (*AudioStream, error) {
	// Порядок фолбэков — от самого надёжного бесключевого к внешнему бинарнику.
	// InnerTube (клиент IOS) и Piped работают на Android; yt-dlp — только на
	// десктопе (на телефоне его нет).
	if stream, err := getYouTubeAudioStreamWithInnerTube(ctx, videoID); err == nil {
		return stream, nil
	}
	if stream, err := getYouTubeAudioStreamWithPiped(ctx, videoID); err == nil {
		return stream, nil
	}
	return getYouTubeAudioStreamWithYtDlp(ctx, videoID)
}

func getYouTubeAudioStreamWithPiped(ctx context.Context, videoID string) (*AudioStream, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	// Публичные Piped-инстансы часто умирают/меняются — держим расширенный
	// список и перебираем по очереди (мёртвые отсеиваются быстро по ошибке
	// соединения). Это лишь третичный фолбэк: основной бесключевой путь на
	// Android — kkdai и InnerTube выше.
	instances := []string{
		"https://pipedapi.kavin.rocks",
		"https://pipedapi.adminforge.de",
		"https://api.piped.yt",
		"https://pipedapi.leptons.xyz",
		"https://pipedapi.r4fo.com",
		"https://pipedapi.reallyaweso.me",
		"https://pipedapi.ducks.party",
	}
	var lastErr error
	for _, instance := range instances {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, instance+"/streams/"+videoID, nil)
		if err != nil {
			lastErr = err
			continue
		}
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			lastErr = fmt.Errorf("Piped returned HTTP %d", response.StatusCode)
			continue
		}
		var payload pipedStreamsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			lastErr = err
			continue
		}
		var best *pipedAudioStream
		for i := range payload.AudioStreams {
			stream := &payload.AudioStreams[i]
			if stream.URL == "" || stream.Bitrate < minimumAudioBitrate || !strings.HasPrefix(stream.MimeType, "audio/") {
				continue
			}
			if best == nil || stream.Bitrate > best.Bitrate {
				best = stream
			}
		}
		if best != nil {
			return &AudioStream{URL: best.URL, Format: best.MimeType, Quality: fmt.Sprintf("%dbps", best.Bitrate), Duration: payload.Duration}, nil
		}
		lastErr = fmt.Errorf("Piped returned no audio stream")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no Piped instances configured")
	}
	return nil, lastErr
}

func getYouTubeAudioStreamWithYtDlp(ctx context.Context, videoID string) (*AudioStream, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", "--no-warnings", "--no-playlist", "--socket-timeout", "15", "--retries", "2", "-f", "bestaudio[abr>=96]/bestaudio", "--get-url", "https://www.youtube.com/watch?v="+videoID)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	streamURL := strings.TrimSpace(string(out))
	if streamURL == "" {
		return nil, fmt.Errorf("yt-dlp returned an empty stream URL")
	}
	return &AudioStream{URL: streamURL, Format: "audio", Quality: "best available (>=96 kbps)"}, nil
}
