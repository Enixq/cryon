package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/logging"
	"Cryon2/internal/recommendations"
	"Cryon2/internal/services/lastfm"
	"Cryon2/internal/store/sqlite"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDJSettingsNeverExposeKey(t *testing.T) {
	a := &App{ctx: context.Background(), store: &notificationFeedbackStore{settings: map[string]string{}}}
	c := DJConfig{Enabled: true, Provider: "openai", Model: "model", APIKey: "private-test-key", DailySegments: 10}
	if err := a.ConfigureIntelligence(c); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(a.IntelligenceStatus())
	if strings.Contains(string(raw), c.APIKey) {
		t.Fatal("key leaked")
	}
	if _, err := a.GetSetting(djConfigKey); err == nil {
		t.Fatal("generic getter exposes secret")
	}
	if err := a.SetSetting(djConfigKey, "{}"); err == nil {
		t.Fatal("generic setter bypasses validation")
	}
	c.APIKey = ""
	if err := a.ConfigureIntelligence(c); err != nil {
		t.Fatal("same endpoint lost key", err)
	}
	c.BaseURL = "https://other.example/v1"
	if err := a.ConfigureIntelligence(c); err == nil {
		t.Fatal("key reused for other endpoint")
	}
	rec := httptest.NewRecorder()
	a.APIHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/intelligence/config", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private-test-key") {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

func TestDJSubmissionHonorsConfigurationAndPersistedBudget(t *testing.T) {
	ctx := context.Background()
	st := &notificationFeedbackStore{settings: map[string]string{}}
	a := &App{ctx: ctx, store: st}
	cfg := DJConfig{Enabled: true, Provider: "openai", Model: "test", APIKey: "test", Revision: 2, DailySegments: 1}
	raw, _ := json.Marshal(cfg)
	st.settings[djConfigKey] = string(raw)
	usage, _ := json.Marshal(djUsage{Day: time.Now().UTC().Format("2006-01-02"), Used: 1, Tokens: 123})
	st.settings["intelligence.private.usage"] = string(usage)
	a.prepareDJ(cfg, intelligence.SessionProfile{ID: "session"}, intelligence.TasteProfile{}, djRadioRequest{}, nil)
	if a.dj.busy || a.dj.done != nil {
		t.Fatal("budget bypass after restart")
	}
	if status := a.IntelligenceStatus(); status.UsedToday != 1 || status.TokensToday != 123 {
		t.Fatal(status)
	}
	st.settings["intelligence.private.usage"] = "{}"
	a.dj.day = ""
	old := cfg
	old.Revision--
	a.prepareDJ(old, intelligence.SessionProfile{ID: "session"}, intelligence.TasteProfile{}, djRadioRequest{}, nil)
	if a.dj.done != nil {
		t.Fatal("stale configuration launched worker")
	}
	cfg.Enabled = false
	raw, _ = json.Marshal(cfg)
	st.settings[djConfigKey] = string(raw)
	a.prepareDJ(cfg, intelligence.SessionProfile{ID: "session"}, intelligence.TasteProfile{}, djRadioRequest{}, nil)
	if a.dj.done != nil {
		t.Fatal("disabled DJ launched worker")
	}
}

func TestDJBackgroundPlanSearchSelectAndConsume(t *testing.T) {
	logging.SetDataDirOverride(t.TempDir())
	defer logging.SetDataDirOverride("")
	st, err := sqlite.New("dj-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{ctx: ctx, store: st, platform: nullHost{}}
	defer func() {
		cancel()
		a.dj.Lock()
		done := a.dj.done
		a.dj.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("worker leaked")
			}
		}
		st.Close()
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var input map[string]json.RawMessage
		_ = json.Unmarshal([]byte(request.Messages[1].Content), &input)
		var output any
		if _, ok := input["pool_id"]; ok {
			var selection intelligence.SelectionInput
			_ = json.Unmarshal([]byte(request.Messages[1].Content), &selection)
			output = intelligence.SelectionPlan{Scope: selection.Context.Scope, PoolID: selection.PoolID, Ranked: []intelligence.CandidateDecision{{ID: selection.Candidates[0].ID, Score: .9, Reason: "Подходит текущей сессии"}}, Avoid: []string{}, Directions: []intelligence.Direction{}}
		} else {
			var planning intelligence.PlanningInput
			_ = json.Unmarshal([]byte(request.Messages[1].Content), &planning)
			output = intelligence.SearchPlan{Scope: planning.Scope, Strategy: intelligence.Strategy{Exploration: .3, SeedAdherence: .9, Diversity: .5}, Steps: []intelligence.SearchStep{{Channel: intelligence.Channel{Provider: "cryon", Capability: "catalog_search"}, Query: "atmospheric rock", Limit: 2}}}
		}
		raw, _ := json.Marshal(output)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(raw)}}}})
	}))
	defer server.Close()
	a.reco = recommendations.New(st, lastfm.New(""), nil, func(context.Context, string) ([]domain.Track, error) {
		return []domain.Track{{ID: "new", Service: domain.ServiceSoundCloud, Title: "New", Artists: []string{"Unknown artist"}, PlayableKind: domain.PlayableStream}}, context.DeadlineExceeded
	})
	cfg := DJConfig{Enabled: true, Provider: "compatible", Model: "test", APIKey: "test", BaseURL: server.URL, Revision: 1, DailySegments: 10}
	raw, _ := json.Marshal(cfg)
	if err = st.SettingSet(ctx, djConfigKey, string(raw)); err != nil {
		t.Fatal(err)
	}
	session := intelligence.SessionProfile{ID: "session", Mode: intelligence.RadioTrack, Seed: &intelligence.TrackSummary{ID: "soundcloud:seed"}}
	if err = st.CreateDJSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	req := djRadioRequest{SessionID: session.ID, Mode: intelligence.RadioTrack, Track: domain.Track{ID: "seed", Service: domain.ServiceSoundCloud, Title: "Seed", Artists: []string{"Seed Artist"}}, Limit: 2}
	start := time.Now()
	a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(ctx), req, nil)
	if time.Since(start) > time.Second {
		t.Fatal("submission blocked")
	}
	a.dj.Lock()
	done := a.dj.done
	a.dj.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("segment not prepared")
	}
	a.dj.Lock()
	prepared := a.dj.prepared
	a.dj.Unlock()
	if prepared != 1 {
		t.Fatal("planner/selector pipeline failed")
	}
	diagnostic := a.IntelligenceStatus().Diagnostic
	if diagnostic.Stage != "ready" || diagnostic.Found != 1 || diagnostic.Selected != 1 || len(diagnostic.Searches) != 1 || diagnostic.Searches[0].State != "error" {
		t.Fatal("partial source results or diagnostics lost", diagnostic)
	}
	// A reaction after preparation must not discard otherwise usable AI work.
	_, err = st.RecordDJFeedback(ctx, intelligence.FeedbackEvent{ID: "reaction", SessionID: session.ID, TrackID: "soundcloud:other", Kind: intelligence.FeedbackCompleted, At: time.Now(), PlayedMS: 10000, DurationMS: 10000})
	if err != nil {
		t.Fatal(err)
	}
	view := httptest.NewRecorder()
	a.APIHandler().ServeHTTP(view, httptest.NewRequest(http.MethodGet, "/v1/intelligence/collection", nil))
	if view.Code != 200 || !strings.Contains(view.Body.String(), "djPlanId") {
		t.Fatal("AI collection not visible", view.Body.String())
	}
	if restarted := (&App{ctx: ctx, store: st}).loadDJCollection(ctx); len(restarted.Tracks) != 1 || restarted.UpdatedAt == "" {
		t.Fatal("AI collection lost after restart", restarted)
	}
	req.PreparedOnly = true
	out, err := a.djRadio(ctx, req)
	if err != nil || len(out.Tracks) != 1 || out.Tracks[0].DJPlanID == "" || !strings.Contains(out.Tracks[0].RecommendationReason, "Cryon DJ") {
		t.Fatal(out, err)
	}
}
