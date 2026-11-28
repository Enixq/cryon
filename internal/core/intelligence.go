package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	djproviders "Cryon2/internal/intelligence/providers"
)

const djConfigKey = "intelligence.private.config"

type djUsage struct {
	Day    string
	Used   int
	Tokens int
}

type DJConfig struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	BaseURL       string `json:"baseUrl"`
	APIKey        string `json:"apiKey,omitempty"`
	Revision      uint64 `json:"revision"`
	DailySegments int    `json:"dailySegments"`
}
type DJStatus struct {
	Diagnostic    DJDiagnostic   `json:"diagnostic"`
	TokensToday   int            `json:"tokensToday"`
	UsedToday     int            `json:"usedToday"`
	Events        map[string]int `json:"events"`
	Enabled       bool           `json:"enabled"`
	Configured    bool           `json:"configured"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	BaseURL       string         `json:"baseUrl"`
	DailySegments int            `json:"dailySegments"`
	State         string         `json:"state"`
	Prepared      uint64         `json:"prepared"`
	Fallbacks     uint64         `json:"fallbacks"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
	TrackCount    int            `json:"trackCount"`
}
type djReady struct {
	tracks []domain.Track
	scope  intelligence.Scope
	at     time.Time
}
type djPending struct {
	req    djRadioRequest
	issued []domain.Track
}
type djState struct {
	sync.Mutex
	jobID        string
	sessionID    string
	busy         bool
	cancel       context.CancelFunc
	ready        map[string]djReady
	cooldown     time.Time
	day          string
	used         int
	tokens       int
	state        string
	prepared     uint64
	fallbacks    uint64
	pending      map[string]djPending
	done         chan struct{}
	toolSlots    chan struct{}
	versionSlots chan struct{}
}

func djID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (a *App) djConfig(ctx context.Context) DJConfig {
	c := DJConfig{Provider: "openai", DailySegments: 100}
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, djConfigKey)
		_ = json.Unmarshal([]byte(raw), &c)
	}
	return c
}
func (a *App) IntelligenceStatus() DJStatus {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := a.djConfig(ctx)
	usedToday := 0
	tokensToday := 0
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.usage")
		var usage djUsage
		if json.Unmarshal([]byte(raw), &usage) == nil && usage.Day == time.Now().UTC().Format("2006-01-02") {
			usedToday = usage.Used
			tokensToday = usage.Tokens
		}
	}
	a.dj.Lock()
	defer a.dj.Unlock()
	state := a.dj.state
	if !c.Enabled {
		state = "disabled"
	} else if state == "" {
		state = "ready"
	}
	events := map[string]int{}
	if repo, ok := a.store.(intelligence.PlanRepository); ok {
		if counts, err := repo.DJMetrics(ctx); err == nil {
			events = counts
		}
	}
	latest := a.loadDJCollection(ctx)
	return DJStatus{Diagnostic: a.loadDJDiagnostic(ctx, c.Revision), UpdatedAt: latest.UpdatedAt, TrackCount: len(latest.Tracks), Enabled: c.Enabled, Configured: c.APIKey != "", Provider: c.Provider, Model: c.Model, BaseURL: c.BaseURL, DailySegments: c.DailySegments, State: state, Prepared: a.dj.prepared, Fallbacks: a.dj.fallbacks, UsedToday: usedToday, TokensToday: tokensToday, Events: events}
}
func (a *App) ConfigureIntelligence(c DJConfig) error {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	c.Provider = strings.TrimSpace(c.Provider)
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	old := a.djConfig(ctx)
	if c.Provider != "openai" && c.Provider != "deepseek" && c.Provider != "anthropic" && c.Provider != "compatible" {
		return fmt.Errorf("неизвестный AI-провайдер")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("API endpoint должен быть HTTPS URL без пароля и параметров")
		}
	}
	if c.Provider == "compatible" && c.BaseURL == "" {
		return fmt.Errorf("укажите API endpoint")
	}
	if c.APIKey == "" && old.Provider == c.Provider && old.BaseURL == c.BaseURL {
		c.APIKey = old.APIKey
	}
	if c.Enabled && ((c.APIKey != "" && c.Model == "") || (old.APIKey != "" && c.APIKey == "" && (old.Provider != c.Provider || old.BaseURL != c.BaseURL))) {
		return fmt.Errorf("укажите модель и ключ; при смене endpoint введите ключ заново")
	}
	if c.DailySegments <= 0 {
		c.DailySegments = 100
	}
	if c.DailySegments > 500 {
		c.DailySegments = 500
	}
	c.Revision = old.Revision + 1
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	a.dj.Lock()
	defer a.dj.Unlock()
	if err = a.store.SettingSet(ctx, djConfigKey, string(raw)); err != nil {
		return err
	}
	if a.dj.cancel != nil {
		a.dj.cancel()
	}
	a.dj.ready = nil
	a.dj.pending = nil
	a.dj.cooldown = time.Time{}
	a.dj.state = "ready"
	return nil
}

type djRadioRequest struct {
	Collection   bool                   `json:"-"`
	PreparedOnly bool                   `json:"preparedOnly"`
	SessionID    string                 `json:"sessionId"`
	Mode         intelligence.RadioMode `json:"mode"`
	Track        domain.Track           `json:"track"`
	Exclude      []domain.Track         `json:"exclude"`
	Limit        int                    `json:"limit"`
}
type djRadioResult struct {
	SessionID string         `json:"sessionId"`
	Tracks    []domain.Track `json:"tracks"`
}

func (a *App) djRadio(ctx context.Context, req djRadioRequest) (djRadioResult, error) {
	result := djRadioResult{Tracks: []domain.Track{}}
	if a.reco == nil {
		return result, nil
	}
	req.Limit = max(1, min(req.Limit, 40))
	if req.Mode == "" {
		req.Mode = intelligence.RadioTrack
	}
	cfg := a.djConfig(ctx)
	fallback := func() ([]domain.Track, error) {
		return a.reco.RelatedTracks(ctx, req.Track, req.Limit, req.Exclude...)
	}
	repo, ok := a.store.(intelligence.SessionRepository)
	if !cfg.Enabled || !ok {
		if req.PreparedOnly {
			return result, nil
		}
		tracks, err := fallback()
		result.Tracks = tracks
		return result, err
	}
	session, exists, err := repo.DJSession(ctx, req.SessionID)
	if err != nil {
		return result, err
	}
	if exists && (session.Mode != req.Mode || req.Mode == intelligence.RadioTrack && (session.Seed == nil || session.Seed.ID != djTrackID(req.Track))) {
		return result, fmt.Errorf("сессия принадлежит другому радио")
	}
	if !exists {
		session = intelligence.SessionProfile{ID: djID(), Mode: req.Mode, PlayedIDs: []string{}}
		if req.Mode == intelligence.RadioTrack {
			s := djSummary(req.Track)
			session.Seed = &s
		}
		if err = repo.CreateDJSession(ctx, session); err != nil {
			return result, err
		}
	}
	result.SessionID = session.ID
	a.dj.Lock()
	ready, hit := a.dj.ready[session.ID]
	delete(a.dj.ready, session.ID)
	a.dj.Unlock()
	if hit && time.Since(ready.at) < 10*time.Minute && ready.scope.ConfigRevision == cfg.Revision {
		excluded := append([]domain.Track{}, req.Exclude...)
		if req.Mode == intelligence.RadioTrack {
			excluded = append(excluded, req.Track)
		}
		played := map[string]bool{}
		for _, id := range session.PlayedIDs {
			played[id] = true
		}
		fresh := []domain.Track{}
		for _, t := range ready.tracks {
			if !played[djTrackID(t)] && !played[t.ID] {
				fresh = append(fresh, t)
			}
		}
		result.Tracks = a.reco.FinalizeDJ(ctx, fresh, excluded, req.Limit)
	}
	// Start planning before source resolution; AI never extends the request path.
	if !hit {
		go func() {
			defer a.recoverGoroutine("intelligence prepare")
			a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(context.WithoutCancel(ctx)), req, nil)
		}()
	}
	// Enabled DJ never substitutes ordinary radio for its unfinished result.

	// Refill the next unissued segment after consuming the ready queue.
	if hit {
		issued := append([]domain.Track{}, result.Tracks...)
		go func() {
			defer a.recoverGoroutine("intelligence prepare")
			a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(context.WithoutCancel(ctx)), req, issued)
		}()
	}
	return result, nil
}
func djTrackID(t domain.Track) string { return string(t.Service) + ":" + t.ID }
func djSummary(t domain.Track) intelligence.TrackSummary {
	tags := []string{}
	if t.Genre != "" {
		tags = append(tags, t.Genre)
	}
	return intelligence.TrackSummary{ID: djTrackID(t), Title: t.Title, Artists: t.Artists, Tags: tags, DurationMS: t.DurationMs}
}

func (a *App) prepareDJ(cfg DJConfig, session intelligence.SessionProfile, taste intelligence.TasteProfile, req djRadioRequest, issued []domain.Track) {
	if _, ok := a.store.(intelligence.SessionRepository); !ok || a.reco == nil {
		return
	}
	a.dj.Lock()
	req.Limit = max(1, min(req.Limit, 40))
	currentConfig := a.djConfig(context.Background())
	if currentConfig.Revision != cfg.Revision || (!req.Collection && !currentConfig.Enabled) {
		a.dj.Unlock()
		return
	}
	if a.dj.pending == nil {
		a.dj.pending = map[string]djPending{}
	}
	if len(a.dj.pending) > 8 {
		a.dj.pending = map[string]djPending{}
	}
	a.dj.pending[session.ID] = djPending{req, append([]domain.Track{}, issued...)}
	day := time.Now().UTC().Format("2006-01-02")
	if day != a.dj.day {
		a.dj.day = day
		a.dj.used = 0
		a.dj.tokens = 0
		if a.store != nil {
			raw, _, _ := a.store.SettingGet(context.Background(), "intelligence.private.usage")
			var usage djUsage
			if json.Unmarshal([]byte(raw), &usage) == nil && usage.Day == day {
				a.dj.used = usage.Used
				a.dj.tokens = usage.Tokens
			}
		}
	}
	if a.dj.busy || time.Now().Before(a.dj.cooldown) {
		a.dj.Unlock()
		return
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 180*time.Second)
	a.dj.busy = true
	a.dj.done = make(chan struct{})
	done := a.dj.done
	a.dj.cancel = cancel

	usage, _ := json.Marshal(djUsage{day, a.dj.used, a.dj.tokens})
	if err := a.store.SettingSet(ctx, "intelligence.private.usage", string(usage)); err != nil {
		a.dj.busy = false
		cancel()
		close(done)
		a.dj.Unlock()
		return
	}
	a.dj.state = "preparing"
	jobID := djID()
	a.dj.jobID = jobID
	a.dj.sessionID = session.ID
	mode := string(session.Mode)
	if req.Collection {
		mode = "collection"
	}
	diagnostic := DJDiagnostic{JobID: jobID, SessionID: session.ID, Mode: mode, ConfigRevision: cfg.Revision, Stage: "planning", Searches: []DJSearchStatus{}}
	a.saveDJDiagnostic(ctx, diagnostic)
	a.dj.Unlock()
	go func() {
		defer close(done)
		defer a.recoverGoroutine("intelligence DJ")
		success := false

		defer func() {
			diagnostic.RequestDeadlineAt = ""
			if !success && diagnostic.Error == "" {
				if ctx.Err() != nil {
					diagnostic.Error = "Подготовка прервана или превышено время ожидания"
				} else {
					diagnostic.Error = "Не удалось сохранить результат подготовки"
				}
			}
			if success {
				diagnostic.Stage = "ready"
			} else {
				diagnostic.Stage = "failed"
				if ctx.Err() == context.Canceled {
					diagnostic.Stage = "cancelled"
				}
			}
			a.saveDJDiagnostic(ctx, diagnostic)
			cancel()
			a.dj.Lock()
			a.dj.busy = false
			a.dj.cancel = nil
			if !success {
				a.dj.state = "fallback"
				a.dj.cooldown = time.Now().Add(time.Minute)
			} else {
				a.dj.state = "ready"
			}
			a.dj.Unlock()
		}()
		repo := a.store.(intelligence.SessionRepository)
		recent, err := repo.DJFeedback(ctx, session.ID, 200)
		if err != nil {
			return
		}
		reactions := []intelligence.FeedbackEvent{}
		for _, event := range recent {
			if event.Kind == intelligence.FeedbackLike || event.Kind == intelligence.FeedbackDislike || event.Kind == intelligence.FeedbackSkip || event.Kind == intelligence.FeedbackCompleted || event.Kind == intelligence.FeedbackReplay {
				reactions = append(reactions, event)
				if len(reactions) == 20 {
					break
				}
			}
		}
		recent = reactions
		input := intelligence.PlanningInput{Scope: intelligence.Scope{RequestID: djID(), SessionID: session.ID, SessionRevision: session.Revision, TasteRevision: taste.Revision, ConfigRevision: cfg.Revision}, Taste: taste, Session: session, Channels: a.reco.DJChannels(), Recent: recent}
		// Similar-track channels require a real seed. A standalone collection has none.
		if req.Track.ID == "" && session.Seed == nil {
			channels := input.Channels[:0]
			for _, c := range input.Channels {
				if c.Channel.Capability != "similar_tracks" {
					channels = append(channels, c)
				}
			}
			input.Channels = channels
		}
		if session.Mode == intelligence.RadioTrack && session.Seed != nil {
			session.Intent = "Explore music related to " + session.Seed.Title + " by " + strings.Join(session.Seed.Artists, ", ") + "; keep the seed above global taste"
		} else {
			session.Intent = "Discover unfamiliar tracks consistent with weighted taste and recent session reactions"
		}
		input.Session = session
		input.Outcomes = []intelligence.SearchOutcome{}
		a.saveDJTasteSnapshot(ctx, jobID, taste)
		input.Directions = []intelligence.Direction{}
		input.RecentSuggestions = []intelligence.TrackSummary{}
		for _, row := range a.recentDJSuggestions(ctx) {
			input.RecentSuggestions = append(input.RecentSuggestions, row.Track)
		}

		if repo, ok := a.store.(intelligence.PlanRepository); ok {
			input.Directions, _ = repo.DJDirections(ctx, session.ID)
		}
		base := cfg.BaseURL
		if base == "" && cfg.Provider == "deepseek" {
			base = "https://api.deepseek.com"
		}
		var director *djproviders.Director
		if cfg.Enabled && cfg.APIKey != "" {
			director, _ = djproviders.New(cfg.Provider, cfg.Model, cfg.APIKey, base)
		}
		defer func() {
			a.dj.Lock()
			defer a.dj.Unlock()
			if director == nil {
				return
			}
			diagnostic.Tokens = director.TokenUsage()
			a.dj.tokens += director.TokenUsage()
			raw, _ := json.Marshal(djUsage{a.dj.day, a.dj.used, a.dj.tokens})
			_ = a.store.SettingSet(context.WithoutCancel(ctx), "intelligence.private.usage", string(raw))
		}()
		output := a.coordinateDJ(ctx, cfg, input, req, issued, director, &diagnostic)
		ordered, candidates, plan, ranked, selection := output.tracks, output.candidates, output.plan, output.ranked, output.selection
		ordered, candidates = a.resolveDJVersions(ctx, ordered, candidates, &diagnostic)
		for i := range ordered {
			ordered[i].DJSessionID = session.ID
			ordered[i].DJPlanID = selection.PoolID
		}
		if len(ordered) == 0 {
			if diagnostic.Error == "" {
				diagnostic.Error = "Нет кандидатов с подтверждёнными персональными связями; предыдущая коллекция сохранена"
			}
			return
		}

		if err = repo.SaveDJCandidates(ctx, session.ID, selection.PoolID, candidates); err != nil {
			return
		}
		if plans, ok := a.store.(intelligence.PlanRepository); ok {
			if err = plans.SaveDJPlan(ctx, session.ID, selection.PoolID, plan, ranked); err != nil {
				return
			}
		}
		// Feedback can advance while the model works. Keep the valid snapshot,
		// then apply current blocks and played/excluded IDs when consuming it.
		ordered = a.reco.FinalizeDJ(ctx, ordered, nil, 40)
		if len(ordered) == 0 {
			return
		}
		diagnostic.Error = ""
		diagnostic.Stage = "saving"
		diagnostic.RequestDeadlineAt = ""
		a.saveDJDiagnostic(ctx, diagnostic)
		diagnostic.ResultID = selection.PoolID
		latest, _ := json.Marshal(djCollection{JobID: jobID, ResultID: selection.PoolID, SessionID: session.ID, Tracks: ordered, UpdatedAt: time.Now().UTC().Format(time.RFC3339)})
		key := "intelligence.private.radioResult." + session.ID
		if req.Collection {
			key = "intelligence.collection"
		}
		if ctx.Err() != nil || a.djConfig(ctx).Revision != cfg.Revision {
			return
		}
		if err = a.store.SettingSet(ctx, key, string(latest)); err != nil {
			return
		}
		a.dj.Lock()
		defer a.dj.Unlock()
		if ctx.Err() != nil {
			return
		}
		if a.dj.ready == nil {
			a.dj.ready = map[string]djReady{}
		}
		if len(a.dj.ready) >= 8 {
			a.dj.ready = map[string]djReady{}
		}
		for _, track := range ordered {
			a.recordDJLifecycle(ctx, session.ID, selection.PoolID, djTrackID(track), intelligence.FeedbackRecommended)
		}
		suggestions := []intelligence.TrackSummary{}
		for _, track := range ordered {
			suggestions = append(suggestions, djSummary(track))
		}
		a.saveDJSuggestions(ctx, suggestions)
		a.dj.ready[session.ID] = djReady{ordered, selection.Context.Scope, time.Now()}
		a.dj.prepared++
		diagnostic.Selected = len(ordered)
		success = true
	}()
}

func (a *App) handleIntelligence(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/v1/intelligence/") {
	case "sources":
		a.handleDJSources(w, r)
	case "tuning":
		a.handleDJTuning(w, r)
	case "profile":
		if r.Method != "GET" {
			writeAPIError(w, 405, "требуется GET")
			return
		}
		if a.reco == nil {
			writeAPIError(w, 503, "движок недоступен")
			return
		}
		writeAPIJSON(w, 200, a.reco.IntelligenceTaste(r.Context()))
	case "behavior":
		a.handleDJBehavior(w, r)
	case "collection":
		a.handleDJCollection(w, r)
	case "config":
		if r.Method == http.MethodGet {
			writeAPIJSON(w, 200, a.IntelligenceStatus())
			return
		}
		if r.Method != http.MethodPost {
			writeAPIError(w, 405, "требуется GET или POST")
			return
		}
		var c DJConfig
		if decodeAPIJSON(w, r, &c) != nil {
			return
		}
		if err := a.ConfigureIntelligence(c); err != nil {
			writeAPIError(w, 400, err.Error())
			return
		}
		writeAPIJSON(w, 200, a.IntelligenceStatus())
	case "radio":
		if r.Method != http.MethodPost {
			writeAPIError(w, 405, "требуется POST")
			return
		}
		var req djRadioRequest
		if decodeAPIJSON(w, r, &req) != nil {
			return
		}
		if req.Mode != intelligence.RadioInfinite && req.Mode != intelligence.RadioTrack {
			writeAPIError(w, 400, "неизвестный режим")
			return
		}
		if req.Mode == intelligence.RadioTrack {
			if err := validateAPITrack(req.Track); err != nil {
				writeAPIError(w, 400, "нужен seed")
				return
			}
		}
		if len(req.Exclude) > 200 {
			req.Exclude = req.Exclude[len(req.Exclude)-200:]
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		out, err := a.djRadio(ctx, req)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, 200, out)
	case "feedback":
		if r.Method != http.MethodPost {
			writeAPIError(w, 405, "требуется POST")
			return
		}
		var e intelligence.FeedbackEvent
		if decodeAPIJSON(w, r, &e) != nil {
			return
		}
		repo, ok := a.store.(intelligence.SessionRepository)
		if !ok {
			writeAPIError(w, 503, "хранилище недоступно")
			return
		}
		added, err := repo.RecordDJFeedback(r.Context(), e)
		if err != nil {
			writeAPIError(w, 400, "событие не принято")
			return
		}
		if added {
			a.saveDJReaction(r.Context(), e, repo)
		} // No paid call per reaction.

		writeAPIJSON(w, 200, map[string]bool{"ok": true})
	case "end":
		if r.Method != http.MethodPost {
			writeAPIError(w, 405, "требуется POST")
			return
		}
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if decodeAPIJSON(w, r, &req) != nil {
			return
		}
		if repo, ok := a.store.(intelligence.SessionRepository); ok {
			_ = repo.EndDJSession(r.Context(), req.SessionID)
		}
		a.dj.Lock()
		if a.dj.sessionID == req.SessionID && a.dj.cancel != nil {
			a.dj.cancel()
		}
		delete(a.dj.ready, req.SessionID)
		delete(a.dj.pending, req.SessionID)
		a.dj.Unlock()
		writeAPIJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeAPIError(w, 404, "маршрут не найден")
	}
}
