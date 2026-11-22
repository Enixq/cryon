package yandex

import (
	"Cryon2/internal/domain"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type similarityTransport func(*http.Request) (*http.Response, error)

func (f similarityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTrackSimilarityUsesRecordingAndFiltersUnavailableResults(t *testing.T) {
	s := New("test-token")
	called := false
	s.httpClient.Transport = similarityTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/tracks/42/similar" {
			t.Fatalf("wrong similarity seed path: %s", r.URL.Path)
		}
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"result":{"similarTracks":[{"id":43,"title":"Similar","artists":[{"name":"Other artist"}],"durationMs":180000,"available":true},{"id":44,"title":"Preview","durationMs":30000,"available":true},{"id":45,"title":"Unavailable","durationMs":180000,"available":false}]}}`)), Request: r}, nil
	})
	tracks, err := s.SimilarTracks(context.Background(), domain.Track{ID: "42:99", Service: domain.ServiceYandex})
	if err != nil || !called || len(tracks) != 1 || tracks[0].Artists[0] != "Other artist" {
		t.Fatalf("bad track similarity: %v %v", tracks, err)
	}
	s.SetToken("")
	called = false
	_, err = s.SimilarTracks(context.Background(), domain.Track{ID: "42", Service: domain.ServiceYandex})
	if err != nil || called {
		t.Fatal("disconnected similarity contacted provider")
	}
}
