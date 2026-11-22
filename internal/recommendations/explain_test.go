package recommendations

import (
	"Cryon2/internal/domain"
	"testing"
)

func TestAdventureChangesRankingAndMetricsUseLibraryArtists(t *testing.T) {
	profile := taste{artists: []artistWeight{{name: "Known"}}}
	pool := []domain.Track{tr("Known", "first", domain.ServiceYouTube), tr("New", "second", domain.ServiceYouTube)}
	a := adventureOrder(pool, profile, 0)
	b := adventureOrder(pool, profile, 1)
	if a[0].Artists[0] != "Known" || b[0].Artists[0] != "New" {
		t.Fatal("slider does not change ranking")
	}
	got := metrics(pool, profile)
	if got.ArtistDiversity != 1 || got.ArtistNovelty != .5 || got.Count != 2 {
		t.Fatal(got)
	}
	if pool[0].Artists[0] != "Known" {
		t.Fatal("comparison mutated original candidates")
	}
}

func TestShortCachedReasons(t *testing.T) {
	for before, after := range map[string]string{
		"Похож по графу исполнителей на Ai Mori":    "Похож на Ai Mori",
		"Исполнитель из вашей библиотеки и истории": "Знакомый исполнитель",
		"Новый для вашей библиотеки исполнитель":    "Новый исполнитель",
		"Из чарта России":                           "Из чарта России",
	} {
		if got := shortReason(before); got != after {
			t.Errorf("got %q want %q", got, after)
		}
	}
}
