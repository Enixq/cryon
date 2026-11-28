// Package intelligence defines the optional AI DJ's planning boundary.
// Models propose searches and selections; Cryon executes and finalizes them.
package intelligence

import (
	"Cryon2/internal/domain"
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
	Preferences    []ArtistPreference `json:"preferences"`
	Positive       []TrackSummary     `json:"positive_examples"`
	Negative       []TrackSummary     `json:"negative_examples"`
	BlockedArtists []string           `json:"blocked_artists"`
	Revision       uint64             `json:"revision"`
	Artists        []string           `json:"artists"`
	Tags           []WeightedTag      `json:"tags"`
}
type ArtistPreference struct {
	Name         string  `json:"name"`
	Weight       float64 `json:"weight"`
	Confidence   float64 `json:"confidence"`
	LastPlayedMS int64   `json:"last_played_ms"`
}
type SearchOutcome struct {
	Iteration int      `json:"iteration"`
	Queries   []string `json:"queries"`
	Found     int      `json:"found"`
	Accepted  int      `json:"accepted"`
	Reason    string   `json:"reason"`
}

type SessionProfile struct {
	ID           string        `json:"id"`
	Revision     uint64        `json:"revision"`
	Mode         RadioMode     `json:"mode"`
	Seed         *TrackSummary `json:"seed"`
	Intent       string        `json:"intent"`
	PlayedIDs    []string      `json:"played_ids"`
	SuggestedIDs []string      `json:"suggested_ids"`
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
	Outcomes          []SearchOutcome `json:"previous_search_outcomes"`
	RecentSuggestions []TrackSummary  `json:"recent_suggestions"`
	Scope             Scope           `json:"scope"`
	Taste             TasteProfile    `json:"taste"`
	Session           SessionProfile  `json:"session"`
	Channels          []ChannelBudget `json:"channels"`
	Recent            []FeedbackEvent `json:"recent"`
	Directions        []Direction     `json:"previous_directions"`
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
	PlaybackCandidates []domain.PlaybackCandidate `json:"playback_candidates,omitempty"`
	CanonicalID        string                     `json:"canonical_id,omitempty"`
	Origins            []CandidateOrigin          `json:"origins"`
	Evidence           Evidence                   `json:"evidence"`
	UnfamiliarArtist   bool                       `json:"unfamiliar_artist"`
	ID                 string                     `json:"candidate_id"`
	Track              TrackSummary               `json:"track"`
	BaseRank           int                        `json:"base_rank"`
	Channels           []Channel                  `json:"channels"`
}
type CandidateOrigin struct {
	Channel       Channel  `json:"channel"`
	AnchorID      string   `json:"anchor_id"`
	Query         string   `json:"query"`
	AnchorArtists []string `json:"anchor_artists,omitempty"`
}
type Evidence struct {
	ArtistAffinity     float64 `json:"artist_affinity"`
	SessionAffinity    float64 `json:"session_affinity"`
	GraphAffinity      float64 `json:"graph_affinity"`
	Feedback           float64 `json:"feedback"`
	IdentityConfidence float64 `json:"identity_confidence"`
	TasteSimilarity    float64 `json:"taste_similarity"`
	SeedSimilarity     float64 `json:"seed_similarity"`
	SeedKnown          bool    `json:"seed_evidence_known"`
	MetadataQuality    float64 `json:"metadata_quality"`
	RecentlyPlayed     bool    `json:"recently_played"`
	RecentlySuggested  bool    `json:"recently_suggested"`
	Version            string  `json:"version"`
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
	FeedbackCandidate     FeedbackKind = "track_candidate_found"
	FeedbackRecommended   FeedbackKind = "track_recommended"
	FeedbackQueued        FeedbackKind = "track_queued"
	FeedbackStarted       FeedbackKind = "playback_started"
	FeedbackShown         FeedbackKind = "shown"
	FeedbackLike          FeedbackKind = "like"
	FeedbackDislike       FeedbackKind = "dislike"
	FeedbackSkip          FeedbackKind = "skip"
	FeedbackCompleted     FeedbackKind = "completed"
	FeedbackReplay        FeedbackKind = "replay"
	FeedbackPlaybackError FeedbackKind = "playback_error"
)

type FeedbackEvent struct {
	Name       string       `json:"name"`
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
