package recommendations

import (
	"context"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

func tr(artist, title string, svc domain.ServiceID) domain.Track {
	return domain.Track{Service: svc, Title: title, Artists: []string{artist}}
}

func titles(ts []domain.Track) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Title
	}
	return out
}

func TestCapPerArtist(t *testing.T) {
	in := []domain.Track{
		tr("A", "a1", domain.ServiceSoundCloud),
		tr("A", "a2", domain.ServiceSoundCloud),
		tr("A", "a3", domain.ServiceSoundCloud),
		tr("B", "b1", domain.ServiceSoundCloud),
		tr("C", "c1", domain.ServiceSoundCloud),
	}

	// Сначала разные исполнители, затем второй трек того же артиста.
	got := capPerArtist(in, 4, 2)
	want := []string{"a1", "b1", "c1", "a2"}
	if !equalStrings(titles(got), want) {
		t.Fatalf("cap с запасом: got %v, want %v", titles(got), want)
	}

	// При недостатке разнообразия подборка короче, а не заполнена одним артистом.
	got = capPerArtist(in, 5, 2)
	want = []string{"a1", "b1", "c1", "a2"}
	if !equalStrings(titles(got), want) {
		t.Fatalf("cap с добором overflow: got %v, want %v", titles(got), want)
	}

	if len(capPerArtist(in, 0, 2)) != 0 {
		t.Fatalf("limit=0 должен вернуть пусто")
	}
}

type cachedGraph struct {
	store.Store
	values map[string]string
}

func (s *cachedGraph) RecoCacheGet(_ context.Context, key string) (string, bool, error) {
	value, ok := s.values[key]
	return value, ok, nil
}

func TestOnlineRecommendationsExploreSecondHop(t *testing.T) {
	graph := &cachedGraph{values: map[string]string{
		"graph:similar:v1:seed":        `[{"name":"Bridge","match":0.9}]`,
		"graph:similar:v1:bridge":      `[{"name":"Discovery","match":0.8}]`,
		"graph:toptracks:v1:bridge":    `[{"artist":"Bridge","name":"First"}]`,
		"graph:toptracks:v1:discovery": `[{"artist":"Discovery","name":"Second"}]`,
	}}
	e := &Engine{store: graph, search: func(_ context.Context, query string) ([]domain.Track, error) {
		switch query {
		case "Bridge First":
			return []domain.Track{tr("Bridge", "First", domain.ServiceSoundCloud)}, nil
		case "Discovery Second":
			return []domain.Track{tr("Discovery", "Second", domain.ServiceSoundCloud)}, nil
		default:
			return nil, nil
		}
	}}
	got := e.onlineRecommendations(context.Background(), taste{}, []artistWeight{{name: "Seed", weight: 1}}, 10)
	if !equalStrings(titles(got), []string{"First", "Second"}) {
		t.Fatalf("second hop missing: %v", titles(got))
	}
}

func TestMergeGraphCandidatesCombinesIndependentSeeds(t *testing.T) {
	got := mergeGraphCandidates([]scoredCand{{artist: "Artist", title: "Song", score: 0.4}, {artist: " artist ", title: "SONG", score: 0.6}, {artist: "Other", title: "Song", score: 0.3}})
	if len(got) != 2 || got[0].score != 1.0 || got[1].artist != "Other" {
		t.Fatalf("unexpected merged candidates: %+v", got)
	}
}

func TestPickCandidate(t *testing.T) {
	e := &Engine{}

	// YT из рекомендаций исключён полностью — выбираем первый не-YT.
	found := []domain.Track{
		tr("A", "yt", domain.ServiceYouTube),
		tr("A", "sc", domain.ServiceSoundCloud),
	}
	best, ok := e.pickCandidate(found)
	if !ok || best.Title != "sc" {
		t.Fatalf("ожидали не-YT кандидата sc, got %v ok=%v", best.Title, ok)
	}

	// Только YT — кандидата нет: в рекомендациях YouTube Music не участвует.
	onlyYT := []domain.Track{tr("A", "yt1", domain.ServiceYouTube)}
	if _, ok := e.pickCandidate(onlyYT); ok {
		t.Fatalf("из одних YT-треков кандидата быть не должно")
	}

	if _, ok := e.pickCandidate(nil); ok {
		t.Fatalf("пустой вход должен вернуть ok=false")
	}
}

func TestOrderCandidates(t *testing.T) {
	e := &Engine{}
	found := []domain.Track{
		tr("A", "yt1", domain.ServiceYouTube),
		tr("A", "sc1", domain.ServiceSoundCloud),
		tr("A", "yt2", domain.ServiceYouTube),
		tr("A", "sp1", domain.ServiceSpotify),
	}
	got := titles(e.orderCandidates(found))
	// YT-треки исключаются полностью, порядок остальных сохраняется.
	want := []string{"sc1", "sp1"}
	if !equalStrings(got, want) {
		t.Fatalf("YT должен исключаться с сохранением порядка: got %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
