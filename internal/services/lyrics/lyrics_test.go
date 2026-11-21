package lyrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type lyricTransport func(*http.Request) (*http.Response, error)

func (f lyricTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLyricsRejectWrongArtistAndWrongLengthThenUseAnotherProvider(t *testing.T) {
	c := New()
	c.httpClient.Transport = lyricTransport(func(r *http.Request) (*http.Response, error) {
		status, body := 200, `{}`
		switch r.URL.Path {
		case "/api/get":
			status = 404
		case "/api/search":
			body = `[{"trackName":"Song","artistName":"Other","syncedLyrics":"wrong"},{"trackName":"Song","artistName":"Artist","duration":30,"plainLyrics":"preview"}]`
		case "/v1/Artist/Song":
			body = `{"lyrics":"Correct full text"}`
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	got, err := c.Get(context.Background(), "Artist", "Song (Official Video)", "", 180)
	if err != nil || !got.Found || got.Plain != "Correct full text" || got.Provider != "lyrics.ovh" {
		t.Fatalf("bad fallback: %+v %v", got, err)
	}
}

func TestEmptyExactRecordDoesNotPreventProviderFallback(t *testing.T) {
	c := New()
	c.httpClient.Transport = lyricTransport(func(r *http.Request) (*http.Response, error) {
		body := `{}`
		switch r.URL.Path {
		case "/api/get":
			body = `{"trackName":"Song","artistName":"Artist"}`
		case "/api/search":
			body = `[]`
		case "/v1/Artist/Song":
			body = `{"lyrics":"Full text from second provider"}`
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	got, err := c.Get(context.Background(), "Artist", "Song", "", 180)
	if err != nil || !got.Found || got.Provider != "lyrics.ovh" {
		t.Fatalf("empty exact record blocked fallback: %+v %v", got, err)
	}
}
