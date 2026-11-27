package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"context"
	"testing"
)

func TestFavoriteNeighborsUseSimilarityWithoutCatalogSearch(t *testing.T) {
	e := New(nil, nil, nil, func(context.Context, string) ([]domain.Track, error) {
		t.Error("generic catalog search used")
		return nil, nil
	})
	e.favFn = func(context.Context) ([]domain.Track, error) {
		return []domain.Track{{ID: "seed", Title: "Favorite", Artists: []string{"Favorite artist"}, Service: domain.ServiceYandex}}, nil
	}
	e.SetTrackSimilarity(func(_ context.Context, seed domain.Track) ([]domain.Track, error) {
		if seed.ID != "seed" {
			t.Error(seed)
		}
		return []domain.Track{seed, {ID: "new", Title: "Linked track", Artists: []string{"New artist"}, Service: domain.ServiceYandex, DurationMs: 180000}}, nil
	})
	rows, err := e.ExecuteDJStep(context.Background(), intelligence.SearchStep{Channel: intelligence.Channel{Provider: "cryon", Capability: "taste_neighbors"}, Limit: 20}, domain.Track{})
	if err != nil || len(rows) != 2 || rows[1].ID != "new" {
		t.Fatal(rows, err)
	}
	rows = rows[1:]
	evidence := e.DJEvidence(context.Background(), rows, domain.Track{}, e.IntelligenceTaste(context.Background()), nil)
	in := intelligence.SelectionInput{Context: intelligence.PlanningInput{Session: intelligence.SessionProfile{Mode: intelligence.RadioInfinite}}, Candidates: []intelligence.Candidate{{ID: "c0", Track: intelligence.TrackSummary{ID: "new"}, Evidence: evidence[0], Channels: []intelligence.Channel{{Provider: "cryon", Capability: "taste_neighbors"}}}}}
	plan := intelligence.PersonalSelection(in)
	result := e.SelectDJ(context.Background(), rows, in.Candidates, plan, intelligence.Strategy{Diversity: .7}, intelligence.RadioInfinite, 20)
	if len(result) != 1 || result[0].ID != "new" {
		t.Fatal("linked recording lost during final admission", plan, result)
	}
}

func TestDiscoveryNeverLabelsNameSearchAsGraph(t *testing.T) {
	e := New(nil, nil, nil, func(context.Context, string) ([]domain.Track, error) { t.Error("name search called"); return nil, nil })
	rows, err := e.ExecuteDJStep(context.Background(), intelligence.SearchStep{Channel: intelligence.Channel{Provider: "cryon", Capability: "discovery"}, Limit: 20}, domain.Track{})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
