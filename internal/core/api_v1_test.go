package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Cryon2/internal/domain"
	"Cryon2/internal/playback"
	"Cryon2/internal/store"
)

func TestAPIHealth(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/health", nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body apiHealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" || body.API != "v1" || body.AtMs <= 0 {
		t.Fatalf("unexpected health payload: %+v", body)
	}
}

func TestAPIHealthReportsLibraryStartupError(t *testing.T) {
	app := &App{startupError: "база создана более новой версией"}
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/system/health", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "более новой версией") {
		t.Fatalf("unexpected health response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDesktopAssetHandlerExposesAPIv1(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/health", nil)
	rec := httptest.NewRecorder()
	app.AssetHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAPIRejectsUnknownMethod(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/health", nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestAPISearchRequiresQuery(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

type apiJobStore struct {
	store.Store
	jobs     []store.Job
	enqueued store.Job
}

type apiSettingsStore struct {
	store.Store
	value string
}

func (s *apiSettingsStore) SettingGet(context.Context, string) (string, bool, error) {
	return s.value, s.value != "", nil
}

func (s *apiSettingsStore) SettingSet(_ context.Context, _ string, value string) error {
	s.value = value
	return nil
}

func (s apiJobStore) JobList(context.Context, int) ([]store.Job, error) {
	return s.jobs, nil
}

func (s *apiJobStore) JobEnqueue(_ context.Context, job store.Job) (store.Job, error) {
	job.ID = "job_created"
	job.State = store.JobQueued
	s.enqueued = job
	return job, nil
}

func TestAPIListsDurableJobs(t *testing.T) {
	jobStore := &apiJobStore{jobs: []store.Job{{ID: "job_1", Kind: "scan", State: store.JobQueued}}}
	app := &App{store: jobStore}
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var result []store.Job
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 || result[0].ID != "job_1" || result[0].State != store.JobQueued {
		t.Fatalf("unexpected jobs: %+v", result)
	}
}

func TestAPIRejectsInvalidJob(t *testing.T) {
	app := &App{store: &apiJobStore{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(`{"kind":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIRejectsUnknownJobKind(t *testing.T) {
	jobs := &apiJobStore{}
	app := &App{store: jobs}
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(`{"kind":"unknown"}`))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || jobs.enqueued.Kind != "" {
		t.Fatalf("unknown job accepted: status=%d job=%+v", rec.Code, jobs.enqueued)
	}
}

func TestAPICreatesDurableJob(t *testing.T) {
	jobs := &apiJobStore{}
	app := &App{store: jobs, platform: nullHost{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(`{"kind":"library.rescan","priority":50}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusCreated)
	}
	if jobs.enqueued.Kind != JobKindLibraryRescan || jobs.enqueued.Priority != 50 {
		t.Fatalf("unexpected enqueued job: %+v", jobs.enqueued)
	}
}

func TestAPIPlaybackResolveRejectsMalformedTrack(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/playback/resolve", bytes.NewBufferString(`{"track":{"id":"","service":"youtube"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIPlaybackCommandsValidateInput(t *testing.T) {
	app := &App{player: &playback.Controller{}}
	cases := []struct {
		path string
		body string
	}{
		{path: "/v1/playback/play", body: `{}`},
		{path: "/v1/playback/seek", body: `{"positionS":-1}`},
		{path: "/v1/playback/volume", body: `{"volume":101}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		rec := httptest.NewRecorder()
		app.APIHandler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want %d", tc.path, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestAPIPlaybackStatusRequiresDesktopPlayer(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/playback/status", nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestAPIPlaybackPreferencesPersist(t *testing.T) {
	settings := &apiSettingsStore{}
	app := &App{store: settings}
	body := `{"preferLocal":true,"preferExplicit":false,"officialOnly":true,"sourcePriority":{"yandex":50,"youtube":10}}`
	req := httptest.NewRequest(http.MethodPut, "/v1/playback/preferences", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("save status: got %d, want %d", rec.Code, http.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/playback/preferences", nil)
	rec = httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var preferences PlaybackPreferences
	if err := json.NewDecoder(rec.Body).Decode(&preferences); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !preferences.PreferLocal || preferences.OfficialOnly || preferences.SourcePriority["yandex"] != 50 {
		t.Fatalf("unexpected preferences: %+v", preferences)
	}
}

type apiEventTestHost struct {
	event string
}

func (h *apiEventTestHost) Emit(event string, _ ...interface{})           { h.event = event }
func (*apiEventTestHost) OpenURL(string)                                  {}
func (*apiEventTestHost) PickFile(string, string, string) (string, error) { return "", nil }
func (*apiEventTestHost) PickDirectory(string) (string, error)            { return "", nil }

func TestAPIEventHubMirrorsPlatformEvents(t *testing.T) {
	hub := newAPIEventHub()
	client := make(chan []byte, 1)
	hub.clients[client] = struct{}{}
	legacy := &apiEventTestHost{}
	host := newEventMirroringHost(legacy, hub)

	host.Emit("source:status-changed", "youtube")

	if legacy.event != "source:status-changed" {
		t.Fatalf("legacy event: got %q", legacy.event)
	}
	var envelope apiEventEnvelope
	select {
	case payload := <-client:
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatalf("decode event: %v", err)
		}
	default:
		t.Fatal("API event was not delivered")
	}
	if envelope.Event != "source:status-changed" || len(envelope.Data) != 1 || envelope.Data[0] != "youtube" {
		t.Fatalf("unexpected event: %+v", envelope)
	}
}

func TestAPIRecommendationRoutesReturnTrackLists(t *testing.T) {
	app := &App{}
	for _, path := range []string{
		"/v1/recommendations?limit=10",
		"/v1/recommendations/daily-mix?limit=10",
		"/v1/recommendations/weekly-discoveries?limit=10",
		"/v1/recommendations/new-releases?limit=10",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		app.APIHandler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestAPIRelatedTracksRequiresTrack(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/recommendations/related", bytes.NewBufferString(`{"limit":10}`))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAPIRejectsUnknownSourceValidation(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/sources/unknown/validate", nil)
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

type apiLibraryStore struct {
	store.Store
	history     []store.HistoryRow
	playlists   []domain.UserPlaylist
	playlistIDs []string
	addedTracks []domain.Track
}

func (s *apiLibraryStore) HistoryAdd(_ context.Context, track domain.Track) error {
	s.history = append(s.history, store.HistoryRow{Track: track, PlayedAtMs: 100})
	return nil
}

func (s *apiLibraryStore) HistoryList(context.Context, int) ([]store.HistoryRow, error) {
	return s.history, nil
}

func (s *apiLibraryStore) HistoryClear(context.Context) error {
	s.history = nil
	return nil
}

func (s *apiLibraryStore) PlaylistCreate(_ context.Context, title, description string) (domain.UserPlaylist, error) {
	playlist := domain.UserPlaylist{ID: "playlist_1", Title: title, Description: description}
	s.playlists = append(s.playlists, playlist)
	return playlist, nil
}

func (s *apiLibraryStore) PlaylistList(context.Context) ([]domain.UserPlaylist, error) {
	return s.playlists, nil
}

func (s *apiLibraryStore) PlaylistAddTrack(_ context.Context, playlistID string, track domain.Track) error {
	s.playlistIDs = append(s.playlistIDs, playlistID)
	s.addedTracks = append(s.addedTracks, track)
	return nil
}

func TestAPIHistoryMutation(t *testing.T) {
	library := &apiLibraryStore{}
	app := &App{store: library}

	req := httptest.NewRequest(http.MethodPost, "/v1/library/history", bytes.NewBufferString(`{"track":{"id":"video","service":"youtube"}}`))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || len(library.history) != 1 {
		t.Fatalf("unexpected history mutation: status=%d history=%+v", rec.Code, library.history)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/library/history", nil)
	rec = httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || len(library.history) != 0 {
		t.Fatalf("unexpected history clear: status=%d history=%+v", rec.Code, library.history)
	}
}

func TestAPIPlaylistMutations(t *testing.T) {
	library := &apiLibraryStore{}
	app := &App{store: library}

	req := httptest.NewRequest(http.MethodPost, "/v1/playlists", bytes.NewBufferString(`{"title":"Road trip","description":"For driving"}`))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || len(library.playlists) != 1 {
		t.Fatalf("unexpected playlist creation: status=%d playlists=%+v", rec.Code, library.playlists)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/playlists/playlist_1/tracks", bytes.NewBufferString(`{"track":{"id":"video","service":"youtube"}}`))
	rec = httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || len(library.addedTracks) != 1 || library.playlistIDs[0] != "playlist_1" {
		t.Fatalf("unexpected playlist track mutation: status=%d ids=%v tracks=%+v", rec.Code, library.playlistIDs, library.addedTracks)
	}
}

type apiStaticMusicService struct {
	id     domain.ServiceID
	tracks []domain.Track
}

func (s apiStaticMusicService) ID() domain.ServiceID { return s.id }

func (s apiStaticMusicService) Search(context.Context, string) ([]domain.Track, error) {
	return s.tracks, nil
}

func TestAPIPlaybackFallbackReturnsEquivalentTrack(t *testing.T) {
	app := &App{
		ctx: context.Background(),
		registry: map[domain.ServiceID]domain.MusicService{
			domain.ServiceSoundCloud: apiStaticMusicService{
				id: domain.ServiceSoundCloud,
				tracks: []domain.Track{{
					ID:           "soundcloud-version",
					Service:      domain.ServiceSoundCloud,
					Title:        "Same Song",
					Artists:      []string{"Artist"},
					DurationMs:   181_000,
					PlayableKind: domain.PlayableStream,
				}},
			},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/playback/fallback", bytes.NewBufferString(`{"track":{"id":"youtube-version","service":"youtube","title":"Same Song","artists":["Artist"],"durationMs":180000,"playableKind":"stream"},"failedService":"youtube"}`))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body apiPlaybackFallbackResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Track.Service != domain.ServiceSoundCloud || body.Track.ID != "soundcloud-version" || body.Reason == "" {
		t.Fatalf("unexpected fallback: %+v", body)
	}
}

func TestAPITrackStreamURLUsesLocalFileRoute(t *testing.T) {
	if got := apiTrackStreamURL(domain.Track{Service: domain.ServiceLocal, ID: "folder/song.mp3"}); got != "/local/folder%2Fsong.mp3" {
		t.Fatalf("local URL: %q", got)
	}
	if got := apiTrackStreamURL(domain.Track{Service: domain.ServiceYouTube, ID: "video"}); got != "/stream/youtube/video" {
		t.Fatalf("remote URL: %q", got)
	}
}

func TestAPIPlaybackFallbackSelectsLocalStreamRoute(t *testing.T) {
	app := &App{ctx: context.Background(), registry: map[domain.ServiceID]domain.MusicService{
		domain.ServiceLocal: apiStaticMusicService{id: domain.ServiceLocal, tracks: []domain.Track{{ID: "local-id", Service: domain.ServiceLocal, Title: "Same Song", Artists: []string{"Artist"}, DurationMs: 180000, PlayableKind: domain.PlayableStream}}},
	}}
	req := httptest.NewRequest(http.MethodPost, "/v1/playback/fallback", bytes.NewBufferString(`{"track":{"id":"video","service":"youtube","title":"Same Song","artists":["Artist"],"durationMs":180000,"playableKind":"stream"},"failedService":"youtube"}`))
	rec := httptest.NewRecorder()
	app.APIHandler().ServeHTTP(rec, req)
	var result apiPlaybackFallbackResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil || result.StreamURL != "/local/local-id" {
		t.Fatalf("fallback: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResolvePlayableTrackRejectsDifferentDuration(t *testing.T) {
	original := domain.Track{ID: "imported", Service: domain.ServiceSpotify, Title: "Same Song", Artists: []string{"Artist"}, DurationMs: 180000, PlayableKind: domain.PlayableExternal}
	app := &App{ctx: context.Background(), registry: map[domain.ServiceID]domain.MusicService{
		domain.ServiceSoundCloud: apiStaticMusicService{id: domain.ServiceSoundCloud, tracks: []domain.Track{{ID: "different", Service: domain.ServiceSoundCloud, Title: "Same Song", Artists: []string{"Artist"}, DurationMs: 240000, PlayableKind: domain.PlayableStream}}},
	}}
	resolved, err := app.ResolvePlayableTrack(original)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != original.ID || resolved.Service != original.Service {
		t.Fatalf("selected recording with incompatible duration: %+v", resolved)
	}
}
