package recommendations

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/services/deezer"
	"context"
	"testing"
)

func TestCatalogResolutionRejectsOtherArtistsVersionsAndPreviews(t *testing.T) {
	e := &Engine{search: func(context.Context, string) ([]domain.Track, error) {
		return []domain.Track{
			{ID: "wrong", Title: "Song", Artists: []string{"Other"}, Service: domain.ServiceYandex, PlayableKind: domain.PlayableStream, DurationMs: 180000},
			{ID: "remix", Title: "Song Remix", Artists: []string{"Artist"}, Service: domain.ServiceYandex, PlayableKind: domain.PlayableStream, DurationMs: 180000},
			{ID: "preview", Title: "Song", Artists: []string{"Artist"}, Service: domain.ServiceYandex, PlayableKind: domain.PlayableStream, DurationMs: 30000},
			{ID: "full", Title: "Song", Artists: []string{"Artist"}, Service: domain.ServiceYandex, PlayableKind: domain.PlayableStream, DurationMs: 180000},
		}, nil
	}}
	metadata := deezer.CatalogTrack{Title: "Song", Duration: 180}
	metadata.Artist.Name = "Artist"
	got := e.ResolveCatalog(context.Background(), []deezer.CatalogTrack{metadata}, 10)
	if len(got) != 1 || got[0].ID != "full" {
		t.Fatalf("wrong recording: %v", got)
	}
}
