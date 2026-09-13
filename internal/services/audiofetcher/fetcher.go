package audiofetcher

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

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
var ytClient = youtube.Client{}

const minimumAudioBitrate = 96000

// GetYouTubeAudioStream извлекает прямой аудиопоток видео YouTube
// по его ID. Работает без API-ключа через библиотеку kkdai/youtube.
func GetYouTubeAudioStream(ctx context.Context, videoID string) (*AudioStream, error) {
	video, err := ytClient.GetVideoContext(ctx, videoID)
	if err != nil {
		if fallback, fallbackErr := getYouTubeAudioStreamWithYtDlp(ctx, videoID); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("?? ??????? ???????? ????? YouTube: %w", err)
	}

	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		if fallback, fallbackErr := getYouTubeAudioStreamWithYtDlp(ctx, videoID); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("???????????? ?? ???????")
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
		if fallback, fallbackErr := getYouTubeAudioStreamWithYtDlp(ctx, videoID); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("YouTube audio stream quality is below 96 kbps")
	}

	streamURL, err := ytClient.GetStreamURLContext(ctx, video, &format)
	if err != nil {
		if fallback, fallbackErr := getYouTubeAudioStreamWithYtDlp(ctx, videoID); fallbackErr == nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("?? ??????? ???????? URL ??????: %w", err)
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
