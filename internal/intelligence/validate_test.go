package intelligence

import (
	"math"
	"testing"
)

func fixture() (PlanningInput, Limits) {
	return PlanningInput{
		Scope:    Scope{RequestID: "request", SessionID: "session", SessionRevision: 2, TasteRevision: 3, ConfigRevision: 1},
		Session:  SessionProfile{ID: "session", Revision: 2, Mode: RadioTrack, Seed: &TrackSummary{ID: "seed"}},
		Taste:    TasteProfile{Revision: 3},
		Channels: []ChannelBudget{{Channel: Channel{Provider: "lastfm", Capability: "similar_tracks"}, MaxCandidates: 20}},
	}, Limits{MaxSteps: 3, MaxCandidates: 40, MaxDirections: 3, MaxQueryRunes: 120, MaxReasonRunes: 160}
}

func TestSearchTrustBoundary(t *testing.T) {
	for _, name := range []string{"valid", "unknown_channel", "aggregate_budget", "stale", "nonfinite", "missing_seed", "global_budget"} {
		t.Run(name, func(t *testing.T) {
			in, limits := fixture()
			plan := SearchPlan{Scope: in.Scope, Strategy: Strategy{Exploration: .2, SeedAdherence: .8, Diversity: .5}, Steps: []SearchStep{{Channel: in.Channels[0].Channel, Query: "seed", Limit: 15}}}
			switch name {
			case "unknown_channel":
				plan.Steps[0].Channel.Provider = "invented"
			case "aggregate_budget":
				plan.Steps = append(plan.Steps, plan.Steps[0])
			case "stale":
				plan.Scope.SessionRevision++
			case "nonfinite":
				plan.Strategy.Exploration = math.NaN()
			case "missing_seed":
				in.Session.Seed = nil
			case "global_budget":
				limits.MaxCandidates = 10
			}
			err := ValidateSearch(in, plan, limits)
			if (err == nil) != (name == "valid") {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}

func TestSelectionTrustBoundary(t *testing.T) {
	for _, name := range []string{"valid_partial", "foreign_id", "duplicate", "avoid_overlap", "stale_pool", "changed_key", "ascending", "infinite"} {
		t.Run(name, func(t *testing.T) {
			ctx, limits := fixture()
			in := SelectionInput{Context: ctx, PoolID: "pool", Candidates: []Candidate{{ID: "a"}, {ID: "b"}}}
			plan := SelectionPlan{Scope: ctx.Scope, PoolID: in.PoolID, Ranked: []CandidateDecision{{ID: "a", Score: .8, Reason: "Подходит seed"}}}
			switch name {
			case "foreign_id":
				plan.Ranked[0].ID = "invented"
			case "duplicate":
				plan.Ranked = append(plan.Ranked, plan.Ranked[0])
			case "avoid_overlap":
				plan.Avoid = []string{"a"}
			case "stale_pool":
				plan.PoolID = "old"
			case "changed_key":
				plan.Scope.ConfigRevision++
			case "ascending":
				plan.Ranked = append(plan.Ranked, CandidateDecision{ID: "b", Score: .9, Reason: "other"})
			case "infinite":
				plan.Ranked[0].Score = math.Inf(1)
			}
			err := ValidateSelection(in, plan, limits)
			if (err == nil) != (name == "valid_partial") {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}
