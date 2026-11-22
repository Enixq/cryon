package core

import (
	"Cryon2/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Apple's regional song sales chart is a metadata fallback, not a search pretending to be a chart.
func russianAppleChart(ctx context.Context) ([]domain.Track, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://itunes.apple.com/ru/rss/topsongs/limit=100/json", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("российский чарт: HTTP %d", resp.StatusCode)
	}
	type label struct {
		Label string `json:"label"`
	}
	var payload struct {
		Feed struct {
			Entry []struct {
				Title  label   `json:"im:name"`
				Artist label   `json:"im:artist"`
				Image  []label `json:"im:image"`
			} `json:"entry"`
		} `json:"feed"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	out := []domain.Track{}
	for _, row := range payload.Feed.Entry {
		if row.Title.Label == "" || row.Artist.Label == "" {
			continue
		}
		item := domain.Track{Title: row.Title.Label, Artists: []string{row.Artist.Label}}
		if len(row.Image) > 0 {
			item.ArtworkURL = row.Image[len(row.Image)-1].Label
		}
		out = append(out, item)
	}
	return out, nil
}
