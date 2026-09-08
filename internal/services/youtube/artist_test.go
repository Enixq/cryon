package youtube

import (
	"context"
	"os"
	"testing"
	"time"

	"Cryon2/internal/domain"
)

func TestParseAlbumTitle(t *testing.T) {
	cases := []struct {
		raw       string
		wantKind  string
		wantTitle string
	}{
		{"Album - БУДНИ", "album", "БУДНИ"},
		{"Single - Музыка", "single", "Музыка"},
		{"EP - Дверь", "ep", "Дверь"},
		{"Playlist - Микс", "album", "Микс"},
		{"Просто название", "album", "Просто название"},
	}
	for _, c := range cases {
		k, tl := parseAlbumTitle(c.raw)
		if k != c.wantKind || tl != c.wantTitle {
			t.Errorf("parseAlbumTitle(%q) = (%q,%q), want (%q,%q)", c.raw, k, tl, c.wantKind, c.wantTitle)
		}
	}
}

func TestBestThumb(t *testing.T) {
	thumbs := []ytThumb{
		{URL: "small", Width: 180, Height: 180},
		{URL: "big", Width: 1200, Height: 1200},
		{URL: "mid", Width: 640, Height: 640},
	}
	if got := bestThumb(thumbs); got != "big" {
		t.Errorf("bestThumb = %q, want big", got)
	}
	if got := bestThumb(nil); got != "" {
		t.Errorf("bestThumb(nil) = %q, want empty", got)
	}
}

// TestAlbumTracksLive проверяет реальный конвейер yt-dlp → трек-лист альбома.
// Требует сети и установленного yt-dlp, поэтому включается только при
// CRYON_YTDLP_IT=1 (в CI/оффлайне пропускается).
func TestAlbumTracksLive(t *testing.T) {
	if os.Getenv("CRYON_YTDLP_IT") != "1" {
		t.Skip("integration: set CRYON_YTDLP_IT=1 to run")
	}
	s := New("")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Альбом «БУДНИ» (Jahmal TGK) на YouTube Music.
	tracks, err := s.AlbumTracks(ctx, "MPREb_9uqvZLifpKX")
	if err != nil {
		t.Fatalf("AlbumTracks: %v", err)
	}
	if len(tracks) < 5 {
		t.Fatalf("ожидали полный трек-лист, получили %d треков", len(tracks))
	}
	for _, tr := range tracks {
		if tr.ID == "" || tr.Title == "" || tr.Service != domain.ServiceYouTube {
			t.Fatalf("битый трек: %+v", tr)
		}
	}
	t.Logf("получено %d треков; первый: %q", len(tracks), tracks[0].Title)
}

// TestGetArtistLive проверяет сборку страницы исполнителя (поиск + раскрытие
// альбомов). Включается только при CRYON_YTDLP_IT=1.
func TestGetArtistLive(t *testing.T) {
	if os.Getenv("CRYON_YTDLP_IT") != "1" {
		t.Skip("integration: set CRYON_YTDLP_IT=1 to run")
	}
	s := New("")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	info, err := s.GetArtist(ctx, "Jahmal TGK")
	if err != nil {
		t.Fatalf("GetArtist: %v", err)
	}
	total := len(info.Albums) + len(info.Singles)
	if total == 0 {
		t.Fatalf("ожидали релизы исполнителя, получили 0")
	}
	for _, al := range append(append([]domain.Album{}, info.Albums...), info.Singles...) {
		if al.ID == "" || al.Title == "" || al.TrackCount == 0 {
			t.Fatalf("битый релиз: %+v", al)
		}
	}
	t.Logf("релизов: %d (альбомов %d, синглов/EP %d), фото: %v",
		total, len(info.Albums), len(info.Singles), info.ArtworkURL != "")
	if len(info.Albums) > 0 {
		t.Logf("первый альбом: %q — %d треков, обложка=%v",
			info.Albums[0].Title, info.Albums[0].TrackCount, info.Albums[0].ArtworkURL != "")
	}
}

