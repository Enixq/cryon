package identity

import (
	"testing"

	"Cryon2/internal/domain"
)

func TestGroupTracksCombinesEquivalentSourceTracks(t *testing.T) {
	groups := GroupTracks([]domain.Track{
		{ID: "yt", Service: domain.ServiceYouTube, Title: "Песня", Artists: []string{"Артист"}, DurationMs: 180_000, PlayableKind: domain.PlayableStream},
		{ID: "sc", Service: domain.ServiceSoundCloud, Title: "Песня (Official Audio)", Artists: []string{"Артист"}, DurationMs: 181_000, PlayableKind: domain.PlayableStream},
		{ID: "live", Service: domain.ServiceYouTube, Title: "Песня Live", Artists: []string{"Артист"}, DurationMs: 220_000, PlayableKind: domain.PlayableStream},
	})
	if len(groups) != 2 {
		t.Fatalf("group count: got %d, want 2", len(groups))
	}
	if len(groups[0].Candidates) != 2 {
		t.Fatalf("candidate count: got %d, want 2", len(groups[0].Candidates))
	}
}

func TestRankCandidatesHonorsLocalAndExplicitPreferences(t *testing.T) {
	candidates := []domain.PlaybackCandidate{
		{Track: domain.Track{ID: "remote", Service: domain.ServiceSoundCloud, PlayableKind: domain.PlayableStream}, Availability: true, IdentityConfidence: 1, Version: domain.VersionOriginal, Explicit: true},
		{Track: domain.Track{ID: "local", Service: domain.ServiceLocal, PlayableKind: domain.PlayableStream}, Availability: true, IdentityConfidence: 0.9, Version: domain.VersionOriginal},
	}
	ranked := RankCandidates(candidates, Preferences{PreferLocal: true, PreferExplicit: true})
	if ranked[0].Track.ID != "local" {
		t.Fatalf("local preference must win: %+v", ranked)
	}
	if ranked[0].SelectionReason == "" {
		t.Fatalf("selection reason is required")
	}
}

func TestRankCandidatesHonorsSourcePriority(t *testing.T) {
	candidates := []domain.PlaybackCandidate{
		{Track: domain.Track{ID: "youtube", Service: domain.ServiceYouTube, PlayableKind: domain.PlayableStream}, Availability: true, IdentityConfidence: 1, Version: domain.VersionOriginal},
		{Track: domain.Track{ID: "yandex", Service: domain.ServiceYandex, PlayableKind: domain.PlayableStream}, Availability: true, IdentityConfidence: 1, Version: domain.VersionOriginal},
	}
	ranked := RankCandidates(candidates, Preferences{SourcePriority: map[domain.ServiceID]int{
		domain.ServiceYandex:  50,
		domain.ServiceYouTube: 10,
	}})
	if ranked[0].Track.ID != "yandex" {
		t.Fatalf("source priority must win: %+v", ranked)
	}
}

func TestNormalizeKeepsCyrillicAndRemovesFeaturedArtist(t *testing.T) {
	if got, want := Normalize("  Артист feat. Гость — Песня! "), "артист"; got != want {
		t.Fatalf("Normalize: got %q, want %q", got, want)
	}
}
