package yandex

import (
	"Cryon2/internal/services/lyrics"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Service) TrackLyrics(ctx context.Context, id string) (lyrics.Result, error) {
	if !s.HasToken() {
		return lyrics.Result{}, nil
	}
	id = strings.SplitN(id, ":", 2)[0]
	for _, format := range []string{"LRC", "TEXT"} {
		if result, err := s.modernLyrics(ctx, id, format); err == nil && result.Found {
			return result, nil
		}
		if ctx.Err() != nil {
			return lyrics.Result{}, ctx.Err()
		}
	}
	body, err := s.apiGet(ctx, apiBase+"/tracks/"+url.PathEscape(id)+"/supplement")
	if err != nil {
		return lyrics.Result{}, err
	}
	var result struct {
		Result struct {
			Lyrics struct {
				FullLyrics string `json:"fullLyrics"`
			} `json:"lyrics"`
		} `json:"result"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return lyrics.Result{}, err
	}
	text := strings.TrimSpace(result.Result.Lyrics.FullLyrics)
	return lyrics.Result{Plain: text, Found: text != "", Provider: "Яндекс Музыка"}, nil
}

func (s *Service) modernLyrics(ctx context.Context, id, format string) (lyrics.Result, error) {
	stamp := fmt.Sprint(time.Now().Unix())
	// Public mobile-client signing key used by the upstream Yandex API adapter.
	mac := hmac.New(sha256.New, []byte("p93jhgh689SBReK6ghtw62"))
	_, _ = mac.Write([]byte(id + stamp))
	query := url.Values{"format": {format}, "timeStamp": {stamp}, "sign": {base64.StdEncoding.EncodeToString(mac.Sum(nil))}}
	body, err := s.apiGet(ctx, apiBase+"/tracks/"+url.PathEscape(id)+"/lyrics?"+query.Encode())
	if err != nil {
		return lyrics.Result{}, err
	}
	var payload struct {
		Result struct {
			DownloadURL string `json:"downloadUrl"`
		} `json:"result"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return lyrics.Result{}, err
	}
	target, err := url.Parse(payload.Result.DownloadURL)
	if err != nil || target == nil || target.Scheme != "https" || target.Host == "" {
		return lyrics.Result{}, fmt.Errorf("ссылка на текст недоступна")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return lyrics.Result{}, err
	}
	response, err := s.httpClient.Do(req)
	if err != nil {
		return lyrics.Result{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return lyrics.Result{}, fmt.Errorf("текст недоступен: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 512<<10))
	if err != nil {
		return lyrics.Result{}, err
	}
	text := strings.TrimSpace(string(data))
	result := lyrics.Result{Found: text != "", Provider: "Яндекс Музыка"}
	if format == "LRC" {
		result.Synced = text
	} else {
		result.Plain = text
	}
	return result, nil
}
