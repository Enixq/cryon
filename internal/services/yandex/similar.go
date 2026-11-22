package yandex

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"Cryon2/internal/domain"
)

// Provider song similarity gives track radio a closer seed than artist charts.
func (s *Service) SimilarTracks(ctx context.Context, seed domain.Track) ([]domain.Track, error) {
	if !s.HasToken() {
		return nil, nil
	}
	id := seed.ID
	if seed.Service != domain.ServiceYandex {
		artist := ""
		if len(seed.Artists) > 0 {
			artist = seed.Artists[0]
		}
		tracks, err := s.apiSearch(ctx, artist+" "+seed.Title)
		if err != nil {
			return nil, err
		}
		id = ""
		for _, track := range tracks {
			if domain.SameRecording(track, artist, seed.Title) {
				id = track.ID
				break
			}
		}
		if id == "" {
			return nil, nil
		}
	}
	id = strings.SplitN(id, ":", 2)[0]
	body, err := s.apiGet(ctx, apiBase+"/tracks/"+url.PathEscape(id)+"/similar")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Result struct {
			SimilarTracks []yaTrack `json:"similarTracks"`
		} `json:"result"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := []domain.Track{}
	for _, track := range payload.Result.SimilarTracks {
		item := track.toDomain()
		if item.PlayableKind == domain.PlayableStream && domain.IsSongDuration(item.DurationMs) {
			out = append(out, item)
		}
	}
	return out, nil
}
