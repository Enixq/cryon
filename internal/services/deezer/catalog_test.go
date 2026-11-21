package deezer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenreUsesOnlyRadiosFromRequestedCatalogue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/radio/genres":
			fmt.Fprint(w, `{"data":[{"id":116,"radios":[{"id":1}]},{"id":152,"radios":[{"id":2}]}]}`)
		case "/radio/2/tracks":
			fmt.Fprint(w, `{"data":[{"id":42,"title":"Rock song","artist":{"name":"Rock band"},"duration":200}]}`)
		default:
			t.Errorf("wrong genre endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	got, err := newTestClient(server.URL).GenreTracks(context.Background(), 152, 20)
	if err != nil || len(got) != 1 || got[0].Title != "Rock song" {
		t.Fatalf("wrong collection: %v %v", got, err)
	}
}

func TestArtistReleasesNeverUseOldAlbumsOrUnknownDates(t *testing.T) {
	recent := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/artist":
			fmt.Fprint(w, `{"data":[{"id":8,"name":"Artist"}]}`)
		case "/artist/8/albums":
			fmt.Fprintf(w, `{"data":[{"id":1,"release_date":"2020-01-01"},{"id":2,"release_date":%q},{"id":3,"release_date":""}]}`, recent)
		case "/album/2/tracks":
			fmt.Fprint(w, `{"data":[{"id":22,"title":"Fresh","duration":180,"artist":{"name":"Artist"}}]}`)
		default:
			t.Errorf("old album requested: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	got, err := newTestClient(server.URL).ArtistRecentTracks(context.Background(), "Artist", 10)
	if err != nil || len(got) != 1 || got[0].ReleaseDate != recent {
		t.Fatalf("unverified release: %v %v", got, err)
	}
}
