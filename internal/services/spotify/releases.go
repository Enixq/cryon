package spotify

import (
	"context"
	"encoding/json"
	"net/url"

	"Cryon2/internal/domain"
)

func (s *Service) recentAlbumTracks(ctx context.Context, token, albumID, date string) ([]domain.Track, error) {
	body, err := s.authGet(ctx, token, "https://api.spotify.com/v1/albums/"+url.PathEscape(albumID)+"/tracks?limit=2")
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			DurationMs int    `json:"duration_ms"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"items"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	out := []domain.Track{}
	for _, track := range result.Items {
		artists := []string{}
		for _, artist := range track.Artists {
			artists = append(artists, artist.Name)
		}
		out = append(out, domain.Track{ID: track.ID, Service: domain.ServiceSpotify, Title: track.Name, Artists: artists, DurationMs: track.DurationMs, ReleaseDate: date, PlayableKind: domain.PlayableExternal, ExternalURL: "https://open.spotify.com/track/" + track.ID})
	}
	return out, nil
}
