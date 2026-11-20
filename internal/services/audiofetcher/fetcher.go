package audiofetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sort"
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

// HTTPClient задан явно с КОНЕЧНЫМИ таймаутами. По умолчанию kkdai/youtube
// берёт http.DefaultClient (Timeout: 0) — из-за этого зависший запрос к
// InnerTube/странице YouTube (медленный TLS под VPN, троттлинг, обрыв сети)
// висел вечно и держал горутину, а с ней — singleflight в localserver (все
// ждущие Range-запросы плеера). Это и была одна из причин «музыка не всегда
// включается»: плеер бесконечно буферизовал без ошибки. Бюджеты разнесены по
// фазам под медленный VPN, но общий предел конечен.
var ytHTTPClient = newYouTubeMetadataClient()

func newYouTubeMetadataClient() *http.Client {
	return &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: googleDialContext(&net.Dialer{
				Timeout:   12 * time.Second,
				KeepAlive: 30 * time.Second,
			}),
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

// streamProbeClient — контрольный Range-запрос к googlevideo при проверке
// потока. Идёт через google-транспорт, чтобы на Android (VPN не пускает Cryon
// в туннель) проба не падала на прямой блокировке youtube/googlevideo.
var streamProbeClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: googleDialContext(&net.Dialer{
			Timeout:   8 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
		TLSHandshakeTimeout:   12 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// streamProbeBytes — размер контрольного блока при проверке потока.
// Запрашивается в начале (0-1023) и в конце файла (total-1024..total-1).
// 1024 байт — достаточный размер для валидации аудиоформата.
const streamProbeBytes = 1024

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
// Работает без API-ключа и параллельно проверяет доступные маршруты.
func GetYouTubeAudioStream(ctx context.Context, videoID string) (*AudioStream, error) {
	attempts := []audioStreamAttempt{
		{
			name:    "metadata",
			timeout: 18 * time.Second,
			resolve: func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamPrimary(ctx, videoID)
			},
		},
		{
			name:    "innertube",
			timeout: 30 * time.Second,
			resolve: verifyResolver(func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamWithInnerTube(ctx, videoID)
			}),
		},
		{
			name:    "piped",
			timeout: 12 * time.Second,
			resolve: verifyResolver(func(ctx context.Context) (*AudioStream, error) {
				return getYouTubeAudioStreamWithPiped(ctx, videoID)
			}),
		},
	}
	if runtime.GOOS != "android" {
		attempts = append(attempts,
			audioStreamAttempt{
				name:    "yt-dlp",
				timeout: 20 * time.Second,
				resolve: verifyResolver(func(ctx context.Context) (*AudioStream, error) {
					return getYouTubeAudioStreamWithYtDlp(ctx, videoID)
				}),
			},
			audioStreamAttempt{
				name:    "yt-dlp-seekable",
				timeout: 20 * time.Second,
				resolve: verifyResolver(func(ctx context.Context) (*AudioStream, error) {
					return getSeekableYouTubeAudioStreamWithYtDlp(ctx, videoID)
				}),
			},
		)
	}
	return raceAudioStreams(ctx, attempts)
}

func verifyResolver(resolve func(context.Context) (*AudioStream, error)) func(context.Context) (*AudioStream, error) {
	return func(ctx context.Context) (*AudioStream, error) {
		stream, err := resolve(ctx)
		if err != nil || stream == nil || stream.URL == "" {
			return stream, err
		}
		// known — длина потока, если она уже известна по предыдущей проверке
		// (0 — ещё нет). Конец контрольного запроса никогда не выходит за конец
		// файла: googlevideo отвечает 403 на Range за пределами файла вместо
		// корректного 416, и такой запрос отбраковывал заведомо рабочий поток.
		//
		// Таймаут задаётся НА КАЖДЫЙ контрольный запрос, а не на всю проверку:
		// раньше общий 6-секундный бюджет делился между началом и концом файла,
		// и при медленном начале (VPN, троттлинг) на проверку хвоста оставалось
		// меньше секунды. Рабочий поток отбраковывался с «context deadline
		// exceeded» — это и была основная причина «то играет, то нет».
		probe := func(position, known int64, timeout time.Duration) (int64, error) {
			end := position + streamProbeBytes - 1
			if known > 0 && end > known-1 {
				end = known - 1
			}
			probeCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, stream.URL, nil)
			if err != nil {
				return 0, err
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", position, end))
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Referer", "https://www.youtube.com/")
			req.Header.Set("Origin", "https://www.youtube.com")
			resp, err := streamProbeClient.Do(req)
			if err != nil {
				var urlError *url.Error
				if errors.As(err, &urlError) {
					err = urlError.Err
				}
				return 0, fmt.Errorf("проверка аудиопотока: %w", err)
			}
			defer resp.Body.Close()
			contentType := strings.ToLower(resp.Header.Get("Content-Type"))
			if resp.StatusCode < 200 || resp.StatusCode >= 300 || strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/json") {
				return 0, fmt.Errorf("аудиопоток недоступен: HTTP %d", resp.StatusCode)
			}
			var sample [1]byte
			if _, err := io.ReadFull(resp.Body, sample[:]); err != nil {
				return 0, fmt.Errorf("пустой аудиопоток: %w", err)
			}
			var first, last, total int64
			if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &total); err == nil && resp.StatusCode == http.StatusPartialContent && first == position && last >= first {
				return total, nil
			}
			if position > 0 {
				return 0, fmt.Errorf("источник не поддерживает перемотку")
			}
			if resp.ContentLength > 0 {
				return resp.ContentLength, nil
			}
			return 0, nil
		}
		total, err := probe(0, 0, streamProbeTimeoutStart)
		if err != nil {
			return nil, err
		}
		if total > streamProbeBytes {
			if _, err := probe(total-streamProbeBytes, total, streamProbeTimeoutTail); err != nil {
				// Хвост не ответил. Различаем два принципиально разных случая:
				//  - HTTP 403/404/не-Range ответ: источник правда не отдаёт
				//    перемотку — такой поток отдавать нельзя, он замолкнет;
				//  - таймаут/сетевая ошибка: googlevideo троттлит дальний чанк.
				//    Пробуем середину файла; если она отвечает — поток годен.
				if !isTransientProbeError(err) {
					return nil, fmt.Errorf("аудиопоток недоступен после начала: %w", err)
				}
				if _, errMid := probe(total/2, total, streamProbeTimeoutStart); errMid != nil && !isTransientProbeError(errMid) {
					return nil, fmt.Errorf("аудиопоток недоступен после начала: %w", errMid)
				}
			}
		}
		return stream, nil
	}
}

// streamProbeTimeout — бюджет на один контрольный Range-запрос.
// start-проверка обычно мгновенна (5с достаточно); tail — медленнее (12с).
const streamProbeTimeoutStart = 5 * time.Second
const streamProbeTimeoutTail = 12 * time.Second

// isTransientProbeError сообщает, что проверка не получила ответа из-за сети
// или временной ошибки сервера (перегрузка, rate-limit). Такие ошибки
// не означают, что источник не поддерживает перемотку — трек будет работать.
func isTransientProbeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "HTTP 429") ||
		strings.Contains(msg, "HTTP 500") ||
		strings.Contains(msg, "HTTP 502") ||
		strings.Contains(msg, "HTTP 503") ||
		strings.Contains(msg, "HTTP 504")
}

// getYouTubeAudioStreamPrimary получает и проверяет форматы через kkdai/youtube.
func getYouTubeAudioStreamPrimary(ctx context.Context, videoID string) (*AudioStream, error) {
	// 2AMB is the current opaque Android player parameter recommended by the
	// library for media access. It improves format availability for some
	// Android/VPN combinations without changing the public API or playback URL.
	ytClient := youtube.Client{HTTPClient: ytHTTPClient, PlayerParams: "2AMB"}
	video, err := ytClient.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить данные видео YouTube: %w", err)
	}

	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		return nil, fmt.Errorf("подходящих аудиоформатов не нашлось")
	}

	return selectYouTubeAudioFormat(ctx, formats, func(ctx context.Context, format *youtube.Format) (*AudioStream, error) {
		streamURL, err := ytClient.GetStreamURLContext(ctx, video, format)
		if err != nil {
			return nil, err
		}
		return &AudioStream{
			URL:      streamURL,
			Format:   format.AudioQuality,
			Quality:  fmt.Sprintf("%dbps", format.Bitrate),
			Duration: int64(video.Duration.Seconds()),
		}, nil
	})
}

func selectYouTubeAudioFormat(ctx context.Context, formats []youtube.Format, resolve func(context.Context, *youtube.Format) (*AudioStream, error)) (*AudioStream, error) {
	if len(formats) == 0 {
		return nil, fmt.Errorf("подходящих аудиоформатов не нашлось")
	}
	var candidates []youtube.Format
	for i := range formats {
		if strings.HasPrefix(formats[i].MimeType, "audio/") && formats[i].Bitrate >= minimumAudioBitrate {
			candidates = append(candidates, formats[i])
		}
	}
	if len(candidates) == 0 && formats[0].Bitrate >= minimumAudioBitrate {
		candidates = append(candidates, formats[0])
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("качество аудиопотока YouTube ниже 96 кбит/с")
	}
	sort.SliceStable(candidates, func(left, right int) bool { return candidates[left].Bitrate > candidates[right].Bitrate })
	var failures []string
	for index := range candidates {
		format := &candidates[index]
		stream, err := verifyResolver(func(ctx context.Context) (*AudioStream, error) { return resolve(ctx, format) })(ctx)
		if err == nil && stream != nil && stream.URL != "" {
			return stream, nil
		}
		if err == nil {
			err = errors.New("пустая ссылка на поток")
		}
		failures = append(failures, fmt.Sprintf("формат %d: %v", format.ItagNo, err))
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("не удалось получить работающий аудиоформат YouTube (%s)", strings.Join(failures, "; "))
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
	// InnerTube (клиент ANDROID) и Piped работают на Android; yt-dlp — только на
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
	return getYtDlpStream(ctx, videoID, "", "bestaudio[abr>=96]/bestaudio")
}

// getSeekableYouTubeAudioStreamWithYtDlp получает ссылку клиентом ANDROID.
// Ссылки клиента по умолчанию (ANDROID_VR) Google отдаёт только до первого
// МиБ: Range с offset >= 1 МиБ отвечает 403, из-за чего трек начинался и
// замолкал, а перемотка не работала. ANDROID отдаёт полный файл.
func getSeekableYouTubeAudioStreamWithYtDlp(ctx context.Context, videoID string) (*AudioStream, error) {
	return getYtDlpStream(ctx, videoID, "android", "bestaudio[abr>=96]/best")
}

// getYtDlpStream получает ссылку на поток через yt-dlp с заданным клиентом
// и форматом.
func getYtDlpStream(ctx context.Context, videoID, playerClient, format string) (*AudioStream, error) {
	args := []string{
		"--no-warnings", "--no-playlist",
		"--socket-timeout", "15", "--retries", "2",
		"-f", format, "--get-url",
		"https://www.youtube.com/watch?v=" + videoID,
	}
	if playerClient != "" {
		args = append([]string{"--extractor-args", "youtube:player_client=" + playerClient}, args...)
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	hideConsole(cmd)
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
