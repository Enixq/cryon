package recommendations

import (
	"Cryon2/internal/domain"
	"context"
	"sort"
	"sync"
	"time"
)

func (e *Engine) historyNeighbors(ctx context.Context, limit int) []domain.Track {
	if e.histFn == nil || e.trackSimilar == nil {
		return nil
	}
	rows, err := e.histFn(ctx, 2000)
	if err != nil {
		return nil
	}
	counts := map[string]float64{}
	tracks := map[string]domain.Track{}
	for _, r := range rows {
		key := trackKey(r.Track)
		age := time.Since(time.UnixMilli(r.PlayedAtMs)).Hours() / 24
		if age < 0 {
			age = 0
		}
		counts[key] += 1 / (1 + age/30)
		tracks[key] = r.Track
	}
	keys := []string{}
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	if len(keys) > 30 {
		keys = keys[:30]
	}
	anchors := []domain.Track{}
	for _, key := range keys {
		anchors = append(anchors, tracks[key])
	}
	anchors = djAnchors(ctx, anchors, 3)
	keys = keys[:0]
	for _, t := range anchors {
		keys = append(keys, trackKey(t))
	}
	result := make([][]domain.Track, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			child, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			result[i], _ = e.trackSimilar(child, tracks[key])
			for _, t := range result[i] {
				recordOrigin(ctx, t, tracks[key])
			}
		}(i, key)
	}
	wg.Wait()
	out := []domain.Track{}
	for _, row := range result {
		out = append(out, row...)
	}
	return e.PrepareDJPool(ctx, out, nil, limit)
}
