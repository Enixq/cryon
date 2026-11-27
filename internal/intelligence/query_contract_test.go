package intelligence

import (
	"strings"
	"testing"
)

func TestContextToolsAcceptEmptyModelQuery(t *testing.T) {
	for _, ch := range []Channel{{"cryon", "discovery"}, {"cryon", "taste_neighbors"}, {"cryon", "library"}, {"lastfm", "similar_tracks"}, {"yandex", "similar_tracks"}} {
		in, limits := fixture()
		in.Channels = []ChannelBudget{{Channel: ch, MaxCandidates: 20}}
		p := SearchPlan{Scope: in.Scope, Strategy: Strategy{Exploration: .5, SeedAdherence: .8, Diversity: .5}, Steps: []SearchStep{{Channel: ch, Query: "", Limit: 10}}}
		if err := ValidateSearch(in, p, limits); err != nil {
			t.Fatalf("%v: %v", ch, err)
		}
		canonical := CanonicalSearch(p)
		if canonical.Steps[0].Query != "context" || p.Steps[0].Query != "" || canonical.Strategy != p.Strategy || canonical.Steps[0].Limit != 10 {
			t.Fatal("strategy mutated", p, canonical)
		}
	}
}

func TestCatalogQueriesRemainBounded(t *testing.T) {
	in, limits := fixture()
	ch := Channel{"cryon", "catalog_search"}
	in.Channels = []ChannelBudget{{Channel: ch, MaxCandidates: 20}}
	for _, q := range []string{"", strings.Repeat("я", 121)} {
		p := CanonicalSearch(SearchPlan{Scope: in.Scope, Steps: []SearchStep{{Channel: ch, Query: q, Limit: 10}}})
		err := ValidateSearch(in, p, limits)
		if err == nil || !strings.Contains(err.Error(), "cryon/catalog_search") {
			t.Fatal("query must remain invalid and identify its channel", err)
		}
	}
}
