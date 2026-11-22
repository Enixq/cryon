package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/store"
	"context"
	"encoding/json"
	"sort"
	"strconv"
)

const AdventureKey = "recommendations.adventure"

func (e *Engine) Allowed(ctx context.Context, tracks []domain.Track) []domain.Track {
	t := e.buildTaste(ctx)
	out := []domain.Track{}
	for _, track := range tracks {
		if !t.blockedTracks[trackKey(track)] && !t.blockedArtists[artistKey(track)] {
			out = append(out, track)
		}
	}
	return e.Explain(ctx, out)
}

func (e *Engine) Adventure(ctx context.Context) float64 {
	if e.store != nil {
		raw, _, _ := e.store.SettingGet(ctx, AdventureKey)
		if n, err := strconv.ParseFloat(raw, 64); err == nil {
			return max(0, min(100, n)) / 100
		}
	}
	return .5
}
func familiar(t taste) map[string]bool {
	out := map[string]bool{}
	for _, a := range t.artists {
		out[feedbackArtistKey(a.name)] = true
	}
	return out
}
func adventureOrder(tracks []domain.Track, t taste, value float64) []domain.Track {
	out := append([]domain.Track(nil), tracks...)
	known := familiar(t)
	scores := map[string]float64{}
	for i, track := range out {
		score := 1.0 / float64(i+1)
		if !known[artistKey(track)] {
			score *= .2 + 1.6*value
		} else {
			score *= 1.8 - 1.6*value
		}
		scores[trackKey(track)] = score
	}
	sort.SliceStable(out, func(i, j int) bool { return scores[trackKey(out[i])] > scores[trackKey(out[j])] })
	return out
}
func (e *Engine) Explain(ctx context.Context, tracks []domain.Track) []domain.Track {
	if e == nil || len(tracks) == 0 {
		return tracks
	}
	t := e.buildTaste(ctx)
	known := familiar(t)
	out := append([]domain.Track(nil), tracks...)
	for i := range out {
		if out[i].RecommendationReason == "" {
			if known[artistKey(out[i])] {
				out[i].RecommendationReason = "Исполнитель из вашей библиотеки и истории"
			} else {
				out[i].RecommendationReason = "Новый для вашей библиотеки исполнитель"
			}
		}
	}
	return out
}
func (e *Engine) staleTracks(ctx context.Context, key string) []domain.Track {
	raw, ok := store.PeekCache(context.WithoutCancel(ctx), e.store, key)
	if !ok {
		return nil
	}
	var tracks []domain.Track
	_ = json.Unmarshal([]byte(raw), &tracks)
	for i := range tracks {
		tracks[i].FromCache = true
	}
	return tracks
}

type Metrics struct {
	Count           int     `json:"count"`
	ArtistDiversity float64 `json:"artistDiversity"`
	ArtistNovelty   float64 `json:"artistNovelty"`
}

func metrics(tracks []domain.Track, t taste) Metrics {
	result := Metrics{Count: len(tracks)}
	if len(tracks) == 0 {
		return result
	}
	artists := map[string]bool{}
	known := familiar(t)
	novel := 0
	for _, track := range tracks {
		key := artistKey(track)
		if key != "" {
			artists[key] = true
			if !known[key] {
				novel++
			}
		}
	}
	result.ArtistDiversity = float64(len(artists)) / float64(len(tracks))
	result.ArtistNovelty = float64(novel) / float64(len(tracks))
	return result
}

// Compare uses one candidate pool and a deterministic split, rather than claiming an unmeasured improvement.
func (e *Engine) Compare(ctx context.Context, tracks []domain.Track) map[string]any {
	t := e.buildTaste(ctx)
	a := adventureOrder(tracks, t, .2)
	b := adventureOrder(tracks, t, .8)
	n := min(20, len(tracks))
	a = a[:n]
	b = b[:n]
	return map[string]any{"familiar": metrics(a, t), "adventurous": metrics(b, t), "familiarTracks": e.Explain(ctx, a), "adventurousTracks": e.Explain(ctx, b)}
}
