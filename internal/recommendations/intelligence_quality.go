package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
	"Cryon2/internal/intelligence"
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

// DJEvidence uses the existing TF-IDF/cosine implementation on known metadata.
// Missing tags remain unknown, never invented acoustic facts.
func (e *Engine) DJEvidence(ctx context.Context, pool []domain.Track, seed domain.Track, taste intelligence.TasteProfile, recent []intelligence.TrackSummary) []intelligence.Evidence {
	vectors := []tagVector{}

	sv := djTagVector([]string{seed.Genre})
	tv := tagVector{}
	for _, tag := range taste.Tags {
		for name := range djTagVector([]string{tag.Name}) {
			tv[name] += tag.Weight
		}
	}
	vectors = append(vectors, sv, tv)
	for _, t := range pool {
		vectors = append(vectors, djTagVector([]string{t.Genre}))
	}
	idf := buildIDF(vectors)
	sv = applyIDF(sv, idf)
	tv = applyIDF(tv, idf)
	played := map[string]bool{}
	if e.histFn != nil {
		if rows, err := e.histFn(ctx, 120); err == nil {
			for _, r := range rows {
				played[trackKey(r.Track)] = true
			}
		}
	}
	suggested := map[string]bool{}
	for _, t := range recent {
		suggested[trackKey(domain.Track{Title: t.Title, Artists: t.Artists})] = true
	}
	out := make([]intelligence.Evidence, len(pool))
	for i, t := range pool {
		quality := 0.0
		if strings.TrimSpace(t.Title) != "" {
			quality += .35
		}
		if len(t.Artists) > 0 && strings.TrimSpace(t.Artists[0]) != "" {
			quality += .35
		}
		if t.DurationMs > 0 {
			quality += .2
		}
		if t.Genre != "" {
			quality += .1
		}
		cv := applyIDF(vectors[i+2], idf)
		sim := cosine(sv, cv)
		known := len(sv) > 0 && len(cv) > 0
		for _, a := range seed.Artists {
			for _, b := range t.Artists {
				if !known && identity.Normalize(a) == identity.Normalize(b) && identity.Normalize(a) != "" {
					sim = .6
					known = true
				}
			}
		}
		version := "unknown"
		if gs := identity.GroupTracks([]domain.Track{t}); len(gs) > 0 && len(gs[0].Candidates) > 0 {
			version = string(gs[0].Candidates[0].Version)
		}
		out[i] = intelligence.Evidence{TasteSimilarity: cosine(tv, cv), SeedSimilarity: sim, SeedKnown: known, MetadataQuality: quality, RecentlyPlayed: played[trackKey(t)], RecentlySuggested: suggested[trackKey(t)], Version: version}
	}
	return out
}

// SelectDJ admits only explicitly selected IDs. Scores are heuristics, not
// probabilities. Unknown seed metadata requires stronger model agreement.
func (e *Engine) SelectDJ(ctx context.Context, pool []domain.Track, candidates []intelligence.Candidate, plan intelligence.SelectionPlan, strategy intelligence.Strategy, mode intelligence.RadioMode, limit int) []domain.Track {
	if mode == intelligence.RadioTrack {
		strategy.SeedAdherence = math.Max(.7, strategy.SeedAdherence)
	}
	type ranked struct {
		track domain.Track
		score float64
	}
	accepted := []ranked{}
	avoid := map[string]bool{}
	for _, id := range plan.Avoid {
		avoid[id] = true
	}
	for _, decision := range plan.Ranked {
		if avoid[decision.ID] || decision.Score < .65 {
			continue
		}
		for i, c := range candidates {
			if c.ID != decision.ID || i >= len(pool) {
				continue
			}
			ev := c.Evidence
			if ev.MetadataQuality < .65 || ev.RecentlyPlayed {
				continue
			}
			if mode == intelligence.RadioTrack && ((ev.SeedKnown && ev.SeedSimilarity < .08 && strategy.SeedAdherence >= .5) || (!ev.SeedKnown && decision.Score < .85)) {
				continue
			}
			novelty := 1.0
			if !c.UnfamiliarArtist {
				novelty = .3
			}
			if ev.RecentlySuggested {
				novelty = 0
			}
			relevance := ev.TasteSimilarity
			if mode == intelligence.RadioTrack {
				relevance = (1-strategy.SeedAdherence)*relevance + strategy.SeedAdherence*ev.SeedSimilarity
				if !ev.SeedKnown {
					relevance = decision.Score * .5
				}
			}
			score := .65*decision.Score + .2*relevance + .1*strategy.Exploration*novelty + .05*ev.MetadataQuality
			if ev.RecentlySuggested {
				score -= .12
			}
			track := pool[i]
			track.RecommendationReason = "Cryon DJ · оценка модели: " + decision.Reason
			accepted = append(accepted, ranked{track, score})
		}
	}
	// Greedy diversity/transition penalty. Never invent a quality score for an unselected track.
	out := []domain.Track{}
	counts := map[string]int{}
	maxPerArtist := 2
	if strategy.Diversity >= .7 {
		maxPerArtist = 1
	}
	for len(accepted) > 0 && len(out) < limit {
		sort.SliceStable(accepted, func(i, j int) bool { return accepted[i].score > accepted[j].score })
		best := accepted[0]
		accepted = accepted[1:]
		artist := artistKey(best.track)
		if counts[artist] >= maxPerArtist {
			continue
		}
		out = append(out, best.track)
		counts[artist]++
		for i := range accepted {
			// Smooth metadata transitions only when both sides have known genres.
			left, right := djTagVector([]string{best.track.Genre}), djTagVector([]string{accepted[i].track.Genre})
			if len(left) > 0 && len(right) > 0 {
				accepted[i].score -= .04 * (1 - cosine(left, right))
			}
			if artistKey(accepted[i].track) == artist {
				accepted[i].score -= .2 * strategy.Diversity
			}
		}
	}
	return e.FinalizeDJ(ctx, out, nil, limit)
}

func (e *Engine) enrichDJTaste(ctx context.Context, out *intelligence.TasteProfile) {
	profile := e.buildTaste(ctx)
	out.Preferences = []intelligence.ArtistPreference{}
	out.Positive = []intelligence.TrackSummary{}
	out.Negative = []intelligence.TrackSummary{}
	out.BlockedArtists = []string{}
	peak := 1.0
	for _, a := range profile.artists {
		peak = math.Max(peak, a.weight)
	}
	for _, a := range profile.artists {
		if len(out.Preferences) == 20 {
			break
		}
		confidence := math.Min(1, float64(a.plays)/8)
		if a.deliberate {
			confidence = math.Max(confidence, .8)
		}
		out.Preferences = append(out.Preferences, intelligence.ArtistPreference{Name: a.name, Weight: a.weight / peak, Confidence: confidence, LastPlayedMS: a.lastPlayed})
	}
	summary := func(t domain.Track) intelligence.TrackSummary {
		tags := []string{}
		if t.Genre != "" {
			tags = append(tags, t.Genre)
		}
		return intelligence.TrackSummary{ID: string(t.Service) + ":" + t.ID, Title: t.Title, Artists: t.Artists, Tags: tags, DurationMS: t.DurationMs}
	}
	if e.favFn != nil {
		if tracks, err := e.favFn(ctx); err == nil {
			for _, t := range tracks {
				if len(out.Positive) == 12 {
					break
				}
				out.Positive = append(out.Positive, summary(t))
			}
		}
	}
	feedback := e.loadFeedback(ctx)
	for artist := range profile.blockedArtists {
		out.BlockedArtists = append(out.BlockedArtists, artist)
	}
	sort.Strings(out.BlockedArtists)
	if e.histFn != nil {
		if rows, err := e.histFn(ctx, 120); err == nil {
			for _, r := range rows {
				if feedback.Tracks[trackKey(r.Track)] < 0 && len(out.Negative) < 12 {
					out.Negative = append(out.Negative, summary(r.Track))
				}
			}
		}
	}
}

// Behavioral deltas are bounded, decay with a 30-day half-life, and
// never infer dislike from a transport failure or one situational skip.
func (e *Engine) djReactionWeights(ctx context.Context) map[string]float64 {
	out := map[string]float64{}
	if e.store == nil {
		return out
	}
	raw, _, _ := e.store.SettingGet(ctx, "intelligence.private.reactions")
	var rows []struct {
		Event intelligence.FeedbackEvent `json:"event"`
		Track intelligence.TrackSummary  `json:"track"`
	}
	_ = json.Unmarshal([]byte(raw), &rows)
	for _, r := range rows {
		weight := 0.0
		switch r.Event.Kind {
		case intelligence.FeedbackLike:
			weight = 3
		case intelligence.FeedbackDislike:
			weight = -3
		case intelligence.FeedbackReplay:
			weight = 1.5
		case intelligence.FeedbackCompleted:
			if r.Event.DurationMS > 0 && r.Event.PlayedMS*100 >= r.Event.DurationMS*80 {
				weight = .4
			}
		}
		if !r.Event.At.IsZero() {
			weight *= math.Exp2(-math.Max(0, time.Since(r.Event.At).Hours()) / (30 * 24))
		}
		for _, artist := range r.Track.Artists {
			key := strings.ToLower(strings.TrimSpace(artist))
			out[key] = math.Max(-6, math.Min(6, out[key]+weight))
		}
	}
	return out
}

// Keep compound tags and their words so alternative rock can overlap rock.
// This is lexical metadata similarity, not acoustic analysis.
func djTagVector(tags []string) tagVector {
	v := tagVector{}
	for _, tag := range tags {
		for _, part := range strings.FieldsFunc(strings.ToLower(tag), func(r rune) bool { return r == ',' || r == ';' || r == '/' }) {
			normalized := identity.Normalize(part)
			switch normalized {
			case "hiphop", "хип хоп", "рэп", "rap":
				normalized = "hip hop"
			case "рок":
				normalized = "rock"
			case "поп":
				normalized = "pop"
			}
			if normalized == "" {
				continue
			}
			v[normalized]++
			words := strings.Fields(normalized)
			if len(words) > 1 {
				for _, word := range words {
					v[word]++
				}
			}
		}
	}
	return v
}
