package audiofetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// InnerTube-плеер — бесключевой путь получения аудиопотока напрямую из
// youtubei/v1/player, независимый от kkdai/youtube. Используем клиент IOS:
// он отдаёт прямые URL потоков без сигнатурного шифра (расшифровка n/sig не
// нужна), на момент написания не требует PoToken и работает на Android (чистый
// net/http, без yt-dlp). Ключ — публичный статический ключ клиента YouTube,
// не секрет и не привязан к аккаунту (см. пояснение в youtube/innertube.go).
//
// Этот путь — дополнительный слой в цепочке фолбэков (kkdai → InnerTube →
// Piped → yt-dlp): он выполняется, только когда основной путь уже не сработал,
// поэтому не может ухудшить работающее воспроизведение.
const (
	innertubeIOSKey           = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	innertubeIOSClientName    = "IOS"
	innertubeIOSClientVersion = "19.45.4"
	innertubeIOSDeviceModel   = "iPhone16,2"
	innertubeIOSUserAgent     = "com.google.ios.youtube/19.45.4 (iPhone16,2; U; CPU iOS 18_1_0 like Mac OS X;)"
)

// innertubeHTTPClient — отдельный HTTP-клиент InnerTube-плеера с конечными
// фазовыми таймаутами (под медленный VPN, но без бесконечного зависания).
var innertubeHTTPClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   12 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

type innertubePlayerRequest struct {
	Context        innertubePlayerContext `json:"context"`
	VideoID        string                 `json:"videoId"`
	ContentCheckOK bool                   `json:"contentCheckOk"`
	RacyCheckOK    bool                   `json:"racyCheckOk"`
}

type innertubePlayerContext struct {
	Client innertubePlayerClient `json:"client"`
}

type innertubePlayerClient struct {
	ClientName    string `json:"clientName"`
	ClientVersion string `json:"clientVersion"`
	DeviceModel   string `json:"deviceModel,omitempty"`
	HL            string `json:"hl"`
	GL            string `json:"gl"`
}

type innertubePlayerResponse struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"playabilityStatus"`
	StreamingData struct {
		AdaptiveFormats []innertubeFormat `json:"adaptiveFormats"`
		Formats         []innertubeFormat `json:"formats"`
	} `json:"streamingData"`
	VideoDetails struct {
		LengthSeconds string `json:"lengthSeconds"`
	} `json:"videoDetails"`
}

type innertubeFormat struct {
	URL          string `json:"url"`
	MimeType     string `json:"mimeType"`
	Bitrate      int    `json:"bitrate"`
	AudioQuality string `json:"audioQuality"`
}

func getYouTubeAudioStreamWithInnerTube(ctx context.Context, videoID string) (*AudioStream, error) {
	reqBody := innertubePlayerRequest{
		Context: innertubePlayerContext{
			Client: innertubePlayerClient{
				ClientName:    innertubeIOSClientName,
				ClientVersion: innertubeIOSClientVersion,
				DeviceModel:   innertubeIOSDeviceModel,
				HL:            "en",
				GL:            "US",
			},
		},
		VideoID:        videoID,
		ContentCheckOK: true,
		RacyCheckOK:    true,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	endpoint := "https://www.youtube.com/youtubei/v1/player?key=" + innertubeIOSKey + "&prettyPrint=false"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", innertubeIOSUserAgent)
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("X-Youtube-Client-Name", "5")
	req.Header.Set("X-Youtube-Client-Version", innertubeIOSClientVersion)

	resp, err := innertubeHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtube innertube player returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	var parsed innertubePlayerResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if status := strings.TrimSpace(parsed.PlayabilityStatus.Status); status != "" && !strings.EqualFold(status, "OK") {
		return nil, fmt.Errorf("youtube innertube player status %s: %s", status, parsed.PlayabilityStatus.Reason)
	}

	best := pickBestInnerTubeAudio(parsed.StreamingData.AdaptiveFormats)
	if best == nil {
		best = pickBestInnerTubeAudio(parsed.StreamingData.Formats)
	}
	if best == nil || best.URL == "" {
		return nil, fmt.Errorf("youtube innertube player returned no audio stream")
	}

	duration := int64(0)
	if secs, perr := strconv.ParseInt(strings.TrimSpace(parsed.VideoDetails.LengthSeconds), 10, 64); perr == nil && secs > 0 {
		duration = secs
	}
	return &AudioStream{
		URL:      best.URL,
		Format:   best.MimeType,
		Quality:  fmt.Sprintf("%dbps", best.Bitrate),
		Duration: duration,
	}, nil
}

// pickBestInnerTubeAudio выбирает аудио-only формат с наибольшим битрейтом,
// но не ниже minimumAudioBitrate. Возвращает nil, если подходящего нет.
func pickBestInnerTubeAudio(formats []innertubeFormat) *innertubeFormat {
	var best *innertubeFormat
	for i := range formats {
		f := &formats[i]
		if f.URL == "" || !strings.HasPrefix(f.MimeType, "audio/") {
			continue
		}
		if f.Bitrate < minimumAudioBitrate {
			continue
		}
		if best == nil || f.Bitrate > best.Bitrate {
			best = f
		}
	}
	return best
}
