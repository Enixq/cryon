package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/identity"
	"Cryon2/internal/intelligence"
	djproviders "Cryon2/internal/intelligence/providers"
	"Cryon2/internal/recommendations"
)

type djOutput struct {
	tracks     []domain.Track
	candidates []intelligence.Candidate
	plan       intelligence.SearchPlan
	ranked     intelligence.SelectionPlan
	selection  intelligence.SelectionInput
}
type cachedDJStrategy struct {
	Key    string                  `json:"key"`
	At     time.Time               `json:"at"`
	Plan   intelligence.SearchPlan `json:"plan"`
	Failed bool                    `json:"failed"`
}

// A strategy is reusable, a playlist is not: the pool, feedback and ranking are
// recomputed for every job. Cache keys contain no credential or user path.
func strategyKey(cfg DJConfig, in intelligence.PlanningInput) string {
	// Ordinary playback timestamps are not a new research intent. Coarse taste
	// buckets retain plans across adjacent queue refills while explicit feedback,
	// meaningful preference changes, mode and seed invalidate them.
	taste := in.Taste
	taste.Revision = 0
	taste.Positive, taste.Artists = nil, nil
	taste.Preferences = append([]intelligence.ArtistPreference{}, in.Taste.Preferences...)
	for i := range taste.Preferences {
		p := &taste.Preferences[i]
		p.LastPlayedMS = 0
		p.Weight = math.Round(p.Weight*5) / 5
		p.Confidence = math.Round(p.Confidence*5) / 5
	}
	sort.Slice(taste.Preferences, func(i, j int) bool { return taste.Preferences[i].Name < taste.Preferences[j].Name })
	taste.Tags = append([]intelligence.WeightedTag{}, in.Taste.Tags...)
	total := 0.0
	for _, tag := range taste.Tags {
		total += tag.Weight
	}
	for i := range taste.Tags {
		if total > 0 {
			taste.Tags[i].Weight = math.Round(taste.Tags[i].Weight/total*5) / 5
		}
	}
	sort.Slice(taste.Tags, func(i, j int) bool { return taste.Tags[i].Name < taste.Tags[j].Name })
	explicit := []intelligence.FeedbackEvent{}
	for _, event := range in.Recent {
		if event.Kind == intelligence.FeedbackLike || event.Kind == intelligence.FeedbackDislike {
			explicit = append(explicit, event)
		}
	}
	raw, _ := json.Marshal(struct {
		Revision uint64
		Taste    intelligence.TasteProfile
		Mode     intelligence.RadioMode
		Seed     *intelligence.TrackSummary
		Channels []intelligence.ChannelBudget
		Recent   []intelligence.FeedbackEvent
	}{cfg.Revision, taste, in.Session.Mode, in.Session.Seed, in.Channels, explicit})
	hash := sha256.Sum256(raw)
	return "intelligence.private.strategy.v2." + hex.EncodeToString(hash[:])
}

func (a *App) djStrategy(ctx context.Context, cfg DJConfig, in intelligence.PlanningInput, director *djproviders.Director, d *DJDiagnostic) intelligence.SearchPlan {
	d.StrategySource = "local"
	fallback := intelligence.CanonicalSearch(intelligence.PersonalSearch(in))
	if director == nil {
		return fallback
	}
	key := strategyKey(cfg, in)
	// Sixteen bounded slots, with full-key validation on collisions.
	slot := "intelligence.private.strategy.slot." + key[len(key)-1:]
	raw, _, _ := a.store.SettingGet(ctx, slot)
	var cached cachedDJStrategy
	limits := intelligence.Limits{MaxSteps: 5, MaxCandidates: 40, MaxDirections: 3, MaxQueryRunes: 120, MaxReasonRunes: 160}
	if json.Unmarshal([]byte(raw), &cached) == nil && cached.Key == key {
		ttl := 6 * time.Hour
		if cached.Failed {
			ttl = 5 * time.Minute
		}
		if time.Since(cached.At) >= 0 && time.Since(cached.At) < ttl {
			cached.Plan.Scope = in.Scope
			if intelligence.ValidateSearch(in, cached.Plan, limits) == nil {
				d.PlanFromCache = true
				d.StrategySource = "cache"
				if cached.Failed {
					d.StrategySource = "local"
					d.Warning = "Модель недавно недоступна; стратегия Cryon"
				}
				return cached.Plan
			}
		}
	}
	a.dj.Lock()
	allowed := a.dj.used < cfg.DailySegments
	if allowed {
		a.dj.used++
		usage, _ := json.Marshal(djUsage{a.dj.day, a.dj.used, a.dj.tokens})
		_ = a.store.SettingSet(ctx, "intelligence.private.usage", string(usage))
	}
	a.dj.Unlock()
	if !allowed {
		d.Warning = "Лимит AI исчерпан; используется локальная стратегия Cryon"
		return fallback
	}
	d.RequestDeadlineAt = time.Now().Add(20 * time.Second).UTC().Format(time.RFC3339)
	a.saveDJDiagnostic(ctx, *d)
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	compact := in
	compact.RecentSuggestions = nil
	if len(compact.Taste.Preferences) > 20 {
		compact.Taste.Preferences = compact.Taste.Preferences[:20]
	}
	if len(compact.Taste.Positive) > 12 {
		compact.Taste.Positive = compact.Taste.Positive[:12]
	}
	if len(compact.Recent) > 12 {
		compact.Recent = compact.Recent[:12]
	}
	plan, err := director.PlanSearch(callCtx, compact)
	d.LLMCalls++
	plan = intelligence.CanonicalSearch(plan)
	d.StrategySource = "model"
	if err == nil {
		err = intelligence.ValidateSearch(in, plan, limits)
	}
	if err != nil {
		plan = fallback
		d.StrategySource = "local"
		d.Warning = "AI-стратегия не применена: " + err.Error() + "; поиск продолжен локально"
	}
	d.RequestDeadlineAt = ""
	if ctx.Err() == nil {
		cached = cachedDJStrategy{Key: key, At: time.Now(), Plan: plan, Failed: err != nil}
		raw, _ := json.Marshal(cached)
		_ = a.store.SettingSet(ctx, slot, string(raw))
	}
	return plan
}

type djToolResult struct {
	index   int
	tracks  []domain.Track
	err     error
	elapsed int64
	origins map[string][]intelligence.CandidateOrigin
}

// The coordinator owns the clock. Even a broken provider ignoring context
// cannot block it; detached tool returns only to a buffered result channel.
func (a *App) djTools(ctx context.Context, plan intelligence.SearchPlan, seed domain.Track, target int) []djToolResult {
	start := time.Now()
	batch, stop := context.WithTimeout(ctx, 8*time.Second)
	defer stop()
	ch := make(chan djToolResult, len(plan.Steps))
	a.dj.Lock()
	if a.dj.toolSlots == nil {
		a.dj.toolSlots = make(chan struct{}, 8)
	}
	slots := a.dj.toolSlots
	a.dj.Unlock()
	for i, step := range plan.Steps {
		go func(i int, step intelligence.SearchStep) {
			defer a.recoverGoroutine("DJ tool")
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				ch <- djToolResult{index: i, err: fmt.Errorf("provider capacity exhausted")}
				return
			}
			start := time.Now()
			toolCtx, cancel := context.WithTimeout(batch, 7*time.Second)
			defer cancel()
			tracks, origins, err := a.reco.ExecuteDJTool(toolCtx, step, seed)
			ch <- djToolResult{index: i, tracks: tracks, err: err, elapsed: time.Since(start).Milliseconds(), origins: origins}
		}(i, step)
	}
	rows := make([]djToolResult, len(plan.Steps))
	complete := make([]bool, len(rows))
	var grace <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	links := map[string]bool{}

	for i := range rows {
		rows[i] = djToolResult{index: i, err: context.DeadlineExceeded, elapsed: 8000}
	}
	for remaining := len(rows); remaining > 0; remaining-- {
		select {
		case row := <-ch:
			rows[row.index] = row
			complete[row.index] = true
			capability := plan.Steps[row.index].Channel.Capability
			if capability == "similar_tracks" || capability == "taste_neighbors" || capability == "history_neighbors" || capability == "discovery" {
				for _, track := range row.tracks {
					links[djTrackID(track)] = true
				}
			}
			if target > 0 && len(links) >= target && grace == nil {
				timer = time.NewTimer(time.Second)
				grace = timer.C
			}
		case <-grace:
			for i := range rows {
				if !complete[i] {
					rows[i].err = context.Canceled
					rows[i].elapsed = time.Since(start).Milliseconds()
				}
			}
			return rows
		case <-batch.Done():
			// Drain completed partial results without waiting for unfinished tools.
			for {
				select {
				case row := <-ch:
					rows[row.index] = row

				default:
					return rows
				}
			}
		}
	}
	return rows
}

func (a *App) coordinateDJ(ctx context.Context, cfg DJConfig, in intelligence.PlanningInput, req djRadioRequest, issued []domain.Track, director *djproviders.Director, d *DJDiagnostic) djOutput {
	if a.registry != nil {
		ctx = recommendations.WithDJVersionResolution(ctx)
	}
	ctx = a.reco.AcquisitionContext(ctx, in.Scope.RequestID)
	plan := a.djStrategy(ctx, cfg, in, director, d)
	output := djOutput{}
	visited := map[intelligence.Channel]bool{}
	pool := []domain.Track{}
	played := map[string]bool{}
	for _, id := range in.Session.PlayedIDs {
		played[id] = true
	}
	provenance := map[string][]intelligence.Channel{}
	origins := map[string][]intelligence.CandidateOrigin{}
	// Two bounded acquisition rounds, at most one strategic model call.
	for round := 0; round < 2 && ctx.Err() == nil; round++ {
		if round > 0 {
			plan = intelligence.CanonicalSearch(intelligence.PersonalSearch(in))
			for _, c := range in.Channels {
				if c.Channel.Provider == "cryon" && c.Channel.Capability == "library" {
					plan.Steps = append(plan.Steps, intelligence.SearchStep{Channel: c.Channel, Query: "context", Limit: min(20, c.MaxCandidates)})
				}
			}
		}
		steps := []intelligence.SearchStep{}
		for _, s := range plan.Steps {
			if !visited[s.Channel] {
				visited[s.Channel] = true
				steps = append(steps, s)
			}
		}
		if len(steps) == 0 {
			break
		}
		plan.Steps = steps
		d.Iteration = round + 1
		d.Stage = "searching"
		a.saveDJDiagnostic(ctx, *d)
		results := a.djTools(ctx, plan, req.Track, min(12, req.Limit))
		for i, row := range results {
			step := steps[i]
			state := "ok"
			if len(row.tracks) == 0 {
				state = "empty"
			}
			if row.err != nil {
				state = "error"
			}
			if row.err == context.DeadlineExceeded {
				state = "timeout"
			}
			if row.err == context.Canceled {
				state = "cancelled"
			}
			d.Searches = append(d.Searches, DJSearchStatus{Iteration: round + 1, Source: step.Channel.Provider + "/" + step.Channel.Capability, Query: step.Query, Found: len(row.tracks), Unique: len(a.reco.PrepareDJPool(ctx, row.tracks, nil, 40)), ElapsedMS: row.elapsed, State: state})
			d.Found += len(row.tracks)
			for _, track := range row.tracks {
				id := djTrackID(track)
				if played[id] || played[track.ID] {
					continue
				}
				provenance[id] = append(provenance[id], step.Channel)
				anchor := "profile"
				if in.Session.Seed != nil {
					anchor = in.Session.Seed.ID
				}
				if len(row.origins[id]) > 0 {
					origins[id] = append(origins[id], row.origins[id]...)
				} else {
					origins[id] = append(origins[id], intelligence.CandidateOrigin{Channel: step.Channel, AnchorID: anchor, Query: step.Query})
				}
				pool = append(pool, track)
			}
		}
		exclude := append(append([]domain.Track{}, req.Exclude...), issued...)
		if req.Mode == intelligence.RadioTrack {
			exclude = append(exclude, req.Track)
		}
		// Pool dedup preserves provenance across playable copies of a recording.
		pool, provenance, origins = a.mergeDJPool(ctx, pool, exclude, provenance, origins)
		d.Candidates = len(pool)
		d.Stage = "ranking"
		a.saveDJDiagnostic(ctx, *d)
		cs := []intelligence.Candidate{}
		evidence := a.reco.DJEvidence(ctx, pool, req.Track, in.Taste, in.RecentSuggestions)
		unfamiliar := a.reco.DJUnfamiliarArtists(ctx, pool)
		for i, t := range pool {
			canonical := ""
			if groups := identity.GroupTracks([]domain.Track{t}); len(groups) > 0 {
				canonical = groups[0].Track.ID
			}
			cs = append(cs, intelligence.Candidate{CanonicalID: canonical, ID: fmt.Sprintf("c%d", i), Track: djSummary(t), Evidence: evidence[i], UnfamiliarArtist: unfamiliar[i], BaseRank: i + 1, Channels: provenance[djTrackID(t)], Origins: origins[djTrackID(t)]})
		}
		selection := intelligence.SelectionInput{Context: in, PoolID: djID(), Strategy: plan.Strategy, Candidates: cs}
		ranked, tracks := a.reco.RankDJCandidates(ctx, selection, pool, min(20, req.Limit))
		d.LocalAdmitted = len(ranked.Ranked)
		d.Engine = "local"
		d.AISelected = 0
		if d.StrategySource == "model" || d.StrategySource == "cache" {
			d.Engine = "local_ai_strategy"
		}
		output = djOutput{tracks, cs, plan, ranked, selection}
		for j := range d.Searches {
			count := 0
			for _, t := range tracks {
				for _, ch := range provenance[djTrackID(t)] {
					if d.Searches[j].Source == ch.Provider+"/"+ch.Capability {
						count++
						break
					}
				}
			}
			d.Searches[j].Accepted = count
		}
		d.Outcomes = append(d.Outcomes, intelligence.SearchOutcome{Iteration: round + 1, Found: len(pool), Accepted: len(tracks), Reason: "local ranking"})
		if len(tracks) >= min(6, req.Limit) {
			break
		}
	}
	if len(pool) == 0 {
		d.Error = "Музыкальные источники не вернули кандидатов; предыдущий результат сохранён"
	}
	return output
}
