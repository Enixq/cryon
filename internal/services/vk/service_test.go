package vk

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDisconnectedVKDoesNotUseNetwork(t *testing.T) {
	s := New("")
	s.transport = testTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("disconnected adapter contacted VK")
		return nil, nil
	})
	tracks, err := s.Search(context.Background(), "Artist Song")
	if err != nil || len(tracks) != 0 {
		t.Fatal("disconnected search must be empty")
	}
	if _, err = s.ResolveStream(context.Background(), "id"); err == nil {
		t.Fatal("disconnected stream accepted")
	}
}

func TestVKSearchPermissionFilteringStreamAndLyrics(t *testing.T) {
	s := New("test-token")
	track := `{"apiId":"good","name":"Song","artists":[{"name":"Artist"}],"duration":180,"permissions":{"permit":true},"file":"https://cdn.example/song.mp3","lyrics":{"intervals":[{"begin":12500,"line":"Words"}]}}`
	s.transport = testTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-App-Id") != "android" {
			t.Fatal("adapter headers missing")
		}
		body := `{"data":{"track":` + track + `}}`
		if strings.Contains(r.URL.Path, "/search/track/") {
			body = `{"data":{"tracks":[` + track + `,{"apiId":"restricted","duration":180,"permissions":{"permit":false},"file":"https://cdn.example/no.mp3"},{"apiId":"preview","duration":30,"permissions":{"permit":true},"file":"https://cdn.example/preview.mp3"}]}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	tracks, err := s.Search(context.Background(), "Artist Song")
	if err != nil || len(tracks) != 1 || tracks[0].ID != "good" || tracks[0].DurationMs != 180000 {
		t.Fatalf("invalid search mapping: %v %v", tracks, err)
	}
	stream, err := s.ResolveStream(context.Background(), "good")
	if err != nil || stream.URL != "https://cdn.example/song.mp3" || stream.MimeType != "audio/mpeg" {
		t.Fatalf("invalid stream: %v %v", stream, err)
	}
	lyrics, err := s.TrackLyrics(context.Background(), "good")
	if err != nil || !lyrics.Found || lyrics.Synced != "[00:12.50]Words" {
		t.Fatalf("invalid source lyrics: %v %v", lyrics, err)
	}
}
