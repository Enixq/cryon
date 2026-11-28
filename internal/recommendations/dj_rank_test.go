package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/store"
	"context"
	"fmt"
	"testing"
	"time"
)

func TestLocalDJAdmissionMMRAndRecentPenalty(t *testing.T) {
	e := New(nil, nil, nil, nil)
	pool := []domain.Track{}
	cs := []intelligence.Candidate{}
	for i, artist := range []string{"A", "A", "B", "C", "Text match only", "Blocked"} {
		id := fmt.Sprint(i)
		pool = append(pool, domain.Track{ID: id, Title: "Song " + id, Artists: []string{artist}, Service: domain.ServiceSoundCloud})
		cs = append(cs, intelligence.Candidate{ID: id, Track: intelligence.TrackSummary{ID: id, Artists: []string{artist}}, Evidence: intelligence.Evidence{TasteSimilarity: .8, ArtistAffinity: .8, MetadataQuality: .9, IdentityConfidence: .9}})
	}
	cs[4].Evidence.ArtistAffinity = 0
	cs[4].Evidence.TasteSimilarity = 0 // A catalog hit alone is not musical evidence.
	cs[5].Evidence.Feedback = -1
	cs[3].Evidence.RecentlyPlayed = true
	input := intelligence.SelectionInput{Context: intelligence.PlanningInput{Session: intelligence.SessionProfile{Mode: intelligence.RadioInfinite}}, Strategy: intelligence.Strategy{Diversity: 1}, Candidates: cs}
	plan, got := e.RankDJCandidates(context.Background(), input, pool, 10)
	if len(got) != 4 || got[0].ID != "0" || got[1].ID != "2" || got[3].ID != "3" {
		t.Fatalf("MMR/recent penalty/admission: %+v", got)
	}
	if plan.Ranked[3].Score >= plan.Ranked[0].Score {
		t.Fatal("recent play not penalized")
	}
	// Recent playback is a penalty, not an eternal ban.
	input.Candidates = cs[3:4]
	_, got = e.RankDJCandidates(context.Background(), input, pool[3:4], 1)
	if len(got) != 1 {
		t.Fatal("recent track banned even when still relevant")
	}
}

func TestLocalDJTrackRadioKeepsSeedAboveGlobalTaste(t *testing.T) {
	e := New(nil, nil, nil, nil)
	pool := []domain.Track{{ID: "global", Title: "Favorite", Artists: []string{"Favorite"}, Service: domain.ServiceYandex}, {ID: "seed-neighbor", Title: "Neighbor", Artists: []string{"New"}, Service: domain.ServiceSoundCloud}}
	cs := []intelligence.Candidate{{ID: "global", Evidence: intelligence.Evidence{TasteSimilarity: 1, ArtistAffinity: 1, MetadataQuality: 1, IdentityConfidence: 1}}, {ID: "neighbor", Channels: []intelligence.Channel{{Provider: "yandex", Capability: "similar_tracks"}}, Evidence: intelligence.Evidence{MetadataQuality: 1, IdentityConfidence: 1}}}
	in := intelligence.SelectionInput{Context: intelligence.PlanningInput{Session: intelligence.SessionProfile{Mode: intelligence.RadioTrack}}, Candidates: cs}
	_, got := e.RankDJCandidates(context.Background(), in, pool, 10)
	if len(got) != 1 || got[0].ID != "seed-neighbor" {
		t.Fatal("global taste replaced seed", got)
	}
}

func TestDJLongProfileUsesOlderHistoryAndUpdatesWithoutRestart(t *testing.T) {
	calls := 0
	rows := []store.HistoryRow{{Track: domain.Track{ID: "old", Title: "Old", Artists: []string{"Year ago"}}, PlayedAtMs: time.Now().Add(-365 * 24 * time.Hour).UnixMilli()}}
	e := New(nil, nil, nil, nil)
	e.histFn = func(_ context.Context, limit int) ([]store.HistoryRow, error) {
		if limit >= 10000 {
			calls++
		}
		return rows, nil
	}
	before := e.IntelligenceTaste(context.Background())
	found := false
	for _, p := range before.Preferences {
		if p.Name == "Year ago" {
			found = true
		}
	}
	if !found || calls == 0 {
		t.Fatal("long history ignored", before)
	}
	rows = append(rows, store.HistoryRow{Track: domain.Track{ID: "new", Title: "New", Artists: []string{"Today"}}, PlayedAtMs: time.Now().UnixMilli()})
	after := e.IntelligenceTaste(context.Background())
	if before.Revision == after.Revision {
		t.Fatal("profile did not update")
	}
}

func TestDJGraphNeighborsPreferStrongPersonalAnchors(t *testing.T) {
	e := New(nil, nil, nil, nil)
	pool := []domain.Track{{ID: "weak", Title: "Weak", Artists: []string{"New1"}, Service: domain.ServiceSoundCloud}, {ID: "strong", Title: "Strong", Artists: []string{"New2"}, Service: domain.ServiceSoundCloud}}
	cs := []intelligence.Candidate{}
	for i, anchor := range []string{"Occasional", "Favorite"} {
		cs = append(cs, intelligence.Candidate{ID: pool[i].ID, Channels: []intelligence.Channel{{Provider: "cryon", Capability: "taste_neighbors"}}, Origins: []intelligence.CandidateOrigin{{AnchorArtists: []string{anchor}}}, Evidence: intelligence.Evidence{MetadataQuality: 1, IdentityConfidence: 1}})
	}
	in := intelligence.SelectionInput{Context: intelligence.PlanningInput{Session: intelligence.SessionProfile{Mode: intelligence.RadioInfinite}, Taste: intelligence.TasteProfile{Preferences: []intelligence.ArtistPreference{{Name: "Favorite", Weight: 1, Confidence: 1}, {Name: "Occasional", Weight: .1, Confidence: .1}}}}, Candidates: cs}
	_, got := e.RankDJCandidates(context.Background(), in, pool, 2)
	if len(got) != 2 || got[0].ID != "strong" {
		t.Fatal("graph ignored long-term anchor weights", got)
	}
}
