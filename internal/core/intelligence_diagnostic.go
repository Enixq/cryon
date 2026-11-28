package core

import (
	"Cryon2/internal/intelligence"
	"context"
	"encoding/json"
	"time"
)

type DJSearchStatus struct {
	Iteration int    `json:"iteration"`
	Unique    int    `json:"unique"`
	Accepted  int    `json:"accepted"`
	ElapsedMS int64  `json:"elapsedMs"`
	Source    string `json:"source"`
	Query     string `json:"query"`
	Found     int    `json:"found"`
	State     string `json:"state"`
}
type DJDiagnostic struct {
	Sources           []DJSourceDiagnostic         `json:"sources,omitempty"`
	StrategySource    string                       `json:"strategySource"`
	LLMCalls          int                          `json:"llmCalls"`
	PlanFromCache     bool                         `json:"planFromCache"`
	Engine            string                       `json:"engine,omitempty"`
	Warning           string                       `json:"warning,omitempty"`
	RequestDeadlineAt string                       `json:"requestDeadlineAt,omitempty"`
	Tokens            int                          `json:"tokens"`
	Outcomes          []intelligence.SearchOutcome `json:"outcomes"`
	JobID             string                       `json:"jobId"`
	ResultID          string                       `json:"resultId"`
	SessionID         string                       `json:"sessionId"`
	Mode              string                       `json:"mode"`
	Iteration         int                          `json:"iteration"`
	ConfigRevision    uint64                       `json:"configRevision"`
	Stage             string                       `json:"stage"`
	Error             string                       `json:"error,omitempty"`
	Found             int                          `json:"found"`
	Candidates        int                          `json:"candidates"`
	LocalAdmitted     int                          `json:"localAdmitted"`
	AISelected        int                          `json:"aiSelected"`
	Selected          int                          `json:"selected"`
	Searches          []DJSearchStatus             `json:"searches"`
	UpdatedAt         string                       `json:"updatedAt"`
}

func (a *App) saveDJDiagnostic(ctx context.Context, d DJDiagnostic) {
	if a.store == nil {
		return
	}
	if d.JobID != "" {
		key := "intelligence.private.radioJob." + d.SessionID
		if d.Mode == "collection" {
			key = "intelligence.private.collectionJob"
		}
		previous, _, _ := a.store.SettingGet(context.WithoutCancel(ctx), key)
		var old DJDiagnostic
		_ = json.Unmarshal([]byte(previous), &old)
		if old.JobID != "" && old.JobID != d.JobID && d.Stage != "planning" {
			return
		}
	}
	d.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if d.Searches == nil {
		d.Searches = []DJSearchStatus{}
	}
	raw, _ := json.Marshal(d)
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = a.store.SettingSet(writeCtx, "intelligence.private.diagnostic", string(raw))
	if d.JobID != "" {
		key := "intelligence.private.radioJob." + d.SessionID
		if d.Mode == "collection" {
			key = "intelligence.private.collectionJob"
		}
		_ = a.store.SettingSet(writeCtx, key, string(raw))
	}
}
func (a *App) loadDJDiagnostic(ctx context.Context, revision uint64) DJDiagnostic {
	d := DJDiagnostic{}
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.diagnostic")
		_ = json.Unmarshal([]byte(raw), &d)
	}
	if d.ConfigRevision != revision {
		return DJDiagnostic{}
	}
	return d
}

func (a *App) recordDJLifecycle(ctx context.Context, session, plan, track string, kind intelligence.FeedbackKind) {
	if repo, ok := a.store.(intelligence.SessionRepository); ok {
		_, _ = repo.RecordDJFeedback(ctx, intelligence.FeedbackEvent{ID: plan + ":" + string(kind) + ":" + track, SessionID: session, PlanID: plan, TrackID: track, Kind: kind, At: time.Now().UTC()})
	}
}

type djReaction struct {
	Event intelligence.FeedbackEvent `json:"event"`
	Track intelligence.TrackSummary  `json:"track"`
}

func (a *App) saveDJReaction(ctx context.Context, event intelligence.FeedbackEvent, repo intelligence.SessionRepository) {
	if event.Kind == intelligence.FeedbackPlaybackError || event.Kind == intelligence.FeedbackCandidate || event.Kind == intelligence.FeedbackRecommended || event.Kind == intelligence.FeedbackShown || event.Kind == intelligence.FeedbackQueued || event.Kind == intelligence.FeedbackStarted {
		return
	}
	candidates, err := repo.DJCandidates(ctx, event.SessionID, event.PlanID)
	if err != nil {
		return
	}
	for _, candidate := range candidates {
		matches := candidate.Track.ID == event.TrackID
		for _, version := range candidate.PlaybackCandidates {
			if version.ID == event.TrackID {
				matches = true
			}
		}
		if !matches {
			continue
		}
		a.appendDJReaction(ctx, djReaction{event, candidate.Track})
		return
	}
}

func (a *App) saveDJTasteSnapshot(ctx context.Context, job string, taste intelligence.TasteProfile) {
	type snapshot struct {
		JobID string                    `json:"jobId"`
		At    string                    `json:"at"`
		Taste intelligence.TasteProfile `json:"taste"`
	}
	raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.tasteHistory")
	history := []snapshot{}
	_ = json.Unmarshal([]byte(raw), &history)
	history = append(history, snapshot{job, time.Now().UTC().Format(time.RFC3339), taste})
	if len(history) > 12 {
		history = history[len(history)-12:]
	}
	data, _ := json.Marshal(history)
	_ = a.store.SettingSet(ctx, "intelligence.private.tasteHistory", string(data))
}
