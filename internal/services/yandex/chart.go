package yandex

import (
	"Cryon2/internal/domain"
	"context"
	"encoding/json"
)

func (s *Service) ChartTracks(ctx context.Context, limit int) ([]domain.Track, error) {
	body, err := s.apiGet(ctx, apiBase+"/landing3/chart?chart=russia")
	if err != nil {
		return nil, err
	}
	var result struct {
		Result struct {
			Chart struct {
				Tracks []struct {
					Track yaTrack `json:"track"`
				} `json:"tracks"`
			} `json:"chart"`
		} `json:"result"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	out := []domain.Track{}
	for _, row := range result.Result.Chart.Tracks {
		t := row.Track.toDomain()
		if t.ID != "" && domain.IsSongDuration(t.DurationMs) {
			out = append(out, t)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
