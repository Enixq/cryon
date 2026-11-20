package youtube

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCatalogHelpersExtractMusicIdentifiers(t *testing.T) {
	value := map[string]any{
		"browseEndpoint": map[string]any{"browseId": "MPREb_release"},
		"watchEndpoint":  map[string]any{"videoId": "video-1"},
		"flexColumns": []any{map[string]any{
			"musicResponsiveListItemFlexColumnRenderer": map[string]any{
				"text": map[string]any{"runs": []any{map[string]any{"text": "Album title"}}},
			},
		}},
	}
	if got := firstBrowseID(value, "MPREb_"); got != "MPREb_release" {
		t.Fatalf("album browse id = %q", got)
	}
	if got := catalogVideoID(value); got != "video-1" {
		t.Fatalf("video id = %q", got)
	}
	if got := catalogItemTitle(value); got != "Album title" {
		t.Fatalf("item title = %q", got)
	}
}

func TestArtistCatalogDoesNotIncludeMenuTextLive(t *testing.T) {
	if os.Getenv("CRYON_YOUTUBE_CATALOG_IT") != "1" {
		t.Skip("live YouTube test is disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	svc := New("")
	info, err := svc.getArtistInnerTube(ctx, "Jahmal TGK")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Albums)+len(info.Singles) == 0 {
		t.Fatal("artist has no releases")
	}
	for _, track := range info.TopTracks {
		for _, artist := range track.Artists {
			if strings.Contains(strings.ToLower(artist), "включить микс") {
				t.Fatalf("menu text in artist: %q", artist)
			}
		}
	}
	for _, release := range info.Albums {
		if strings.EqualFold(release.Title, "SON CUBANO") {
			tracks, ok := svc.cachedAlbumTracks(release.ID)
			if !ok || len(tracks) == 0 {
				t.Fatalf("album %q has no cached tracks", release.Title)
			}
			for _, track := range tracks {
				if len(track.Artists) == 0 || strings.Contains(strings.ToLower(track.Artists[0]), "включить микс") {
					t.Fatalf("bad artist for %q: %q", track.Title, track.Artists)
				}
			}
			t.Logf("%s: %d tracks, first artist %q", release.Title, len(tracks), tracks[0].Artists[0])
		}
	}
	t.Logf("%d albums, %d singles, %d tracks", len(info.Albums), len(info.Singles), len(info.TopTracks))
}

func TestParseCatalogDuration(t *testing.T) {
	if got := parseCatalogDuration("4:07"); got != 247000 {
		t.Fatalf("duration = %d", got)
	}
	if got := parseCatalogDuration("bad"); got != 0 {
		t.Fatalf("invalid duration = %d", got)
	}
}

func TestMissingColumnDoesNotIncludeMenuText(t *testing.T) {
	item := map[string]any{
		"flexColumns": []any{map[string]any{"musicResponsiveListItemFlexColumnRenderer": map[string]any{"text": map[string]any{"runs": []any{map[string]any{"text": "Song"}}}}}},
		"menu":        map[string]any{"runs": []any{map[string]any{"text": "Sign in to add to queue"}}},
	}
	if got := catalogItemColumn(item, 1); got != "" {
		t.Fatalf("missing artist column included unrelated text: %q", got)
	}
}

func TestCatalogPlaylistLive(t *testing.T) {
	if os.Getenv("CRYON_YOUTUBE_CATALOG_IT") != "1" {
		t.Skip("integration: set CRYON_YOUTUBE_CATALOG_IT=1 to run")
	}
	s := New("")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	playlists, err := s.SearchPlaylists(ctx, "2к17", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) == 0 {
		t.Fatal("плейлисты YouTube Music не найдены")
	}
	tracks, err := s.PlaylistTracks(ctx, playlists[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) == 0 {
		t.Fatalf("плейлист %q вернул 0 треков", playlists[0].Name)
	}
}
