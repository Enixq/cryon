package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/services/local"
	"Cryon2/internal/services/vkweb"
)

type versionService struct {
	id     domain.ServiceID
	tracks []domain.Track
	err    error
	auth   bool
	calls  atomic.Int32
	wait   bool
}

func TestDJLocalCatalogUsesFieldSearchAndVerifiesFile(t *testing.T) {
	a, _ := versionApp(t)
	a.djVersionProbe = nil
	file := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(file, []byte("test-file"), 0600); err != nil {
		t.Fatal(err)
	}
	track := versionTrack(domain.ServiceLocal)
	a.local = local.New()
	a.local.Replace([]local.LocalTrack{{Track: track, FilePath: file}})
	a.registry = map[domain.ServiceID]domain.MusicService{domain.ServiceLocal: a.local}
	anchor := versionTrack("")
	d := DJDiagnostic{}
	queue, _ := a.resolveDJVersions(context.Background(), []domain.Track{anchor}, []intelligence.Candidate{versionCandidate(anchor)}, &d)
	if len(queue) != 1 || queue[0].Service != domain.ServiceLocal {
		t.Fatal(queue, d)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	d = DJDiagnostic{}
	queue, _ = a.resolveDJVersions(context.Background(), []domain.Track{anchor}, []intelligence.Candidate{versionCandidate(anchor)}, &d)
	if len(queue) != 0 || sourceDiagnostic(t, d, domain.ServiceLocal).State != "unplayable" {
		t.Fatal(queue, d)
	}
}

func (s *versionService) ID() domain.ServiceID { return s.id }
func (s *versionService) HasToken() bool       { return s.auth }
func (s *versionService) Search(ctx context.Context, _ string) ([]domain.Track, error) {
	s.calls.Add(1)
	if s.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.tracks, s.err
}

func versionTrack(id domain.ServiceID) domain.Track {
	return domain.Track{ID: "recording", Service: id, Title: "Song", Artists: []string{"Artist"}, DurationMs: 180000, PlayableKind: domain.PlayableStream}
}
func versionApp(t *testing.T) (*App, map[domain.ServiceID]*versionService) {
	t.Helper()
	a := coordinatorApp(t)
	a.registry = map[domain.ServiceID]domain.MusicService{}
	mocks := map[domain.ServiceID]*versionService{}
	for _, id := range djVersionSources {
		s := &versionService{id: id, auth: true}
		mocks[id] = s
		a.registry[id] = s
	}
	a.djVersionProbe = func(context.Context, domain.Track) error { return nil }
	return a, mocks
}
func versionCandidate(track domain.Track) intelligence.Candidate {
	return intelligence.Candidate{ID: "candidate", CanonicalID: "work", Track: djSummary(track), Origins: []intelligence.CandidateOrigin{{Channel: intelligence.Channel{Provider: "lastfm", Capability: "similar_tracks"}, AnchorID: "seed"}}}
}
func sourceDiagnostic(t *testing.T, d DJDiagnostic, id domain.ServiceID) DJSourceDiagnostic {
	t.Helper()
	for _, s := range d.Sources {
		if s.Source == id {
			return s
		}
	}
	t.Fatal("missing diagnostic", id)
	return DJSourceDiagnostic{}
}
func setVersionPolicy(t *testing.T, a *App, p DJSourcePolicy) {
	t.Helper()
	raw, _ := json.Marshal(p)
	if err := a.store.SettingSet(context.Background(), djSourcePolicyKey, string(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestDJAllVersionsPersistAsOneRecommendationAndFallback(t *testing.T) {
	a, m := versionApp(t)
	for _, id := range djVersionSources[:4] {
		m[id].tracks = []domain.Track{versionTrack(id)}
	}
	anchor := versionTrack(domain.ServiceYandex)
	d := DJDiagnostic{}
	queue, candidates := a.resolveDJVersions(context.Background(), []domain.Track{anchor}, []intelligence.Candidate{versionCandidate(anchor)}, &d)
	if len(queue) != 1 || len(candidates) != 1 || len(candidates[0].PlaybackCandidates) != 4 {
		t.Fatal(queue, candidates)
	}
	if candidates[0].CanonicalID != "work" || candidates[0].Origins[0].AnchorID != "seed" {
		t.Fatal("lost provenance")
	}
	for _, id := range djVersionSources[:4] {
		s := sourceDiagnostic(t, d, id)
		if s.Queries != 1 || s.Found != 1 || s.Matched != 1 {
			t.Fatal(s)
		}
	}
	repo := a.store.(intelligence.SessionRepository)
	if err := repo.CreateDJSession(context.Background(), intelligence.SessionProfile{ID: "session", Mode: intelligence.RadioInfinite}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveDJCandidates(context.Background(), "session", "plan", candidates); err != nil {
		t.Fatal(err)
	}
	track := queue[0]
	track.DJSessionID = "session"
	track.DJPlanID = "plan"
	alternative, ok := a.resolvePlaybackFallback(track, string(track.Service), nil)
	if !ok || alternative.Track.Service == track.Service || alternative.Track.DJPlanID != "plan" {
		t.Fatal("stored fallback failed", alternative, ok)
	}
	// A replacement version still feeds the musical reaction profile.
	event := intelligence.FeedbackEvent{SessionID: "session", PlanID: "plan", TrackID: alternative.ID, Kind: intelligence.FeedbackLike}
	a.saveDJReaction(context.Background(), event, repo)
	raw, _, _ := a.store.SettingGet(context.Background(), "intelligence.private.reactions")
	var reactions []djReaction
	_ = json.Unmarshal([]byte(raw), &reactions)
	if len(reactions) != 1 {
		t.Fatal("fallback reaction lost", raw)
	}
}

func TestDJSourceFailuresAndPreferences(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(*App, map[domain.ServiceID]*versionService)
		want        domain.ServiceID
		stateSource domain.ServiceID
		state       string
	}{
		{"youtube_only", func(a *App, m map[domain.ServiceID]*versionService) {
			m[domain.ServiceYouTube].tracks = []domain.Track{versionTrack(domain.ServiceYouTube)}
		}, domain.ServiceYouTube, domain.ServiceYandex, "empty"},
		{"yandex_down", func(a *App, m map[domain.ServiceID]*versionService) {
			m[domain.ServiceYandex].err = errors.New("unavailable")
			m[domain.ServiceVK].tracks = []domain.Track{versionTrack(domain.ServiceVK)}
			m[domain.ServiceSoundCloud].tracks = []domain.Track{versionTrack(domain.ServiceSoundCloud)}
		}, domain.ServiceSoundCloud, domain.ServiceYandex, "search_error"},
		{"vk_no_session", func(a *App, m map[domain.ServiceID]*versionService) {
			m[domain.ServiceVK].auth = false
			m[domain.ServiceSoundCloud].tracks = []domain.Track{versionTrack(domain.ServiceSoundCloud)}
		}, domain.ServiceSoundCloud, domain.ServiceVK, "no_auth"},
		{"soundcloud_error", func(a *App, m map[domain.ServiceID]*versionService) {
			m[domain.ServiceSoundCloud].err = errors.New("private/deleted/rate limit")
			m[domain.ServiceYouTube].tracks = []domain.Track{versionTrack(domain.ServiceYouTube)}
		}, domain.ServiceYouTube, domain.ServiceSoundCloud, "search_error"},
		{"youtube_stream_failure", func(a *App, m map[domain.ServiceID]*versionService) {
			m[domain.ServiceYouTube].tracks = []domain.Track{versionTrack(domain.ServiceYouTube)}
			m[domain.ServiceVK].tracks = []domain.Track{versionTrack(domain.ServiceVK)}
			setVersionPolicy(t, a, DJSourcePolicy{Priority: []domain.ServiceID{domain.ServiceYouTube}})
			a.djVersionProbe = func(_ context.Context, tr domain.Track) error {
				if tr.Service == domain.ServiceYouTube {
					return errors.New("stream failed")
				}
				return nil
			}
		}, domain.ServiceVK, domain.ServiceYouTube, "unplayable"},
		{"explicit_priority", func(a *App, m map[domain.ServiceID]*versionService) {
			for _, id := range djVersionSources[:4] {
				m[id].tracks = []domain.Track{versionTrack(id)}
			}
			setVersionPolicy(t, a, DJSourcePolicy{Priority: []domain.ServiceID{domain.ServiceYouTube, domain.ServiceVK, domain.ServiceYandex}})
		}, domain.ServiceYouTube, domain.ServiceYouTube, "selected"},
		{"disabled", func(a *App, m map[domain.ServiceID]*versionService) {
			for _, id := range djVersionSources[:4] {
				m[id].tracks = []domain.Track{versionTrack(id)}
			}
			setVersionPolicy(t, a, DJSourcePolicy{Enabled: map[domain.ServiceID]bool{domain.ServiceYandex: false}, Priority: []domain.ServiceID{domain.ServiceVK}})
		}, domain.ServiceVK, domain.ServiceYandex, "disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, m := versionApp(t)
			tc.setup(a, m)
			// Metadata-only anchor must not manufacture an available Yandex item.
			anchor := versionTrack("metadata")
			d := DJDiagnostic{}
			queue, _ := a.resolveDJVersions(context.Background(), []domain.Track{anchor}, []intelligence.Candidate{versionCandidate(anchor)}, &d)
			if len(queue) != 1 || queue[0].Service != tc.want {
				t.Fatal(queue, d)
			}
			if got := sourceDiagnostic(t, d, tc.stateSource); got.State != tc.state {
				t.Fatal(got)
			}
			if tc.state == "disabled" || tc.state == "no_auth" {
				if m[tc.stateSource].calls.Load() != 0 {
					t.Fatal("queried unavailable source")
				}
			}
		})
	}
}

func TestDJWrongRecordingNeverQueued(t *testing.T) {
	for _, variant := range []string{"other_artist", "live", "remix", "acoustic", "remaster", "duration"} {
		t.Run(variant, func(t *testing.T) {
			a, m := versionApp(t)
			tr := versionTrack(domain.ServiceYouTube)
			switch variant {
			case "other_artist":
				tr.Artists = []string{"Wrong"}
			case "duration":
				tr.DurationMs = 450000
			default:
				tr.Title += " (" + variant + ")"
			}
			m[domain.ServiceYouTube].tracks = []domain.Track{tr}
			anchor := versionTrack("metadata")
			d := DJDiagnostic{}
			queue, _ := a.resolveDJVersions(context.Background(), []domain.Track{anchor}, []intelligence.Candidate{versionCandidate(anchor)}, &d)
			if len(queue) != 0 || sourceDiagnostic(t, d, domain.ServiceYouTube).Matched != 0 {
				t.Fatal(queue, d)
			}
		})
	}
}

func TestDJBoundedSlowSourceAndParallelPartialResults(t *testing.T) {
	a, m := versionApp(t)
	m[domain.ServiceVK].wait = true
	m[domain.ServiceYouTube].tracks = []domain.Track{versionTrack(domain.ServiceYouTube)}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	anchor := versionTrack("metadata")
	d := DJDiagnostic{}
	_, _ = a.resolveDJVersions(ctx, []domain.Track{anchor}, []intelligence.Candidate{versionCandidate(anchor)}, &d)
	if time.Since(started) > time.Second || sourceDiagnostic(t, d, domain.ServiceYouTube).Matched != 1 {
		t.Fatal("slow source blocked results", d)
	}
}

func TestDJProbeBudgetDoesNotLeakConcurrencySlots(t *testing.T) {
	a, m := versionApp(t)
	for _, id := range djVersionSources {
		for j := 0; j < 20; j++ {
			track := versionTrack(id)
			track.ID = fmt.Sprint(j)
			m[id].tracks = append(m[id].tracks, track)
		}
	}
	var calls atomic.Int32
	a.djVersionProbe = func(context.Context, domain.Track) error { calls.Add(1); return errors.New("unavailable") }
	anchors := make([]domain.Track, 12)
	for i := range anchors {
		anchors[i] = versionTrack("metadata")
		anchors[i].ID = fmt.Sprint(i)
	}
	d := DJDiagnostic{}
	a.resolveDJVersions(context.Background(), anchors, nil, &d)
	if calls.Load() != 48 || len(a.djVersionSlots()) != 0 {
		t.Fatal("probe budget or slots leaked", calls.Load(), len(a.djVersionSlots()))
	}
}

func TestDJVKWithoutLoginUsesRealAdapterAuthState(t *testing.T) {
	a, m := versionApp(t)
	a.registry[domain.ServiceVK] = vkweb.New()
	m[domain.ServiceYouTube].tracks = []domain.Track{versionTrack(domain.ServiceYouTube)}
	anchor := versionTrack("")
	d := DJDiagnostic{}
	queue, _ := a.resolveDJVersions(context.Background(), []domain.Track{anchor}, nil, &d)
	status := sourceDiagnostic(t, d, domain.ServiceVK)
	if status.Auth != "missing" || status.State != "no_auth" || status.Queries != 0 || status.SearchAttempted || len(queue) != 1 {
		t.Fatal(status, queue)
	}
}

func TestDJSourceSettingsValidationAndRevision(t *testing.T) {
	a, _ := versionApp(t)
	if err := a.ConfigureIntelligence(DJConfig{Provider: "openai", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	revision := a.djConfig(context.Background()).Revision
	w := httptest.NewRecorder()
	a.APIHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/intelligence/sources", strings.NewReader(`{"enabled":{"vk":false},"priority":["youtube","local"]}`)))
	if w.Code != 200 || a.djSourcePolicy(context.Background()).Enabled[domain.ServiceVK] || a.djConfig(context.Background()).Revision != revision+1 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	a.APIHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/intelligence/sources", strings.NewReader(`{"priority":["youtube_music"]}`)))
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestDJRadioModesUseSharedVersionResolver(t *testing.T) {
	for _, mode := range []intelligence.RadioMode{intelligence.RadioTrack, intelligence.RadioInfinite} {
		t.Run(string(mode), func(t *testing.T) {
			a, m := versionApp(t)
			seed := versionTrack(domain.ServiceYandex)
			seed.ID = "seed"
			seed.Title = "Seed"
			anchor := versionTrack(domain.ServiceYandex)
			if err := a.store.FavoriteAdd(context.Background(), anchor); err != nil {
				t.Fatal(err)
			}
			a.reco.SetTrackSimilarity(func(context.Context, domain.Track) ([]domain.Track, error) { return []domain.Track{anchor}, nil })
			a.reco.SetTrackSimilarityAvailability(func() bool { return true })
			m[domain.ServiceYouTube].tracks = []domain.Track{versionTrack(domain.ServiceYouTube)}
			setVersionPolicy(t, a, DJSourcePolicy{Priority: []domain.ServiceID{domain.ServiceYouTube}})
			if err := a.ConfigureIntelligence(DJConfig{Enabled: true, Provider: "openai"}); err != nil {
				t.Fatal(err)
			}
			cfg := a.djConfig(context.Background())
			session := intelligence.SessionProfile{ID: fmt.Sprint("mode-", mode), Mode: mode}
			if mode == intelligence.RadioTrack {
				summary := djSummary(seed)
				session.Seed = &summary
			}
			repo := a.store.(intelligence.SessionRepository)
			if err := repo.CreateDJSession(context.Background(), session); err != nil {
				t.Fatal(err)
			}
			a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(context.Background()), djRadioRequest{SessionID: session.ID, Track: seed, Mode: mode, Limit: 1}, nil)
			a.dj.Lock()
			done := a.dj.done
			a.dj.Unlock()
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				t.Fatal("radio job stuck")
			}
			a.dj.Lock()
			ready := a.dj.ready[session.ID]
			a.dj.Unlock()
			if len(ready.tracks) != 1 || ready.tracks[0].Service != domain.ServiceYouTube {
				t.Fatal("shared radio resolution not applied", ready, a.loadDJDiagnostic(context.Background(), cfg.Revision))
			}
			if m[domain.ServiceVK].calls.Load() != 1 || m[domain.ServiceYouTube].calls.Load() != 1 {
				t.Fatal("catalogs not checked")
			}
		})
	}
}
