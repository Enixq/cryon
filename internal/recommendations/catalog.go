package recommendations

import (
	"context"
	"fmt"
	"sync"

	"Cryon2/internal/domain"
	"Cryon2/internal/services/deezer"
)

func (e *Engine) ResolveCatalog(ctx context.Context, metadata []deezer.CatalogTrack, limit int) []domain.Track {
	out := []domain.Track{}
	known := map[string]bool{}
	profile := e.buildTaste(ctx)
	for start := 0; start < len(metadata) && len(out) < limit && ctx.Err() == nil; start += searchConcurrency {
		end := min(start+searchConcurrency, len(metadata))
		results := make([]domain.Track, end-start)
		var wg sync.WaitGroup
		for i, meta := range metadata[start:end] {
			if !domain.IsSongDuration(meta.Duration * 1000) {
				continue
			}
			wg.Add(1)
			go func(i int, meta deezer.CatalogTrack) {
				defer wg.Done()
				found, err := e.search(ctx, meta.Artist.Name+" "+meta.Title)
				if err != nil {
					return
				}
				matching := []domain.Track{}
				for _, t := range found {
					if domain.SameRecording(t, meta.Artist.Name, meta.Title) && t.PlayableKind == domain.PlayableStream && domain.IsSongDuration(t.DurationMs) && (meta.Duration == 0 || t.DurationMs == 0 || abs(t.DurationMs-meta.Duration*1000) <= 15000) {
						matching = append(matching, t)
					}
				}
				best, ok := e.pickCandidateFor(matching, profile.sources)
				if !ok {
					return
				}
				if meta.ReleaseDate != "" {
					best.ReleaseDate = meta.ReleaseDate
				}
				if best.Album == "" {
					best.Album = meta.Album.Title
				}
				if best.ArtworkURL == "" {
					best.ArtworkURL = meta.Album.Cover
				}
				results[i] = best
			}(i, meta)
		}
		wg.Wait()
		for _, t := range results {
			k := trackKey(t)
			if t.ID != "" && !known[k] && !profile.blockedTracks[k] {
				known[k] = true
				out = append(out, t)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (e *Engine) CatalogCollection(ctx context.Context, kind string, limit int) ([]domain.Track, error) {
	if e.deezer == nil {
		return nil, fmt.Errorf("каталог недоступен")
	}
	var metadata []deezer.CatalogTrack
	var err error
	if kind == "world-chart" {
		metadata, err = e.deezer.CatalogChart(ctx, limit*2)
	} else {
		ids := map[string]int{"pop": 132, "rock": 152, "hip-hop": 116, "electronic": 106, "indie": 85, "jazz": 129, "classical": 98, "rnb": 165, "dance": 113, "metal": 464}
		id, ok := ids[kind]
		if !ok {
			return nil, fmt.Errorf("неизвестная подборка")
		}
		metadata, err = e.deezer.GenreTracks(ctx, id, limit*2)
	}
	if err != nil {
		return nil, err
	}
	return e.ResolveCatalog(ctx, metadata, limit), nil
}

func (e *Engine) RecentCatalogReleases(ctx context.Context, seeds []string, limit int) []domain.Track {
	if e.deezer == nil {
		return nil
	}
	metadata := []deezer.CatalogTrack{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, artist := range seeds {
		wg.Add(1)
		go func(artist string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tracks, _ := e.deezer.ArtistRecentTracks(ctx, artist, 4)
			mu.Lock()
			metadata = append(metadata, tracks...)
			mu.Unlock()
		}(artist)
	}
	wg.Wait()
	return e.ResolveCatalog(ctx, metadata, limit)
}
