package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"Cryon2/internal/intelligence"
)

var _ intelligence.SessionRepository = (*Store)(nil)

var intelligenceSchema = []string{
	`CREATE TABLE IF NOT EXISTS dj_sessions(id TEXT PRIMARY KEY, revision INTEGER NOT NULL, profile_json TEXT NOT NULL, ended_at INTEGER, created_at INTEGER NOT NULL);`,
	`CREATE TABLE IF NOT EXISTS dj_feedback(id TEXT PRIMARY KEY, session_id TEXT NOT NULL, event_json TEXT NOT NULL, at INTEGER NOT NULL);`,
	`CREATE INDEX IF NOT EXISTS idx_dj_feedback_session ON dj_feedback(session_id, at DESC);`,
	`CREATE TABLE IF NOT EXISTS dj_candidates(session_id TEXT NOT NULL, plan_id TEXT NOT NULL, candidate_id TEXT NOT NULL, candidate_json TEXT NOT NULL, PRIMARY KEY(session_id, plan_id, candidate_id));`,
	`CREATE TABLE IF NOT EXISTS dj_plans(plan_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, search_json TEXT NOT NULL, selection_json TEXT NOT NULL, created_at INTEGER NOT NULL);`,
}

// CreateDJSession stores the seed once. Subsequent feedback never changes it.
func (s *Store) CreateDJSession(ctx context.Context, profile intelligence.SessionProfile) error {
	if profile.ID == "" || profile.Revision != 0 || (profile.Mode != intelligence.RadioTrack && profile.Mode != intelligence.RadioInfinite) {
		return errors.New("invalid DJ session")
	}
	if profile.Mode == intelligence.RadioTrack && (profile.Seed == nil || profile.Seed.ID == "") {
		return errors.New("track radio requires seed")
	}
	if len(profile.PlayedIDs) != 0 {
		return errors.New("new session cannot contain plays")
	}
	if profile.Intent == "" {
		profile.Intent = "Explore weighted long-term taste with session context"
		if profile.Mode == intelligence.RadioTrack {
			profile.Intent = "Preserve the original seed and explore related music"
		}
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO dj_sessions(id,revision,profile_json,created_at) VALUES(?,0,?,?)`, profile.ID, string(raw), time.Now().UnixMilli())
	return err
}

func (s *Store) DJSession(ctx context.Context, id string) (intelligence.SessionProfile, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT profile_json FROM dj_sessions WHERE id=? AND ended_at IS NULL`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return intelligence.SessionProfile{}, false, nil
	}
	if err != nil {
		return intelligence.SessionProfile{}, false, err
	}
	var profile intelligence.SessionProfile
	err = json.Unmarshal([]byte(raw), &profile)
	return profile, err == nil, err
}

func (s *Store) RecordDJFeedback(ctx context.Context, event intelligence.FeedbackEvent) (bool, error) {
	if event.ID == "" || event.SessionID == "" || event.TrackID == "" || event.At.IsZero() || !intelligence.ValidFeedback(event.Kind) || event.PlayedMS < 0 || event.DurationMS < 0 {
		return false, errors.New("invalid DJ feedback")
	}
	event.Name = intelligence.EventName(event.Kind)
	raw, err := json.Marshal(event)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT event_json FROM dj_feedback WHERE id=?`, event.ID).Scan(&previous)
	if err == nil {
		if previous != string(raw) {
			return false, errors.New("event ID reused with different payload")
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var profileJSON string
	err = tx.QueryRowContext(ctx, `SELECT profile_json FROM dj_sessions WHERE id=? AND ended_at IS NULL`, event.SessionID).Scan(&profileJSON)
	if err != nil {
		return false, fmt.Errorf("active DJ session: %w", err)
	}
	var profile intelligence.SessionProfile
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return false, err
	}
	if event.Kind == intelligence.FeedbackLike || event.Kind == intelligence.FeedbackDislike || event.Kind == intelligence.FeedbackSkip || event.Kind == intelligence.FeedbackCompleted || event.Kind == intelligence.FeedbackReplay {
		profile.Revision++
	}
	if event.Kind == intelligence.FeedbackRecommended || event.Kind == intelligence.FeedbackQueued || event.Kind == intelligence.FeedbackShown {
		found := false
		for _, id := range profile.SuggestedIDs {
			if id == event.TrackID {
				found = true
				break
			}
		}
		if !found {
			profile.SuggestedIDs = append(profile.SuggestedIDs, event.TrackID)
			if len(profile.SuggestedIDs) > 200 {
				profile.SuggestedIDs = profile.SuggestedIDs[len(profile.SuggestedIDs)-200:]
			}
		}
	}
	// Showing a recommendation or failing to load audio is not a play.
	if event.Kind == intelligence.FeedbackCompleted || event.Kind == intelligence.FeedbackReplay || event.Kind == intelligence.FeedbackSkip && event.PlayedMS > 0 {
		found := false
		for _, id := range profile.PlayedIDs {
			if id == event.TrackID {
				found = true
				break
			}
		}
		if !found {
			profile.PlayedIDs = append(profile.PlayedIDs, event.TrackID)
			if len(profile.PlayedIDs) > 200 {
				profile.PlayedIDs = profile.PlayedIDs[len(profile.PlayedIDs)-200:]
			}
		}
	}
	updated, err := json.Marshal(profile)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO dj_feedback(id,session_id,event_json,at) VALUES(?,?,?,?)`, event.ID, event.SessionID, string(raw), event.At.UnixMilli()); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE dj_sessions SET revision=?,profile_json=? WHERE id=?`, profile.Revision, string(updated), profile.ID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) SaveDJPlan(ctx context.Context, sessionID, planID string, search intelligence.SearchPlan, selection intelligence.SelectionPlan) error {
	searchJSON, err := json.Marshal(search)
	if err != nil {
		return err
	}
	selectionJSON, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO dj_plans(plan_id,session_id,search_json,selection_json,created_at) VALUES(?,?,?,?,?)`, planID, sessionID, string(searchJSON), string(selectionJSON), time.Now().UnixMilli())
	return err
}

func (s *Store) DJDirections(ctx context.Context, sessionID string) ([]intelligence.Direction, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT selection_json FROM dj_plans WHERE session_id=? ORDER BY created_at DESC LIMIT 1`, sessionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return []intelligence.Direction{}, nil
	}
	if err != nil {
		return nil, err
	}
	var plan intelligence.SelectionPlan
	err = json.Unmarshal([]byte(raw), &plan)
	return plan.Directions, err
}

// Counts are observations, not a claim that an unfamiliar liked track was novel.
func (s *Store) DJMetrics(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT json_extract(event_json,'$.kind'), COUNT(*) FROM dj_feedback GROUP BY json_extract(event_json,'$.kind')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		counts[kind] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var discoveries int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT json_extract(f.event_json,'$.track_id')) FROM dj_feedback f JOIN dj_candidates c ON c.session_id=f.session_id AND c.plan_id=json_extract(f.event_json,'$.plan_id') AND json_extract(c.candidate_json,'$.track.id')=json_extract(f.event_json,'$.track_id') WHERE json_extract(f.event_json,'$.kind')='like' AND json_extract(c.candidate_json,'$.unfamiliar_artist')=1`).Scan(&discoveries)
	if err != nil {
		return nil, err
	}
	counts["unfamiliar_artist_likes"] = discoveries
	return counts, nil
}

func (s *Store) DJFeedback(ctx context.Context, sessionID string, limit int) ([]intelligence.FeedbackEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_json FROM dj_feedback WHERE session_id=? ORDER BY at DESC,id DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []intelligence.FeedbackEvent{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var event intelligence.FeedbackEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *Store) EndDJSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE dj_sessions SET ended_at=? WHERE id=? AND ended_at IS NULL`, time.Now().UnixMilli(), id)
	return err
}

// SaveDJCandidates persists the original pool and source evidence for a segment.
// A plan ID is immutable; corrections use a new plan ID.
func (s *Store) SaveDJCandidates(ctx context.Context, sessionID, planID string, candidates []intelligence.Candidate) error {
	if sessionID == "" || planID == "" || len(candidates) == 0 || len(candidates) > 200 {
		return errors.New("invalid DJ candidate pool")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM dj_sessions WHERE id=? AND ended_at IS NULL`, sessionID).Scan(&active); err != nil {
		return err
	}
	if active != 1 {
		return errors.New("inactive DJ session")
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM dj_candidates WHERE session_id=? AND plan_id=?`, sessionID, planID).Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return errors.New("DJ plan pool already exists")
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.ID == "" || candidate.Track.ID == "" || seen[candidate.ID] {
			return errors.New("invalid DJ candidate identity")
		}
		seen[candidate.ID] = true
		raw, err := json.Marshal(candidate)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO dj_candidates(session_id,plan_id,candidate_id,candidate_json) VALUES(?,?,?,?)`, sessionID, planID, candidate.ID, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DJCandidates(ctx context.Context, sessionID, planID string) ([]intelligence.Candidate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT candidate_json FROM dj_candidates WHERE session_id=? AND plan_id=? ORDER BY candidate_id`, sessionID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []intelligence.Candidate{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var candidate intelligence.Candidate
		if err := json.Unmarshal([]byte(raw), &candidate); err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, rows.Err()
}
