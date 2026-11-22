package core

import (
	"Cryon2/internal/domain"
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSavedCollectionsPersistAcrossAppInstancesWithoutDuplicates(t *testing.T) {
	settings := &apiSettingsStore{}
	registry := map[domain.ServiceID]domain.MusicService{domain.ServiceYouTube: apiStaticMusicService{}}
	item := SavedCollection{ID: "playlist:youtube:p1", Kind: "playlist", Title: "Saved list", Service: "youtube", CatalogID: "p1", Tracks: []domain.Track{{ID: "t1", Service: domain.ServiceYouTube, Title: "Song"}}}
	body, _ := json.Marshal(item)
	app := &App{store: settings, registry: registry}
	for range 2 {
		r := httptest.NewRecorder()
		app.APIHandler().ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/library/collections", bytes.NewReader(body)))
		if r.Code != 200 {
			t.Fatalf("save %d: %s", r.Code, r.Body.String())
		}
	}
	reopened := &App{store: settings, registry: registry}
	r := httptest.NewRecorder()
	reopened.APIHandler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/library/collections", nil))
	var got []SavedCollection
	if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil || len(got) != 1 || len(got[0].Tracks) != 1 || got[0].CatalogID != "p1" {
		t.Fatalf("lost/duplicated collection: %s %v", r.Body.String(), err)
	}
	r = httptest.NewRecorder()
	reopened.APIHandler().ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/v1/library/collections?id=playlist%3Ayoutube%3Ap1", nil))
	if r.Code != 200 || r.Body.String() != "[]\n" {
		t.Fatalf("delete: %d %s", r.Code, r.Body.String())
	}
}

func TestSearchFiltersSongBoundariesForSingleAndCombinedSources(t *testing.T) {
	tracks := []domain.Track{{ID: "preview", DurationMs: 30000}, {ID: "short", DurationMs: 31000}, {ID: "song", DurationMs: 180000}, {ID: "album", DurationMs: 1200000}, {ID: "unknown"}}
	app := &App{ctx: context.Background(), registry: map[domain.ServiceID]domain.MusicService{domain.ServiceYouTube: apiStaticMusicService{tracks: tracks}}}
	for _, multi := range []bool{false, true} {
		var got []domain.Track
		var err error
		if multi {
			got, err = app.SearchAll("song")
		} else {
			got, err = app.Search("youtube", "song")
		}
		if err != nil || len(got) != 3 || got[0].ID != "short" || got[2].ID != "unknown" {
			t.Fatalf("unfiltered results: %v %v", got, err)
		}
	}
}

func TestAudioExportPreservesActualFormatFilenameAndBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio")
	payload := []byte("ID3\x04\x00\x00\x00\x00\x00\x00complete-audio-payload")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	(&localAssetHandler{}).writeStreamFileContentType(path, "application/octet-stream")
	key := "youtube\x00export-fixture"
	streamFileCache.Lock()
	streamFileCache.m[key] = cachedStreamFile{path: path, size: int64(len(payload)), at: time.Now()}
	streamFileCache.Unlock()
	t.Cleanup(func() { streamFileCache.Lock(); delete(streamFileCache.m, key); streamFileCache.Unlock() })
	app := &App{registry: map[domain.ServiceID]domain.MusicService{domain.ServiceYouTube: apiStaticMusicService{}}}
	response := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/library/export?service=youtube&id=export-fixture&name=Artist%20Song", nil))
	_, params, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
	if response.Code != 200 || err != nil || params["filename"] != "Artist Song.mp3" || !bytes.Equal(payload, response.Body.Bytes()) {
		t.Fatalf("bad export: %d %s %v", response.Code, response.Header(), err)
	}
	(&localAssetHandler{}).writeStreamFileContentType(path, "video/mp4")
	if err := os.WriteFile(path, []byte("<html>upstream error</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	(&localAssetHandler{}).writeStreamFileContentType(path, "audio/mpeg")
	response = httptest.NewRecorder()
	app.APIHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/library/export?service=youtube&id=export-fixture", nil))
	if response.Code == 200 {
		t.Fatal("upstream error exported as MP3")
	}
	(&localAssetHandler{}).writeStreamFileContentType(path, "video/mp4")
	if err := os.WriteFile(path, []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isommp41"), 0600); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	app.APIHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/library/export?service=youtube&id=export-fixture", nil))
	if response.Code == 200 {
		t.Fatal("video incorrectly exported as audio")
	}
}
