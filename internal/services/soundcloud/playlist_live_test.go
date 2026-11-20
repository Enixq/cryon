package soundcloud

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPublicPlaylistLive(t *testing.T) {
	if os.Getenv("CRYON_SOUNDCLOUD_CATALOG_IT") != "1" {
		t.Skip("live SoundCloud test is disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := New("")
	playlists, err := s.SearchPlaylists(ctx, "Святые из хрущёвок", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(playlists) == 0 {
		t.Fatal("no playlists found")
	}
	for _, playlist := range playlists {
		tracks, err := s.PlaylistTracks(ctx, playlist.ID)
		if err == nil && len(tracks) > 0 {
			t.Logf("playlist %q: %d tracks", playlist.Name, len(tracks))
			return
		}
		t.Logf("playlist %q: %d tracks, error %v", playlist.Name, len(tracks), err)
	}
	t.Fatal("no playlist tracks could be loaded")
}
