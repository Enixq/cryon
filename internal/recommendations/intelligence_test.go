package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/services/lastfm"
	"context"
	"testing"
)

func TestDJFinalizationAndCapabilityAvailability(t *testing.T) {
	e := New(nil, lastfm.New(""), nil, nil)
	e.SetTrackSimilarity(func(context.Context, domain.Track) ([]domain.Track, error) { return nil, nil })
	e.SetTrackSimilarityAvailability(func() bool { return false })
	for _, channel := range e.DJChannels() {
		if channel.Channel.Provider == "yandex" {
			t.Fatal("disconnected channel advertised")
		}
	}
	seed := domain.Track{ID: "seed", Service: domain.ServiceSoundCloud, Title: "Seed", Artists: []string{"Original"}}
	newTrack := domain.Track{ID: "new", Service: domain.ServiceSoundCloud, Title: "New", Artists: []string{"Unknown"}, RecommendationReason: "Рекомендовано Cryon AI", DJPlanID: "plan"}
	duplicate := newTrack
	duplicate.ID = "another-source-id"
	external := domain.Track{ID: "external", Service: domain.ServiceSpotify, Title: "External", Artists: []string{"Other"}, PlayableKind: domain.PlayableExternal}
	youtube := domain.Track{ID: "youtube", Service: domain.ServiceYouTube, Title: "Video", Artists: []string{"Video"}}
	embedded := domain.Track{ID: "embedded", Service: domain.ServiceSpotify, Title: "Embed", Artists: []string{"Embedded"}, PlayableKind: domain.PlayableEmbeddedWeb}
	out := e.FinalizeDJ(context.Background(), []domain.Track{seed, newTrack, duplicate, external, youtube, embedded}, []domain.Track{seed}, 40)
	if len(out) != 1 || out[0].DJPlanID != "plan" || out[0].RecommendationReason != "Рекомендовано Cryon AI" {
		t.Fatal(out)
	}
	// Metadata capabilities are independent from the audio service.
	e.SetTrackSimilarityAvailability(func() bool { return true })
	found := false
	for _, channel := range e.DJChannels() {
		found = found || channel.Channel == (intelligence.Channel{Provider: "yandex", Capability: "similar_tracks"})
	}
	if !found {
		t.Fatal("connected channel absent")
	}
}
