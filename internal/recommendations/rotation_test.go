package recommendations

import (
	"Cryon2/internal/domain"
	"context"
	"fmt"
	"testing"
)

func TestArtistBlockSurvivesRestartAndCoversCollaborations(t *testing.T) {
	ctx := context.Background()
	st := newFakeSettings()
	e := &Engine{store: st}
	if _, err := e.SetFeedback(ctx, "  ARTIST  ", "*", -1); err != nil {
		t.Fatal(err)
	}
	restarted := &Engine{store: st}
	collab := tr("Other", "Collaboration", domain.ServiceSoundCloud)
	collab.Artists = append(collab.Artists, "Artist")
	tracks := []domain.Track{tr("Artist", "New release", domain.ServiceYandex), collab, tr("Allowed", "Good", domain.ServiceYandex)}
	if got := restarted.Allowed(ctx, tracks); len(got) != 1 || got[0].Title != "Good" {
		t.Fatal(got)
	}
	if _, err := restarted.SetFeedback(ctx, "Artist", "*", 0); err != nil {
		t.Fatal(err)
	}
	if got := restarted.Allowed(ctx, tracks); len(got) != 3 {
		t.Fatal("unblock failed", got)
	}
}

func TestExposureRotationSurvivesRestartAndServices(t *testing.T) {
	ctx := context.Background()
	st := newFakeSettings()
	e := &Engine{store: st}
	tracks := []domain.Track{}
	for i := 0; i < 12; i++ {
		tracks = append(tracks, tr(fmt.Sprint("Artist", i), fmt.Sprint("Song", i), domain.ServiceYandex))
	}
	first := e.Rotate(ctx, tracks, 4)
	if len(first) != 4 {
		t.Fatal(first)
	}
	for i := range tracks {
		tracks[i].Service = domain.ServiceSoundCloud
	}
	restarted := &Engine{store: st}
	next := restarted.Rotate(ctx, tracks, 4)
	keys := map[string]bool{}
	for _, t := range first {
		keys[trackKey(t)] = true
	}
	for _, candidate := range next {
		if keys[trackKey(candidate)] {
			t.Fatal("recent recommendation repeated despite fresh candidates")
		}
	}
}
