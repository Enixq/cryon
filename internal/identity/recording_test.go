package identity

import (
	"Cryon2/internal/domain"
	"testing"
)

func TestRecordingIdentityAcrossCatalogPresentation(t *testing.T) {
	anchor := domain.Track{Title: "Song", Artists: []string{"Artist"}, DurationMs: 180000, Album: "Album"}
	cases := []struct {
		name  string
		track domain.Track
		match bool
	}{
		{"topic", domain.Track{Title: "Song", Artists: []string{"Artist - Topic"}, DurationMs: 180100}, true},
		{"video_title", domain.Track{Title: "Artist - Song (Official Music Video)", Artists: []string{"Label"}, DurationMs: 182000}, true},
		{"punctuation", domain.Track{Title: "Song!", Artists: []string{"ARTIST"}}, true},
		{"no_artist_evidence", domain.Track{Title: "Song", Artists: []string{"Label"}}, false},
		{"live", domain.Track{Title: "Song (Live)", Artists: []string{"Artist"}}, false},
		{"live_after_feature", domain.Track{Title: "Song feat. Guest (Live)", Artists: []string{"Artist"}}, false},
		{"remix", domain.Track{Title: "Song (Remix)", Artists: []string{"Artist"}}, false},
		{"acoustic", domain.Track{Title: "Song (Acoustic)", Artists: []string{"Artist"}}, false},
		{"wrong_duration", domain.Track{Title: "Song", Artists: []string{"Artist"}, DurationMs: 250000}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score, reason := MatchRecording(anchor, tc.track)
			if (score > 0) != tc.match {
				t.Fatal(score, reason)
			}
		})
	}
	anchor.Title = "Song (Live)"
	if score, _ := MatchRecording(anchor, domain.Track{Title: "Artist - Song [Live]", Artists: []string{"Label"}}); score == 0 {
		t.Fatal("same live recording not accepted")
	}
}
