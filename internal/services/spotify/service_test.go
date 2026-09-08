package spotify

import (
	"testing"

	"Cryon2/internal/domain"
)

func TestServiceID(t *testing.T) {
	if New("", "").ID() != domain.ServiceSpotify {
		t.Errorf("ID() = %q, want spotify", New("", "").ID())
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "x", "y"); got != "x" {
		t.Errorf("firstNonEmpty = %q, want x", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty пустых = %q, want пусто", got)
	}
	if got := firstNonEmpty("a"); got != "a" {
		t.Errorf("firstNonEmpty = %q, want a", got)
	}
}

func TestSpotifyTrimsCreds(t *testing.T) {
	s := New("  id  ", "  secret ")
	if s.clientID != "id" || s.clientSecret != "secret" {
		t.Errorf("креды не обрезаны: %q / %q", s.clientID, s.clientSecret)
	}
}

func TestParseSpotifySearchPage(t *testing.T) {
	html := `<html><body><a href="/track/abc123">Midnight City</a><a href="https://open.spotify.com/track/def456">Another Song</a></body></html>`
	got := parseSpotifySearchPage(html)
	if len(got) != 2 {
		t.Fatalf("parseSpotifySearchPage() вернул %d результатов, want 2", len(got))
	}
	if got[0].ExternalURL != "https://open.spotify.com/track/abc123" {
		t.Fatalf("первый URL = %q, want spotify track url", got[0].ExternalURL)
	}
	if got[0].Title != "Midnight City" {
		t.Fatalf("первый title = %q, want Midnight City", got[0].Title)
	}
}
