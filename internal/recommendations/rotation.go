package recommendations

import (
	"Cryon2/internal/domain"
	"context"
	"crypto/sha256"
	"encoding/json"
	"sort"
	"time"
)

func trackArtistBlocked(t domain.Track, blocked map[string]bool) bool {
	for _, artist := range t.Artists {
		if blocked[feedbackArtistKey(artist)] {
			return true
		}
	}
	return false
}

// Rotate uses durable, cross-service exposure history. Recent suggestions lose
// priority, but remain a last resort when the catalog has no fresh candidates.
// Call only on baseline output, never after an AI has ranked its candidates.
func (e *Engine) Rotate(ctx context.Context, tracks []domain.Track, limit int) []domain.Track {
	if len(tracks) == 0 {
		return []domain.Track{}
	}
	tracks = e.Allowed(ctx, tracks)
	e.feedbackMu.Lock()
	defer e.feedbackMu.Unlock()
	seen := map[string]int64{}
	if e.store != nil {
		raw, _, _ := e.store.SettingGet(ctx, "reco.exposures")
		_ = json.Unmarshal([]byte(raw), &seen)
	}
	if seen == nil {
		seen = map[string]int64{}
	}
	now := time.Now()
	cutoff := now.Add(-7 * 24 * time.Hour).UnixMilli()
	for k, v := range seen {
		if v < now.Add(-30*24*time.Hour).UnixMilli() {
			delete(seen, k)
		}
	}
	day := now.Format("2006-01-02")
	sort.SliceStable(tracks, func(i, j int) bool {
		a, b := seen[trackKey(tracks[i])], seen[trackKey(tracks[j])]
		if a < cutoff {
			a = 0
		}
		if b < cutoff {
			b = 0
		}
		if a != b {
			return a < b
		}
		x, y := sha256.Sum256([]byte(day+trackKey(tracks[i]))), sha256.Sum256([]byte(day+trackKey(tracks[j])))
		return string(x[:]) < string(y[:])
	})
	tracks = diversifyTracks(tracks, nil, max(1, limit), artistCap)
	for _, t := range tracks {
		seen[trackKey(t)] = now.UnixMilli()
	}
	// Bound the local ledger even for very large libraries.
	if len(seen) > 5000 {
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return seen[keys[i]] > seen[keys[j]] })
		for _, k := range keys[5000:] {
			delete(seen, k)
		}
	}
	if e.store != nil {
		raw, _ := json.Marshal(seen)
		_ = e.store.SettingSet(context.WithoutCancel(ctx), "reco.exposures", string(raw))
	}
	return tracks
}
