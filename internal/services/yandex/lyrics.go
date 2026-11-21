package yandex

import (
	"Cryon2/internal/services/lyrics"
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

func (s *Service) TrackLyrics(ctx context.Context, id string) (lyrics.Result, error) {
	if !s.HasToken() {
		return lyrics.Result{}, nil
	}
	id = strings.SplitN(id, ":", 2)[0]
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
