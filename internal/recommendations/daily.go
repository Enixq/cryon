package recommendations

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"Cryon2/internal/domain"
)

// A daily mix includes familiar music. Listening to it must not drain its cache.
func (e *Engine) dailyMix(ctx context.Context, limit int) ([]domain.Track, error) {
	profile := e.buildTaste(ctx)
	seeds := profile.artists
	if len(seeds) > 10 {
		seeds = seeds[:10]
	}
	names := []string{}
	for _, seed := range seeds {
		names = append(names, strings.ToLower(seed.name))
	}
	sort.Strings(names)
	key := fmt.Sprintf("daily:v4:%d:%d:%d:%s", periodSeed(MixDaily), profile.feedbackRev, limit, strings.Join(names, "|"))
	var pool []domain.Track
	if e.store != nil {
		if raw, ok, _ := e.store.RecoCacheGet(ctx, key); ok {
			_ = json.Unmarshal([]byte(raw), &pool)
			pool = filterRecommended(pool, profile)
			if len(pool) >= limit {
				return pool[:limit], nil
			}
		}
	}
	// Clear listening-history exclusions, while retaining explicit dislikes.
	mixProfile := profile
	mixProfile.seen = profile.blockedTracks
	if e.OnlineAvailable() && len(seeds) > 0 {
		pool = append(pool, e.onlineRecommendations(ctx, mixProfile, seeds, limit*2)...)
	}
	if len(pool) < limit {
		pool = append(pool, e.offlineRecommendations(ctx, mixProfile, seeds, limit*2)...)
	}
	if e.favFn != nil {
		tracks, _ := e.favFn(ctx)
		pool = append(pool, tracks...)
	}
	if e.histFn != nil {
		rows, _ := e.histFn(ctx, 120)
		for _, row := range rows {
			pool = append(pool, row.Track)
		}
	}
	if e.localFn != nil {
		rows, _ := e.localFn(ctx)
		for _, row := range rows {
			pool = append(pool, domain.Track{ID: row.ID, Service: domain.ServiceLocal, Title: row.Title, Artists: row.Artists, PlayableKind: domain.PlayableStream})
		}
	}
	if len(pool) == 0 {
		pool = e.chartFallbackRecommendations(ctx, mixProfile, limit)
	}
	known := map[string]bool{}
	unique := make([]domain.Track, 0, len(pool))
	for _, t := range filterRecommended(pool, profile) {
		k := trackKey(t)
		if !known[k] && domain.IsSongDuration(t.DurationMs) {
			known[k] = true
			unique = append(unique, t)
		}
	}
	shuffled := deterministicShuffle(unique, periodSeed(MixDaily))
	result := diversifyTracks(shuffled, nil, limit, 4)
	// Small libraries can have only one or two artists. Prefer diversity, but
	// do not turn an otherwise complete daily mix into a four-song playlist.
	if len(result) < limit {
		selected := map[string]bool{}
		for _, track := range result {
			selected[trackKey(track)] = true
		}
		for _, track := range shuffled {
			if !selected[trackKey(track)] {
				result = append(result, track)
				if len(result) >= limit {
					break
				}
			}
		}
	}
	if len(result) > 0 && e.store != nil {
		data, _ := json.Marshal(result)
		_ = e.store.RecoCacheSet(ctx, key, string(data), 24*time.Hour)
	}
	return result, nil
}
