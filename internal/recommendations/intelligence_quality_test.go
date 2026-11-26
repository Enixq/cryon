package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
	"Cryon2/internal/intelligence"
	"context"
	"encoding/json"
	"math"
	"testing"
)

func TestDJReactionsRespectSignalStrengthAndTechnicalFailures(t *testing.T) {
	st := newFakeSettings()
	e := New(st, nil, nil, nil)
	rows := []struct {
		Event intelligence.FeedbackEvent `json:"event"`
		Track intelligence.TrackSummary  `json:"track"`
	}{}
	for _, kind := range []intelligence.FeedbackKind{intelligence.FeedbackPlaybackError, intelligence.FeedbackSkip, intelligence.FeedbackCompleted, intelligence.FeedbackReplay, intelligence.FeedbackLike, intelligence.FeedbackDislike} {
		rows = append(rows, struct {
			Event intelligence.FeedbackEvent `json:"event"`
			Track intelligence.TrackSummary  `json:"track"`
		}{intelligence.FeedbackEvent{Kind: kind, DurationMS: 100, PlayedMS: 90}, intelligence.TrackSummary{Artists: []string{string(kind)}}})
	}
	raw, _ := json.Marshal(rows)
	st.values["intelligence.private.reactions"] = string(raw)
	weights := e.djReactionWeights(context.Background())
	for kind, want := range map[string]float64{"playback_error": 0, "skip": 0, "completed": .4, "replay": 1.5, "like": 3, "dislike": -3} {
		if math.Abs(weights[kind]-want) > 1e-6 {
			t.Fatal(kind, weights)
		}
	}
}

func TestDJQualityRejectsFillerAndSeedDrift(t *testing.T) {
	e := New(nil, nil, nil, nil)
	pool := []domain.Track{}
	candidates := []intelligence.Candidate{}
	decisions := []intelligence.CandidateDecision{}
	for _, id := range []string{"good", "unselected", "weak", "wrong-style", "played", "incomplete", "unknown", "unknown-strong"} {
		pool = append(pool, domain.Track{ID: id, Title: id, Artists: []string{id}, Service: domain.ServiceSoundCloud})
		candidates = append(candidates, intelligence.Candidate{ID: id, Evidence: intelligence.Evidence{MetadataQuality: 1, SeedKnown: true, SeedSimilarity: 1}})
		if id != "unselected" {
			decisions = append(decisions, intelligence.CandidateDecision{ID: id, Score: .9})
		}
	}
	decisions[1].Score = .64
	candidates[3].Evidence.SeedSimilarity = 0
	candidates[4].Evidence.RecentlyPlayed = true
	candidates[5].Evidence.MetadataQuality = .35
	candidates[6].Evidence.SeedKnown = false
	decisions[5].Score = .8
	candidates[7].Evidence.SeedKnown = false
	got := e.SelectDJ(context.Background(), pool, candidates, intelligence.SelectionPlan{Ranked: decisions}, intelligence.Strategy{SeedAdherence: 0}, intelligence.RadioTrack, 20)
	if len(got) != 2 || got[0].ID != "good" || got[1].ID != "unknown-strong" {
		t.Fatal("filler or seed drift", got)
	}
}

func TestDJStrategyChangesNoveltyAndArtistCaps(t *testing.T) {
	e := New(nil, nil, nil, nil)
	pool := []domain.Track{{ID: "a", Title: "A", Artists: []string{"Familiar"}}, {ID: "b", Title: "B", Artists: []string{"New"}}, {ID: "c", Title: "C", Artists: []string{"Familiar"}}}
	cs := []intelligence.Candidate{{ID: "a", Evidence: intelligence.Evidence{MetadataQuality: 1}}, {ID: "b", UnfamiliarArtist: true, Evidence: intelligence.Evidence{MetadataQuality: 1}}, {ID: "c", Evidence: intelligence.Evidence{MetadataQuality: 1}}}
	plan := intelligence.SelectionPlan{Ranked: []intelligence.CandidateDecision{{ID: "a", Score: .9}, {ID: "b", Score: .9}, {ID: "c", Score: .9}}}
	plain := e.SelectDJ(context.Background(), pool, cs, plan, intelligence.Strategy{}, intelligence.RadioInfinite, 20)
	varied := e.SelectDJ(context.Background(), pool, cs, plan, intelligence.Strategy{Exploration: 1, Diversity: 1}, intelligence.RadioInfinite, 20)
	if len(plain) != 3 || plain[0].ID != "a" || len(varied) != 2 || varied[0].ID != "b" {
		t.Fatal(plain, varied)
	}
}

func TestDJEvidenceAndVersionIdentity(t *testing.T) {
	e := New(nil, nil, nil, nil)
	seed := domain.Track{Title: "Anchor", Artists: []string{"Seed"}, Genre: "rock"}
	pool := []domain.Track{{Title: "Compatible", Artists: []string{"New"}, Genre: "rock"}, {Title: "Unrelated", Artists: []string{"Pop"}, Genre: "pop"}, {Title: "Unknown", Artists: []string{"Other"}}}
	ev := e.DJEvidence(context.Background(), pool, seed, intelligence.TasteProfile{}, nil)
	if !ev[0].SeedKnown || ev[0].SeedSimilarity != 1 || !ev[1].SeedKnown || ev[1].SeedSimilarity != 0 || ev[2].SeedKnown {
		t.Fatal(ev)
	}
	versions := []domain.Track{}
	for _, title := range []string{"Song", "Song (Live)", "Song (Remix)", "Song (Clean)", "Song (Explicit)", "Song (Remaster)"} {
		versions = append(versions, domain.Track{Title: title, Artists: []string{"Artist"}, DurationMs: 200000})
	}
	versions = append(versions, domain.Track{Title: "Song", Artists: []string{"Artist"}, DurationMs: 201000})
	if got := e.FinalizeDJ(context.Background(), versions, nil, 20); len(got) != 2 {
		t.Fatal("artist cap changed", got)
	}
	groups := identity.GroupTracks(versions)
	if len(groups) != 6 || len(groups[0].Candidates) != 2 {
		t.Fatal("versions merged or duplicates retained", groups)
	}
}
