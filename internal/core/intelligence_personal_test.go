package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/logging"
	"Cryon2/internal/recommendations"
	"Cryon2/internal/store/sqlite"
)

func TestPersonalCollectionSurvivesMissingOrInvalidAI(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-key", true: "invalid-model-decision"}[enabled], func(t *testing.T) {
			logging.SetDataDirOverride(t.TempDir())
			defer logging.SetDataDirOverride("")
			st, err := sqlite.New("personal")
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			ctx := context.Background()
			if err = st.Init(ctx); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}],"usage":{"total_tokens":23}}`))
			}))
			defer server.Close()
			a := &App{ctx: ctx, store: st, reco: recommendations.New(st, nil, nil, nil)}
			a.reco.SetTrackSimilarity(func(context.Context, domain.Track) ([]domain.Track, error) {
				return []domain.Track{{ID: "new", Title: "New discovery", Artists: []string{"Unfamiliar"}, Service: domain.ServiceSoundCloud, DurationMs: 180000}}, nil
			})
			cfg := DJConfig{Enabled: enabled, Provider: "compatible", Model: "mock", BaseURL: server.URL, Revision: 1, DailySegments: 10}
			if enabled {
				cfg.APIKey = "test"
			}
			raw, _ := json.Marshal(cfg)
			_ = st.SettingSet(ctx, djConfigKey, string(raw))
			session := intelligence.SessionProfile{ID: "personal", Mode: intelligence.RadioTrack, Seed: &intelligence.TrackSummary{ID: "seed"}}
			if err = st.CreateDJSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(ctx), djRadioRequest{Collection: true, Mode: session.Mode, Limit: 10}, nil)
			a.dj.Lock()
			done := a.dj.done
			a.dj.Unlock()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not finish")
			}
			result := a.loadDJCollection(ctx)
			if enabled {
				if result.Job.Stage != "failed" || calls.Load() != 1 || result.Job.Tokens != 23 || len(result.Tracks) != 0 {
					t.Fatal("invalid planner must not execute sources", result.Job, calls.Load())
				}
				return
			}
			if result.Job.Stage != "ready" || len(result.Tracks) != 1 || result.Job.Engine != "personal" {
				t.Fatal(result)
			}
			if enabled && (calls.Load() != 1 || result.Job.Warning == "" || result.Job.Tokens != 23) {
				t.Fatal(result.Job, calls.Load())
			}
			if !enabled && calls.Load() != 0 {
				t.Fatal("paid request without key")
			}
		})
	}
}
