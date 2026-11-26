package intelligence

import (
	"sort"
	"strings"
)

// PersonalSearch starts from the existing graph engine, never an artist-name
// catalog search. Graph channels resolve their own playable catalog candidates.
func PersonalSearch(in PlanningInput) SearchPlan {
	p := SearchPlan{Scope: in.Scope, Strategy: Strategy{Exploration: .7, Diversity: .7, SeedAdherence: .8}, Steps: []SearchStep{}}
	remaining := 40
	if in.Session.Mode == RadioInfinite {
		for _, c := range in.Channels {
			if c.Channel.Capability == "taste_neighbors" {
				n := min(20, c.MaxCandidates)
				p.Steps = append(p.Steps, SearchStep{Channel: c.Channel, Query: "favorite track neighbors", Limit: n})
				remaining -= n
			}
		}
	}
	for _, c := range in.Channels {
		if in.Session.Mode == RadioTrack && c.Channel.Capability == "similar_tracks" && remaining > 12 && len(p.Steps) < 4 {
			n := min(12, c.MaxCandidates)
			p.Steps = append(p.Steps, SearchStep{Channel: c.Channel, Query: "seed", Limit: n})
			remaining -= n
		}
	}
	for _, c := range in.Channels {
		if c.Channel.Provider == "cryon" && c.Channel.Capability == "discovery" {
			p.Steps = append(p.Steps, SearchStep{Channel: c.Channel, Query: "personal music graph", Limit: min(remaining, c.MaxCandidates)})
			break
		}
	}
	return p
}

// PersonalSelection is an explicit Go admission decision, independent of an API.
// Discovery provenance supplies weak graph evidence, not acoustic certainty.
func PersonalSelection(in SelectionInput) SelectionPlan {
	p := SelectionPlan{Scope: in.Context.Scope, PoolID: in.PoolID, Ranked: []CandidateDecision{}, Avoid: []string{}, Directions: []Direction{}}
	for _, c := range in.Candidates {
		ev := c.Evidence
		if ev.MetadataQuality < .65 || ev.RecentlyPlayed || ev.RecentlySuggested {
			continue
		}
		blocked := false
		affinity := ev.TasteSimilarity
		for _, artist := range c.Track.Artists {
			for _, b := range in.Context.Taste.BlockedArtists {
				if strings.EqualFold(strings.TrimSpace(artist), strings.TrimSpace(b)) {
					blocked = true
				}
			}
			for _, pref := range in.Context.Taste.Preferences {
				if strings.EqualFold(artist, pref.Name) {
					affinity = max(affinity, pref.Weight*pref.Confidence)
				}
			}
		}
		graph := false
		for _, ch := range c.Channels {
			if ch.Capability == "discovery" || ch.Capability == "similar_tracks" || ch.Capability == "taste_neighbors" {
				graph = true
			}
		}
		if blocked || (!graph && affinity < .15) {
			continue
		}
		score := .55 + .2*affinity + .1*ev.MetadataQuality
		if graph {
			score += .1
		}
		if c.UnfamiliarArtist {
			score += .04
		}
		if in.Context.Session.Mode == RadioTrack {
			seedGraph := false
			for _, ch := range c.Channels {
				if ch.Capability == "similar_tracks" {
					seedGraph = true
				}
			}
			if ev.SeedKnown && ev.SeedSimilarity < .08 {
				continue
			}
			if !ev.SeedKnown && !seedGraph {
				continue
			}
			if seedGraph {
				score = max(score, .86)
			} else {
				score = .65 + .3*ev.SeedSimilarity
			}
		}
		if score >= .65 {
			p.Ranked = append(p.Ranked, CandidateDecision{ID: c.ID, Score: min(.99, score), Reason: "Персональный профиль и музыкальные связи; оценка Cryon"})
		}
	}
	sort.SliceStable(p.Ranked, func(i, j int) bool { return p.Ranked[i].Score > p.Ranked[j].Score })
	return p
}
