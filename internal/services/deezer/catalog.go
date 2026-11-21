package deezer

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"Cryon2/internal/domain"
)

// CatalogTrack is metadata, never Deezer's 30-second preview.
type CatalogTrack struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Duration    int    `json:"duration"`
	ReleaseDate string `json:"release_date"`
	Artist      struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
		Cover string `json:"cover_medium"`
	} `json:"album"`
}

func (c *Client) CatalogChart(ctx context.Context, limit int) ([]CatalogTrack, error) {
	return c.catalogTracks(ctx, "/chart/0/tracks", limit)
}

func (c *Client) catalogTracks(ctx context.Context, path string, limit int) ([]CatalogTrack, error) {
	var result struct {
		Data  []CatalogTrack `json:"data"`
		Error *deezerError   `json:"error"`
	}
	if err := c.get(ctx, path, url.Values{"limit": {strconv.Itoa(limit)}}, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error.err()
	}
	return result.Data, nil
}

// Genre collections are editorial radio catalogues assigned to the genre by
// the provider, rather than arbitrary search results containing its name.
func (c *Client) GenreTracks(ctx context.Context, genreID int, limit int) ([]CatalogTrack, error) {
	var genres struct {
		Data []struct {
			ID     int `json:"id"`
			Radios []struct {
				ID int64 `json:"id"`
			} `json:"radios"`
		} `json:"data"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, "/radio/genres", nil, &genres); err != nil {
		return nil, err
	}
	if genres.Error != nil {
		return nil, genres.Error.err()
	}
	for _, genre := range genres.Data {
		if genre.ID != genreID {
			continue
		}
		var out []CatalogTrack
		for i, radio := range genre.Radios {
			if i >= 3 || len(out) >= limit {
				break
			}
			tracks, err := c.catalogTracks(ctx, fmt.Sprintf("/radio/%d/tracks", radio.ID), limit)
			if err == nil {
				out = append(out, tracks...)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("жанровая подборка недоступна")
}

func (c *Client) ArtistRecentTracks(ctx context.Context, artist string, limit int) ([]CatalogTrack, error) {
	id, ok, err := c.searchArtistID(ctx, artist)
	if err != nil || !ok {
		return nil, err
	}
	var albums struct {
		Data []struct {
			ID          int64  `json:"id"`
			ReleaseDate string `json:"release_date"`
		} `json:"data"`
		Error *deezerError `json:"error"`
	}
	if err := c.get(ctx, fmt.Sprintf("/artist/%d/albums", id), url.Values{"limit": {"100"}}, &albums); err != nil {
		return nil, err
	}
	if albums.Error != nil {
		return nil, albums.Error.err()
	}
	sort.SliceStable(albums.Data, func(i, j int) bool { return albums.Data[i].ReleaseDate > albums.Data[j].ReleaseDate })
	out := []CatalogTrack{}
	for _, album := range albums.Data {
		if len(out) >= limit {
			break
		}
		if !domain.RecentRelease(album.ReleaseDate, time.Now()) {
			continue
		}
		tracks, err := c.catalogTracks(ctx, fmt.Sprintf("/album/%d/tracks", album.ID), 2)
		if err != nil {
			continue
		}
		for _, track := range tracks {
			track.ReleaseDate = album.ReleaseDate
			out = append(out, track)
		}
	}
	return out, nil
}
