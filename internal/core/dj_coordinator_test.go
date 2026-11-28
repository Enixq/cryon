package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	djproviders "Cryon2/internal/intelligence/providers"
	"Cryon2/internal/logging"
	"Cryon2/internal/recommendations"
	"Cryon2/internal/store/sqlite"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func coordinatorApp(t *testing.T) *App {
	t.Helper()
	logging.SetDataDirOverride(t.TempDir())
	t.Cleanup(func() { logging.SetDataDirOverride("") })
	st, err := sqlite.New("coordinator")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err = st.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &App{ctx: context.Background(), store: st, reco: recommendations.New(st, nil, nil, nil)}
}

func TestDJStrategyCacheAndBudgetNeverBlockLocalSelection(t *testing.T) {
	a := coordinatorApp(t)
	ctx := context.Background()
	var calls atomic.Int32
	channel := intelligence.Channel{Provider: "yandex", Capability: "similar_tracks"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var in intelligence.PlanningInput
		_ = json.Unmarshal([]byte(body.Messages[1].Content), &in)
		plan := intelligence.SearchPlan{Scope: in.Scope, Strategy: intelligence.Strategy{SeedAdherence: .9}, Steps: []intelligence.SearchStep{{Channel: channel, Limit: 5}}}
		raw, _ := json.Marshal(plan)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(raw)}}}, "usage": map[string]int{"total_tokens": 12}})
	}))
	defer server.Close()
	cfg := DJConfig{Enabled: true, Provider: "compatible", Model: "mock", APIKey: "test", BaseURL: server.URL, Revision: 1, DailySegments: 1}
	director, _ := djproviders.New(cfg.Provider, cfg.Model, cfg.APIKey, cfg.BaseURL)
	in := intelligence.PlanningInput{Scope: intelligence.Scope{RequestID: "one", SessionID: "s"}, Session: intelligence.SessionProfile{ID: "s", Mode: intelligence.RadioTrack, Seed: &intelligence.TrackSummary{ID: "yandex:seed"}}, Channels: []intelligence.ChannelBudget{{Channel: channel, MaxCandidates: 10}}}
	d := DJDiagnostic{}
	a.djStrategy(ctx, cfg, in, director, &d)
	if calls.Load() != 1 || d.LLMCalls != 1 {
		t.Fatal("first strategy not planned", calls.Load(), d)
	}
	in.Scope.RequestID = "two"
	d = DJDiagnostic{}
	cached := a.djStrategy(ctx, cfg, in, director, &d)
	if calls.Load() != 1 || !d.PlanFromCache || cached.Scope.RequestID != "two" {
		t.Fatal("cache miss or stale scope", calls.Load(), d, cached)
	}
	// A changed profile invalidates the plan, but an exhausted model budget must
	// still allow actual acquisition, local ranking and playback candidates.
	in.Taste.Revision = 2
	in.Scope.TasteRevision = 2
	in.Taste.BlockedArtists = []string{"Changed"}
	a.reco.SetTrackSimilarity(func(context.Context, domain.Track) ([]domain.Track, error) {
		return []domain.Track{{ID: "new", Title: "New", Artists: []string{"New artist"}, Service: domain.ServiceSoundCloud}}, nil
	})
	d = DJDiagnostic{}
	out := a.coordinateDJ(ctx, cfg, in, djRadioRequest{Mode: intelligence.RadioTrack, Limit: 5}, nil, director, &d)
	if len(out.tracks) != 1 || calls.Load() != 1 || d.Engine != "local" || d.Warning == "" {
		t.Fatal("budget broke local DJ", out, d, calls.Load())
	}
}

func TestDJSlowToolCannotDiscardFastChannel(t *testing.T) {
	a := coordinatorApp(t)
	blocked := make(chan struct{})
	defer close(blocked)
	a.reco.SetDJSourceSearch(func() []string { return []string{"slow", "fast"} }, func(_ context.Context, source, _ string) ([]domain.Track, error) {
		if source == "slow" {
			<-blocked
			return nil, nil
		}
		return []domain.Track{{ID: "fast", Title: "Fast", Artists: []string{"A"}, Service: domain.ServiceSoundCloud}}, nil
	})
	plan := intelligence.SearchPlan{Steps: []intelligence.SearchStep{{Channel: intelligence.Channel{Provider: "slow", Capability: "catalog_search"}, Limit: 5}, {Channel: intelligence.Channel{Provider: "fast", Capability: "catalog_search"}, Limit: 5}}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	rows := a.djTools(ctx, plan, domain.Track{}, 0)
	if time.Since(start) > time.Second || len(rows[1].tracks) != 1 || rows[0].err != context.DeadlineExceeded {
		t.Fatal("slow provider starved completed results", rows)
	}
}

func TestOrdinaryBehaviorIsIdempotentAndProfileRebuilds(t *testing.T) {
	a := coordinatorApp(t)
	body := `{"track":{"id":"song","service":"soundcloud","title":"Song","artists":["New artist"],"playableKind":"stream"},"event":{"id":"unique","kind":"like"}}`
	before := a.reco.IntelligenceTaste(context.Background())
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.handleDJBehavior(w, httptest.NewRequest("POST", "/v1/intelligence/behavior", strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	after := a.reco.IntelligenceTaste(context.Background())
	if before.Revision == after.Revision || len(after.Preferences) != 1 {
		t.Fatal("ordinary behavior not reflected", before, after)
	}
	raw, _, _ := a.store.SettingGet(context.Background(), "intelligence.private.reactions")
	var rows []djReaction
	_ = json.Unmarshal([]byte(raw), &rows)
	if len(rows) != 1 {
		t.Fatal("duplicate behavior applied twice")
	}
	a.saveDJSuggestions(context.Background(), []intelligence.TrackSummary{{ID: "one"}})
	a.saveDJSuggestions(context.Background(), []intelligence.TrackSummary{{ID: "two"}})
	if len(a.recentDJSuggestions(context.Background())) != 2 {
		t.Fatal("recent suggestions replaced instead of accumulated")
	}
}

func TestDJEnoughNeighborsCancelSlowSearchBeforeDeadline(t *testing.T) {
	a := coordinatorApp(t)
	a.reco.SetDJSourceSearch(func() []string { return []string{"slow"} }, func(ctx context.Context, _, _ string) ([]domain.Track, error) { <-ctx.Done(); return nil, ctx.Err() })
	a.reco.SetTrackSimilarity(func(context.Context, domain.Track) ([]domain.Track, error) {
		rows := []domain.Track{}
		for i := 0; i < 4; i++ {
			rows = append(rows, domain.Track{ID: fmt.Sprint(i), Title: fmt.Sprint(i), Artists: []string{"A"}, Service: domain.ServiceYandex})
		}
		return rows, nil
	})
	plan := intelligence.SearchPlan{Steps: []intelligence.SearchStep{{Channel: intelligence.Channel{Provider: "yandex", Capability: "similar_tracks"}, Limit: 4}, {Channel: intelligence.Channel{Provider: "slow", Capability: "catalog_search"}, Query: "q", Limit: 4}}}
	start := time.Now()
	rows := a.djTools(context.Background(), plan, domain.Track{}, 4)
	if time.Since(start) > 3*time.Second || len(rows[0].tracks) != 4 || rows[1].err != context.Canceled {
		t.Fatal("enough candidates still waited for slow source", rows)
	}
}
