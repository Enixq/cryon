package recommendations

import (
	"testing"

	"Cryon2/internal/domain"
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

	// Кап 2 на артиста при достатке других: a3 откладывается за b1/c1.
	got := capPerArtist(in, 4, 2)
	want := []string{"a1", "a2", "b1", "c1"}
	if !equalStrings(titles(got), want) {
		t.Fatalf("cap с запасом: got %v, want %v", titles(got), want)
	}

	// Если других не хватает на limit, добавляем отложенные сверх капа.
	got = capPerArtist(in, 5, 2)
	want = []string{"a1", "a2", "b1", "c1", "a3"}
	if !equalStrings(titles(got), want) {
		t.Fatalf("cap с добором overflow: got %v, want %v", titles(got), want)
	}

	if len(capPerArtist(in, 0, 2)) != 0 {
		t.Fatalf("limit=0 должен вернуть пусто")
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
