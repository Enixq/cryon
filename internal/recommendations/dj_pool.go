package recommendations

import (
	"context"
	"sync"

	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
)

// PrepareDJPool enforces identity, blocks and playback shape only. Musical
// preference, recent history and artist diversity belong to the final selector.
func (e *Engine) PrepareDJPool(ctx context.Context, tracks, exclude []domain.Track, limit int) []domain.Track {
	excluded := map[string]bool{}
	for _, t := range exclude {
		excluded[trackKey(t)] = true
	}
	out := []domain.Track{}
	for _, group := range identity.GroupTracks(e.Allowed(ctx, tracks)) {
		prefs := identity.Preferences{}
		if e.djPlaybackPreferences != nil {
			prefs = e.djPlaybackPreferences()
		}
		for _, c := range identity.RankCandidates(group.Candidates, prefs) {
			t := c.Track
			if excluded[trackKey(t)] || t.Service == domain.ServiceYouTube || (t.PlayableKind != "" && t.PlayableKind != domain.PlayableStream) {
				continue
			}
			out = append(out, t)
			break
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

// Resolution is identity matching, not a second recommendation engine. Keep
// familiar artists, played tracks and multiple tracks by the same artist so DJ
// can evaluate them with the complete current context.
func (e *Engine) resolveDJMetadata(ctx context.Context, metadata []scoredCand, limit int) []domain.Track {
	if e.search == nil {
		return nil
	}
	if len(metadata) > 40 {
		metadata = metadata[:40]
	}
	out := []domain.Track{}
	for start := 0; start < len(metadata) && ctx.Err() == nil && len(out) < limit; start += searchConcurrency {
		end := min(len(metadata), start+searchConcurrency)
		rows := make([][]domain.Track, end-start)
		var wg sync.WaitGroup
		for i, c := range metadata[start:end] {
			wg.Add(1)
			go func(i int, c scoredCand) {
				defer wg.Done()
				found, _ := e.search(ctx, c.artist+" "+c.title)
				for _, t := range found {
					if domain.SameRecording(t, c.artist, c.title) {
						rows[i] = append(rows[i], t)
					}
				}
			}(i, c)
		}
		wg.Wait()
		for _, row := range rows {
			out = append(out, row...)
		}
	}
	return e.PrepareDJPool(ctx, out, nil, limit)
}

func (e *Engine) discoveryDJPool(ctx context.Context, limit int) []domain.Track {
	if !e.OnlineAvailable() {
		return nil
	}
	profile := e.buildTaste(ctx)
	seeds := profile.artists
	if len(seeds) > discoverySeedCap {
		seeds = seeds[:discoverySeedCap]
	}
	if len(seeds) == 0 {
		return nil
	}
	return e.resolveDJMetadata(ctx, e.discoveryCandidatesFor(ctx, profile, seeds, true), limit)
}
