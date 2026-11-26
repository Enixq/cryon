package intelligence

import "testing"

func TestCatalogCoveragePreservesQueriesAndBudgets(t *testing.T) {
	in := PlanningInput{Scope: Scope{RequestID: "r", SessionID: "s"}, Session: SessionProfile{ID: "s", Mode: RadioInfinite}}
	for _, provider := range []string{"cryon", "yandex", "vk"} {
		in.Channels = append(in.Channels, ChannelBudget{Channel: Channel{provider, "catalog_search"}, MaxCandidates: 20})
	}
	in.Channels = append(in.Channels, ChannelBudget{Channel: Channel{"cryon", "discovery"}, MaxCandidates: 20})
	original := SearchPlan{Scope: in.Scope, Steps: []SearchStep{{Channel: in.Channels[0].Channel, Query: "anchor", Limit: 20}, {Channel: in.Channels[3].Channel, Query: "direction", Limit: 20}}}
	limits := Limits{MaxSteps: 5, MaxCandidates: 40, MaxQueryRunes: 120, MaxReasonRunes: 160}
	got := EnsureCatalogCoverage(in, original, limits)
	if err := ValidateSearch(in, got, limits); err != nil {
		t.Fatal(err, got)
	}
	if len(got.Steps) != 4 || got.Steps[0].Query != "anchor" || got.Steps[1].Query != "direction" || original.Steps[0].Limit != 20 {
		t.Fatal("coverage lost model intent or mutated original", got, original)
	}
}
