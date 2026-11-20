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
// youtubei/v1/player. Клиент ANDROID отдаёт прогрессивный формат (itag=18) с
// прямым URL и полным Range-доступом без расшифровки сигнатур. Работает на
// Windows и Android (чистый net/http, без yt-dlp). Ключ — публичный
// статический ключ клиента YouTube, не секрет (см. youtube/innertube.go).
const (
	innertubeAndroidKey           = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	innertubeAndroidClientName    = "ANDROID"
	innertubeAndroidClientVersion = "21.02.35"
	innertubeAndroidUserAgent     = "com.google.android.youtube/21.02.35 (Linux; U; Android 11) gzip"
)

// innertubeHTTPClient — отдельный HTTP-клиент InnerTube-плеера с конечными
// фазовыми таймаутами (под медленный VPN, но без бесконечного зависания).
var innertubeHTTPClient = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: googleDialContext(&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
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
	ClientName        string `json:"clientName"`
	ClientVersion     string `json:"clientVersion"`
	AndroidSdkVersion int    `json:"androidSdkVersion,omitempty"`
	OsName            string `json:"osName,omitempty"`
	OsVersion         string `json:"osVersion,omitempty"`
	HL                string `json:"hl"`
	GL                string `json:"gl"`
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
				ClientName:        innertubeAndroidClientName,
				ClientVersion:     innertubeAndroidClientVersion,
				AndroidSdkVersion: 30,
				OsName:            "Android",
				OsVersion:         "11",
				HL:                "en",
				GL:                "US",
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

	endpoint := "https://www.youtube.com/youtubei/v1/player?key=" + innertubeAndroidKey + "&prettyPrint=false"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", innertubeAndroidUserAgent)
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("X-Youtube-Client-Name", "3")
	req.Header.Set("X-Youtube-Client-Version", innertubeAndroidClientVersion)

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
		best = pickBestInnerTubeProgressive(parsed.StreamingData.Formats)
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

// pickBestInnerTubeProgressive выбирает прогрессивный формат (видео+аудио) с
// наибольшим битрейтом. ANDROID-клиент InnerTube отдаёт audio-only форматы
// только через sigCipher, а прогрессивные — с прямым URL (itag=18).
func pickBestInnerTubeProgressive(formats []innertubeFormat) *innertubeFormat {
	var best *innertubeFormat
	for i := range formats {
		f := &formats[i]
		if f.URL == "" {
			continue
		}
		if !strings.Contains(f.MimeType, "mp4a") {
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
