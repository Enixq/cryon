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

func TestDJEmptyContextQueryReachesSourceAndSavesResult(t *testing.T) {
	logging.SetDataDirOverride(t.TempDir())
	defer logging.SetDataDirOverride("")
	st, err := sqlite.New("query-contract")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err = st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	var modelCalls, sourceCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal([]byte(req.Messages[1].Content), &fields)
		var reply any
		if _, ok := fields["pool_id"]; ok {
			var in intelligence.SelectionInput
			_ = json.Unmarshal([]byte(req.Messages[1].Content), &in)
			if len(in.Candidates) != 1 {
				t.Error("candidate missing")
				http.Error(w, "bad pool", 400)
				return
			}
			reply = intelligence.SelectionPlan{Scope: in.Context.Scope, PoolID: in.PoolID, Ranked: []intelligence.CandidateDecision{{ID: in.Candidates[0].ID, Score: .95, Reason: "Связь с исходным треком"}}, Avoid: []string{}, Directions: []intelligence.Direction{}}
		} else {
			var in intelligence.PlanningInput
			_ = json.Unmarshal([]byte(req.Messages[1].Content), &in)
			reply = intelligence.SearchPlan{Scope: in.Scope, Strategy: intelligence.Strategy{SeedAdherence: .8, Diversity: .5}, Steps: []intelligence.SearchStep{{Channel: intelligence.Channel{Provider: "yandex", Capability: "similar_tracks"}, Query: "", Limit: 10}}}
		}
		raw, _ := json.Marshal(reply)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(raw)}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	a := &App{ctx: ctx, store: st, reco: recommendations.New(st, nil, nil, nil)}
	a.reco.SetTrackSimilarity(func(context.Context, domain.Track) ([]domain.Track, error) {
		sourceCalls.Add(1)
		return []domain.Track{{ID: "new", Title: "Neighbor", Artists: []string{"New artist"}, Service: domain.ServiceYandex, DurationMs: 180000}}, nil
	})
	cfg := DJConfig{Enabled: true, Provider: "compatible", Model: "mock", APIKey: "test", BaseURL: server.URL, Revision: 1, DailySegments: 10}
	raw, _ := json.Marshal(cfg)
	_ = st.SettingSet(ctx, djConfigKey, string(raw))
	seed := domain.Track{ID: "seed", Title: "Seed", Artists: []string{"Anchor"}, Service: domain.ServiceYandex}
	summary := djSummary(seed)
	session := intelligence.SessionProfile{ID: "session", Mode: intelligence.RadioTrack, Seed: &summary}
	if err = st.CreateDJSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(ctx), djRadioRequest{Collection: true, Track: seed, Mode: session.Mode, Limit: 1}, nil)
	a.dj.Lock()
	done := a.dj.done
	a.dj.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker leaked")
	}
	result := a.loadDJCollection(ctx)
	if result.Job.Stage != "ready" || len(result.Tracks) != 1 || modelCalls.Load() != 1 || sourceCalls.Load() != 1 {
		t.Fatal(result.Job, modelCalls.Load(), sourceCalls.Load())
	}
}
