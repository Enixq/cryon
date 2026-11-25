// Package intelligence defines the optional AI DJ's planning boundary.
// Models propose searches and selections; Cryon executes and finalizes them.
package intelligence

import (
	"context"
	"time"
)

type RadioMode string

const (
	RadioInfinite RadioMode = "infinite"
	RadioTrack    RadioMode = "track"
)

// TrackSummary intentionally excludes URLs, credentials and local file paths.
type TrackSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Artists    []string `json:"artists"`
	Tags       []string `json:"tags"`
	DurationMS int      `json:"duration_ms"`
}

type WeightedTag struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

type TasteProfile struct {
	Revision uint64        `json:"revision"`
	Artists  []string      `json:"artists"`
	Tags     []WeightedTag `json:"tags"`
}

type SessionProfile struct {
	ID        string        `json:"id"`
	Revision  uint64        `json:"revision"`
	Mode      RadioMode     `json:"mode"`
	Seed      *TrackSummary `json:"seed"`
	Intent    string        `json:"intent"`
	PlayedIDs []string      `json:"played_ids"`
}

// Channel identifies metadata capabilities, not a playback service.
type Channel struct {
	Provider   string `json:"provider"`
	Capability string `json:"capability"`
}

type ChannelBudget struct {
	Channel       Channel `json:"channel"`
	MaxCandidates int     `json:"max_candidates"`
}

type Strategy struct {
	Exploration   float64 `json:"exploration"`
	SeedAdherence float64 `json:"seed_adherence"`
	Diversity     float64 `json:"diversity"`
}

// Scope binds each proposal to one exact context. RequestID changes per phase.
type Scope struct {
	RequestID       string `json:"request_id"`
	SessionID       string `json:"session_id"`
	SessionRevision uint64 `json:"session_revision"`
	TasteRevision   uint64 `json:"taste_revision"`
	ConfigRevision  uint64 `json:"config_revision"`
}

type PlanningInput struct {
	Scope      Scope           `json:"scope"`
	Taste      TasteProfile    `json:"taste"`
	Session    SessionProfile  `json:"session"`
	Channels   []ChannelBudget `json:"channels"`
	Recent     []FeedbackEvent `json:"recent"`
	Directions []Direction     `json:"previous_directions"`
}

type SearchStep struct {
	Channel Channel `json:"channel"`
	Query   string  `json:"query"`
	Limit   int     `json:"limit"`
}

type SearchPlan struct {
	Scope    Scope        `json:"scope"`
	Strategy Strategy     `json:"strategy"`
	Steps    []SearchStep `json:"steps"`
}

type Candidate struct {
	UnfamiliarArtist bool         `json:"unfamiliar_artist"`
	ID               string       `json:"candidate_id"`
	Track            TrackSummary `json:"track"`
	BaseRank         int          `json:"base_rank"`
	Channels         []Channel    `json:"channels"`
}

type SelectionInput struct {
	Strategy   Strategy      `json:"strategy"`
	Context    PlanningInput `json:"context"`
	PoolID     string        `json:"pool_id"`
	Candidates []Candidate   `json:"candidates"`
}

type CandidateDecision struct {
	ID     string  `json:"candidate_id"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type Direction struct {
	Label string `json:"label"`
	Query string `json:"query"`
}

type SelectionPlan struct {
	Scope      Scope               `json:"scope"`
	PoolID     string              `json:"pool_id"`
	Ranked     []CandidateDecision `json:"ranked"`
	Avoid      []string            `json:"avoid"`
	Directions []Direction         `json:"directions"`
}

type FeedbackKind string

const (
	FeedbackShown         FeedbackKind = "shown"
	FeedbackLike          FeedbackKind = "like"
	FeedbackDislike       FeedbackKind = "dislike"
	FeedbackSkip          FeedbackKind = "skip"
	FeedbackCompleted     FeedbackKind = "completed"
	FeedbackReplay        FeedbackKind = "replay"
	FeedbackPlaybackError FeedbackKind = "playback_error"
)

type FeedbackEvent struct {
	ID         string       `json:"id"`
	SessionID  string       `json:"session_id"`
	PlanID     string       `json:"plan_id"`
	TrackID    string       `json:"track_id"`
	Kind       FeedbackKind `json:"kind"`
	PlayedMS   int          `json:"played_ms"`
	DurationMS int          `json:"duration_ms"`
	At         time.Time    `json:"at"`
}

// Director is invoked only by background workers, never playback handlers.
// Planning and selection are separate calls: unknown candidates cannot be ranked.
type Director interface {
	PlanSearch(context.Context, PlanningInput) (SearchPlan, error)
	Select(context.Context, SelectionInput) (SelectionPlan, error)
}

// PlanRepository is optional so the baseline Store remains unchanged.
type PlanRepository interface {
	SaveDJPlan(context.Context, string, string, SearchPlan, SelectionPlan) error
	DJDirections(context.Context, string) ([]Direction, error)
	DJMetrics(context.Context) (map[string]int, error)
}
