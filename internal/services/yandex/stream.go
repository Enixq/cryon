package yandex

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// downloadInfoSalt — соль для подписи download-info. Значение общеизвестно
// и используется всеми клиентами Yandex Music (см. MarshalX/yandex-music-api).
const downloadInfoSalt = "XGRlBW9FXlekgbPrRHuSiA"

// StreamInfo — разрешённый прямой mp3-поток трека.
type StreamInfo struct {
	URL        string
	MimeType   string
	DurationMs int
}

// ResolveStream разрешает прямой mp3-поток трека по его id. trackRef имеет
// форму "trackID" или "trackID:albumID" (второй вариант приходит из поиска).
func (s *Service) ResolveStream(ctx context.Context, trackRef string) (*StreamInfo, error) {
	if s.currentToken() == "" {
		return nil, fmt.Errorf("yandex: нет токена, прямой поток недоступен")
	}
	trackID := trackRef
	if idx := strings.Index(trackRef, ":"); idx != -1 {
		trackID = trackRef[:idx]
	}
	if trackID == "" {
		return nil, fmt.Errorf("yandex: пустой id трека")
	}

	infos, err := s.fetchDownloadInfo(ctx, trackID)
	if err != nil {
		return nil, err
	}

	// Выбираем mp3 с максимальным битрейтом.
	var best *downloadInfo
	for i := range infos {
		if infos[i].Codec != "mp3" {
			continue
		}
		if best == nil || infos[i].BitrateKbps > best.BitrateKbps {
			best = &infos[i]
		}
	}
	if best == nil && len(infos) > 0 {
		best = &infos[0]
	}
	if best == nil {
		return nil, fmt.Errorf("yandex: у трека нет доступных потоков")
	}

	streamURL, err := s.resolveDownloadURL(ctx, best.DownloadInfoURL)
	if err != nil {
		return nil, err
	}
	return &StreamInfo{URL: streamURL, MimeType: "audio/mpeg"}, nil
}

type downloadInfo struct {
	Codec           string `json:"codec"`
	BitrateKbps     int    `json:"bitrateInKbps"`
	DownloadInfoURL string `json:"downloadInfoUrl"`
}

// fetchDownloadInfo получает список вариантов загрузки для трека.
func (s *Service) fetchDownloadInfo(ctx context.Context, trackID string) ([]downloadInfo, error) {
	endpoint := fmt.Sprintf("%s/tracks/%s/download-info", apiBase, url.PathEscape(trackID))
	body, err := s.apiGet(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result []downloadInfo `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("yandex: не удалось разобрать download-info: %w", err)
	}
	return resp.Result, nil
}

// xmlDownloadInfo — ответ downloadInfoUrl с данными для сборки ссылки.
type xmlDownloadInfo struct {
	XMLName xml.Name `xml:"download-info"`
	Host    string   `xml:"host"`
	Path    string   `xml:"path"`
	TS      string   `xml:"ts"`
	Region  string   `xml:"region"`
	S       string   `xml:"s"`
}

// resolveDownloadURL обращается к downloadInfoUrl, получает XML с host/path/ts/s
// и собирает финальную ссылку на mp3 с MD5-подписью.
func (s *Service) resolveDownloadURL(ctx context.Context, downloadInfoURL string) (string, error) {
	body, err := s.apiGet(ctx, downloadInfoURL)
	if err != nil {
		return "", err
	}

	var info xmlDownloadInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("yandex: не удалось разобрать XML потока: %w", err)
	}
	if info.Path == "" || info.Host == "" {
		return "", fmt.Errorf("yandex: неполные данные потока")
	}

	// Подпись: md5(salt + path[1:] + s).
	signInput := downloadInfoSalt + strings.TrimPrefix(info.Path, "/") + info.S
	sign := fmt.Sprintf("%x", md5.Sum([]byte(signInput)))

	streamURL := fmt.Sprintf(
		"https://%s/get-mp3/%s/%s%s",
		info.Host, sign, info.TS, info.Path,
	)
	return streamURL, nil
}
