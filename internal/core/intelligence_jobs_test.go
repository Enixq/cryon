package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/logging"
	"Cryon2/internal/recommendations"
	"Cryon2/internal/store/sqlite"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDJRestartRecoversInterruptedJobAndRejectsStaleDiagnostic(t *testing.T) {
	ctx := context.Background()
	st := &notificationFeedbackStore{settings: map[string]string{}}
	a := &App{ctx: ctx, store: st}
	raw, _ := json.Marshal(djCollection{JobID: "completed", ResultID: "result", Tracks: []domain.Track{{ID: "old"}}})
	st.settings["intelligence.collection"] = string(raw)
	a.saveDJDiagnostic(ctx, DJDiagnostic{JobID: "interrupted", Mode: "collection", Stage: "planning"})
	a.saveDJDiagnostic(ctx, DJDiagnostic{JobID: "obsolete", Mode: "collection", Stage: "ready"})
	view := httptest.NewRecorder()
	a.handleDJCollection(view, httptest.NewRequest("GET", "/", nil))
	var got djCollection
	_ = json.Unmarshal(view.Body.Bytes(), &got)
	if got.Job.Stage != "cancelled" || got.Job.JobID != "interrupted" || got.ResultID != "result" || len(got.Tracks) != 1 {
		t.Fatal(got)
	}
}

func TestDJManualRestartCancelsWorkerWithoutCooldownOrOldResultLoss(t *testing.T) {
	ctx := context.Background()
	logging.SetDataDirOverride(t.TempDir())
	defer logging.SetDataDirOverride("")
	st, err := sqlite.New("dj-jobs")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
		http.Error(w, "invalid model", 400)
	}))
	defer server.Close()
	defer close(release)
	a := &App{ctx: ctx, store: st, reco: recommendations.New(st, nil, nil, nil)}
	cfg := DJConfig{Enabled: true, Provider: "compatible", Model: "mock", APIKey: "mock", BaseURL: server.URL, Revision: 1, DailySegments: 10}
	raw, _ := json.Marshal(cfg)
	_ = st.SettingSet(ctx, djConfigKey, string(raw))
	session := intelligence.SessionProfile{ID: "manual", Mode: intelligence.RadioInfinite}
	if err := st.CreateDJSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	_ = st.SettingSet(ctx, "intelligence.collection.session", session.ID)
	raw, _ = json.Marshal(djCollection{JobID: "old", ResultID: "old-result", Tracks: []domain.Track{{ID: "old"}}})
	_ = st.SettingSet(ctx, "intelligence.collection", string(raw))
	a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(ctx), djRadioRequest{Collection: true, SessionID: session.ID, Mode: session.Mode, Limit: 10}, nil)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	first := a.loadDJCollection(ctx).Job.JobID
	rec := httptest.NewRecorder()
	a.handleDJCollection(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	a.dj.Lock()
	done := a.dj.done
	a.dj.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement worker leaked")
	}
	current := a.loadDJCollection(ctx)
	if current.Job.JobID == first || current.Job.Stage != "failed" || current.ResultID != "old-result" || calls.Load() != 2 {
		t.Fatal("replacement not launched or prior result lost", current, calls.Load())
	}
}
