package core

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

type importTestStore struct {
	store.Store
	tracks []domain.Track
}

func TestAPISpotifyPreviewAndSelectedImport(t *testing.T) {
	database := &importTestStore{}
	app := &App{ctx: context.Background(), store: database, platform: nullHost{}}
	makeRequest := func(action, ids string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, err := writer.CreateFormFile("file", "YourLibrary.json")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write([]byte(`{"tracks":[{"track":"One","artist":"A","uri":"spotify:track:one"},{"track":"Two","artist":"B","uri":"spotify:track:two"}]}`))
		_ = writer.WriteField("action", action)
		_ = writer.WriteField("trackIds", ids)
		_ = writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/v1/imports/spotify", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		app.APIHandler().ServeHTTP(rec, req)
		return rec
	}
	preview := makeRequest("preview", "")
	var tracks []domain.Track
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &tracks) != nil || len(tracks) != 2 || len(database.tracks) != 0 {
		t.Fatalf("preview status=%d tracks=%+v stored=%+v", preview.Code, tracks, database.tracks)
	}
	result := makeRequest("import", `["two"]`)
	if result.Code != http.StatusOK || len(database.tracks) != 1 || database.tracks[0].ID != "two" {
		t.Fatalf("import status=%d body=%s stored=%+v", result.Code, result.Body.String(), database.tracks)
	}
}

func (s *importTestStore) FavoriteList(context.Context) ([]domain.Track, error) { return s.tracks, nil }
func (s *importTestStore) FavoriteAdd(_ context.Context, track domain.Track) error {
	s.tracks = append(s.tracks, track)
	return nil
}
func (*importTestStore) NotificationAdd(context.Context, store.NotificationRow) error { return nil }

func TestSpotifyLibraryTracksAcceptsExportObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "YourLibrary.json")
	data := `{"tracks":[{"track":"Song","artist":"Artist","album":"Album","uri":"spotify:track:abc123"}],"albums":[]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	tracks, err := spotifyLibraryTracks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].ID != "abc123" || tracks[0].Title != "Song" {
		t.Fatalf("unexpected imported tracks: %+v", tracks)
	}
}

func TestPreviewSpotifyExportDoesNotRequireLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "YourLibrary.json")
	if err := os.WriteFile(path, []byte(`{"tracks":[{"track":"Song","artist":"Artist","uri":"spotify:track:abc123"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	tracks, err := (&App{}).PreviewSpotifyLibraryExport(path)
	if err != nil || len(tracks) != 1 || tracks[0].ID != "abc123" {
		t.Fatalf("preview: tracks=%+v err=%v", tracks, err)
	}
}

func TestImportSelectedSpotifyTracks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "YourLibrary.json")
	data := `{"tracks":[{"track":"One","artist":"Artist","uri":"spotify:track:one"},{"track":"Two","artist":"Artist","uri":"spotify:track:two"},{"track":"Two","artist":"Artist","uri":"spotify:track:two"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	database := &importTestStore{}
	app := &App{ctx: context.Background(), store: database, platform: nullHost{}}
	result, err := app.ImportSelectedSpotifyLibraryExport(path, []string{"two"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Found != 2 || result.Imported != 1 || result.Skipped != 1 || len(database.tracks) != 1 || database.tracks[0].ID != "two" {
		t.Fatalf("unexpected selection: result=%+v tracks=%+v", result, database.tracks)
	}
	empty, err := app.ImportSelectedSpotifyLibraryExport(path, []string{})
	if err != nil || empty.Imported != 0 || len(database.tracks) != 1 {
		t.Fatalf("empty selection: result=%+v err=%v", empty, err)
	}
}
