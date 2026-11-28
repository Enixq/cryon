package recommendations

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
	"Cryon2/internal/intelligence"
)

// RankingWeights are local heuristic weights, not probabilities or audio features.
// They can be configured in intelligence.ranking.weights; invalid values fall back.
type RankingWeights struct {
	Taste    float64 `json:"taste"`
	Artist   float64 `json:"artist"`
	Session  float64 `json:"session"`
	Graph    float64 `json:"graph"`
	Novelty  float64 `json:"novelty"`
	Metadata float64 `json:"metadata"`
	Repeat   float64 `json:"repeat"`
	MMR      float64 `json:"mmr"`
}

func DefaultRankingWeights() RankingWeights {
	return RankingWeights{.24, .18, .12, .30, .08, .08, .25, .18}
}
func (w RankingWeights) Valid() bool {
	for _, v := range []float64{w.Taste, w.Artist, w.Session, w.Graph, w.Novelty, w.Metadata, w.Repeat, w.MMR} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return false
		}
	}
	return w.Taste+w.Artist+w.Session+w.Graph+w.Metadata > 0
}

func (e *Engine) rankingWeights(ctx context.Context) RankingWeights {
	defaults := DefaultRankingWeights()
	if e.store == nil {
		return defaults
	}
	raw, _, _ := e.store.SettingGet(ctx, "intelligence.ranking.weights")
	w := defaults
	if json.Unmarshal([]byte(raw), &w) != nil {
		return defaults
	}
	for _, v := range []float64{w.Taste, w.Artist, w.Session, w.Graph, w.Novelty, w.Metadata, w.Repeat, w.MMR} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return defaults
		}
	}
	if w.Taste+w.Artist+w.Session+w.Graph+w.Metadata == 0 {
		return defaults
	}
	return w
}

// RankDJCandidates is the single musical admission point. A candidate found by
// text search needs independent evidence; matching its title never adds relevance.
// MMR uses maximum similarity to the selected set, not cumulative order penalties.
func (e *Engine) RankDJCandidates(ctx context.Context, in intelligence.SelectionInput, pool []domain.Track, limit int) (intelligence.SelectionPlan, []domain.Track) {
	out := intelligence.SelectionPlan{Scope: in.Context.Scope, PoolID: in.PoolID, Ranked: []intelligence.CandidateDecision{}, Avoid: []string{}, Directions: []intelligence.Direction{}}
	w := e.rankingWeights(ctx)
	type item struct {
		index  int
		score  float64
		reason string
	}
	items := []item{}
	for i, c := range in.Candidates {
		if i >= len(pool) {
			break
		}
		ev := c.Evidence
		blocked := false
		for _, artist := range c.Track.Artists {
			for _, name := range in.Context.Taste.BlockedArtists {
				if identity.Normalize(artist) == identity.Normalize(name) {
					blocked = true
				}
			}
		}
		if blocked {
			continue
		}
		ev.SessionAffinity = e.sessionTasteAffinity(ctx, c, in.Context.Session.ID)
		if ev.MetadataQuality < .65 || ev.IdentityConfidence < .5 || ev.Feedback <= -1 {
			continue
		}
		graph := ev.GraphAffinity
		for _, ch := range c.Channels {
			if ch.Capability == "similar_tracks" {
				graph = math.Max(graph, .85)
			}
			if ch.Capability == "taste_neighbors" || ch.Capability == "history_neighbors" {
				graph = math.Max(graph, .7)
			}
			if ch.Capability == "discovery" {
				graph = math.Max(graph, .55)
			}
		}
		if in.Context.Session.Mode == intelligence.RadioInfinite {
			anchorAffinity := 0.0
			knownAnchor := false
			for _, origin := range c.Origins {
				if len(origin.AnchorArtists) > 0 {
					knownAnchor = true
					anchorAffinity = math.Max(anchorAffinity, artistAffinity(origin.AnchorArtists, in.Context.Taste))
				}
			}
			if knownAnchor {
				graph *= .3 + .7*anchorAffinity
			}
		}
		ev.GraphAffinity = graph
		in.Candidates[i].Evidence = ev
		relevance := math.Max(ev.TasteSimilarity, math.Max(ev.ArtistAffinity, graph))
		seed := ev.SeedSimilarity
		if in.Context.Session.Mode == intelligence.RadioTrack {
			if ev.SeedKnown && seed < .08 {
				continue
			}
			linked := false
			for _, ch := range c.Channels {
				if ch.Capability == "similar_tracks" {
					linked = true
				}
			}
			// Genre overlap alone is weaker than a recording-level graph edge.
			if !linked && (!ev.SeedKnown || seed < .25) {
				continue
			}
			if linked {
				seed = math.Max(seed, .8)
			}
			relevance = seed
		}
		if relevance < .15 {
			continue
		}
		novelty := .25
		if c.UnfamiliarArtist {
			novelty = 1
		}
		score := w.Taste*ev.TasteSimilarity + w.Artist*ev.ArtistAffinity + w.Session*ev.SessionAffinity + w.Graph*graph + w.Metadata*ev.MetadataQuality + w.Novelty*in.Strategy.Exploration*novelty
		score += .08 * math.Max(-1, math.Min(1, ev.Feedback))
		if in.Context.Session.Mode == intelligence.RadioTrack {
			adherence := math.Max(.7, in.Strategy.SeedAdherence)
			score = adherence*seed + (1-adherence)*score
		}
		if ev.RecentlyPlayed {
			score -= w.Repeat
		}
		if ev.RecentlySuggested {
			score -= w.Repeat * .7
		}
		if score < .12 {
			continue
		}
		reason := fmt.Sprintf("Cryon: вкус %.2f · артист %.2f · связи %.2f · сессия %.2f", ev.TasteSimilarity, ev.ArtistAffinity, graph, ev.SessionAffinity)
		items = append(items, item{i, score, reason})
	}
	selected := []int{}
	counts := map[string]int{}
	for len(items) > 0 && len(selected) < limit {
		best := -1
		bestScore := -math.MaxFloat64
		for j, it := range items {
			artist := artistKey(pool[it.index])
			if counts[artist] >= 2 {
				continue
			}
			redundancy := 0.0
			for _, index := range selected {
				sim := cosine(djTagVector([]string{pool[it.index].Genre}), djTagVector([]string{pool[index].Genre}))
				if artist != "" && artist == artistKey(pool[index]) {
					sim = 1
				}
				redundancy = math.Max(redundancy, sim)
			}
			value := it.score - w.MMR*in.Strategy.Diversity*redundancy
			if value > bestScore {
				bestScore = value
				best = j
			}
		}
		if best < 0 {
			break
		}
		it := items[best]
		items = append(items[:best], items[best+1:]...)
		selected = append(selected, it.index)
		counts[artistKey(pool[it.index])]++
		out.Ranked = append(out.Ranked, intelligence.CandidateDecision{ID: in.Candidates[it.index].ID, Score: math.Max(0, math.Min(1, it.score)), Reason: it.reason})
	}
	tracks := []domain.Track{}
	for i, index := range selected {
		t := pool[index]
		t.RecommendationReason = "Cryon DJ · " + out.Ranked[i].Reason
		tracks = append(tracks, t)
	}
	return out, e.PrepareDJPool(ctx, tracks, nil, limit)
}

func artistAffinity(artists []string, taste intelligence.TasteProfile) float64 {
	score := 0.0
	for _, a := range artists {
		for _, p := range taste.Preferences {
			if identity.Normalize(a) == identity.Normalize(p.Name) {
				score = math.Max(score, p.Weight*(.5+.5*p.Confidence))
			}
		}
	}
	return score
}

func (e *Engine) feedbackFor(ctx context.Context, t domain.Track) float64 {
	fb := e.loadFeedback(ctx)
	if fb.Tracks[trackKey(t)] < 0 {
		return -1
	}
	if fb.Tracks[trackKey(t)] > 0 {
		return 1
	}
	raw, _, _ := e.storeSetting(ctx, "intelligence.private.reactions")
	var rows []struct {
		Event intelligence.FeedbackEvent `json:"event"`
		Track intelligence.TrackSummary  `json:"track"`
	}
	_ = json.Unmarshal([]byte(raw), &rows)
	score := 0.0
	w := e.behaviorWeights(ctx)
	for _, r := range rows {
		if trackKey(domain.Track{Title: r.Track.Title, Artists: r.Track.Artists}) != trackKey(t) {
			continue
		}
		age := math.Max(0, time.Since(r.Event.At).Hours())
		score += w.Signal(r.Event) / 3 * math.Exp2(-age/(w.HalfLifeDays*24))
	}
	// Persistent exclusions belong to the explicit library feedback above;
	// historical reactions must not silently recreate an exclusion after undo.
	return math.Max(-.9, math.Min(1, score))
}
func (e *Engine) storeSetting(ctx context.Context, key string) (string, bool, error) {
	if e.store == nil {
		return "", false, nil
	}
	return e.store.SettingGet(ctx, strings.TrimSpace(key))
}

func (e *Engine) sessionTasteAffinity(ctx context.Context, c intelligence.Candidate, session string) float64 {
	raw, _, _ := e.storeSetting(ctx, "intelligence.private.reactions")
	var rows []struct {
		Event intelligence.FeedbackEvent `json:"event"`
		Track intelligence.TrackSummary  `json:"track"`
	}
	_ = json.Unmarshal([]byte(raw), &rows)
	total, matched := 0.0, 0.0
	w := e.behaviorWeights(ctx)
	for _, r := range rows {
		if time.Since(r.Event.At) > 2*time.Hour || (r.Event.SessionID != "" && r.Event.SessionID != session) {
			continue
		}
		weight := math.Max(0, w.Signal(r.Event))
		total += weight
		similarity := cosine(djTagVector(c.Track.Tags), djTagVector(r.Track.Tags))
		for _, a := range c.Track.Artists {
			for _, b := range r.Track.Artists {
				if identity.Normalize(a) == identity.Normalize(b) {
					similarity = 1
				}
			}
		}
		matched += weight * similarity
	}
	if total == 0 {
		return 0
	}
	return matched / total
}
