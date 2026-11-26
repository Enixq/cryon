package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"Cryon2/internal/intelligence"
)

func TestDJSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s := &Store{dbPath: filepath.Join(t.TempDir(), "dj.db")}
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := intelligence.SessionProfile{ID: "session", Mode: intelligence.RadioTrack, Seed: &intelligence.TrackSummary{ID: "seed"}}
	if err := s.CreateDJSession(ctx, p); err != nil {
		t.Fatal(err)
	}
	pool := []intelligence.Candidate{{UnfamiliarArtist: true, ID: "candidate", Track: intelligence.TrackSummary{ID: "track"}, Channels: []intelligence.Channel{{Provider: "lastfm", Capability: "similar_tracks"}}}}
	if err := s.SaveDJCandidates(ctx, p.ID, "plan", pool); err != nil {
		t.Fatal(err)
	}
	search := intelligence.SearchPlan{Scope: intelligence.Scope{SessionID: p.ID, ConfigRevision: 7}}
	selection := intelligence.SelectionPlan{Scope: search.Scope, PoolID: "plan", Directions: []intelligence.Direction{{Label: "Atmospheric", Query: "atmospheric rock"}}}
	if err := s.SaveDJPlan(ctx, p.ID, "plan", search, selection); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDJPlan(ctx, p.ID, "plan", search, selection); err == nil {
		t.Fatal("immutable plan overwritten")
	}
	if directions, err := s.DJDirections(ctx, p.ID); err != nil || len(directions) != 1 || directions[0].Query != "atmospheric rock" {
		t.Fatal(directions, err)
	}
	if err := s.SaveDJCandidates(ctx, p.ID, "plan", pool); err == nil {
		t.Fatal("immutable pool overwritten")
	}
	if saved, err := s.DJCandidates(ctx, p.ID, "plan"); err != nil || len(saved) != 1 || saved[0].Channels[0].Provider != "lastfm" {
		t.Fatal(saved, err)
	}
	if err := s.CreateDJSession(ctx, p); err == nil {
		t.Fatal("seed/session overwrite accepted")
	}
	e := intelligence.FeedbackEvent{ID: "event", SessionID: p.ID, TrackID: "track", Kind: intelligence.FeedbackCompleted, At: time.Now().UTC()}
	if added, err := s.RecordDJFeedback(ctx, e); err != nil || !added {
		t.Fatal(added, err)
	}
	if added, err := s.RecordDJFeedback(ctx, e); err != nil || added {
		t.Fatal("duplicate not idempotent", added, err)
	}
	e.Kind = intelligence.FeedbackSkip
	if _, err := s.RecordDJFeedback(ctx, e); err == nil {
		t.Fatal("conflicting event accepted")
	}
	p, ok, err := s.DJSession(ctx, p.ID)
	if err != nil || !ok || p.Revision != 1 || p.Seed.ID != "seed" || len(p.PlayedIDs) != 1 {
		t.Fatal(p, ok, err)
	}
	e.ID = "error"
	e.Kind = intelligence.FeedbackPlaybackError
	e.TrackID = "broken"
	if _, err := s.RecordDJFeedback(ctx, e); err != nil {
		t.Fatal(err)
	}
	p, _, _ = s.DJSession(ctx, p.ID)
	if len(p.PlayedIDs) != 1 {
		t.Fatal("playback error treated as play")
	}
	for _, kind := range []intelligence.FeedbackKind{intelligence.FeedbackCandidate, intelligence.FeedbackRecommended, intelligence.FeedbackQueued, intelligence.FeedbackStarted} {
		event := intelligence.FeedbackEvent{ID: string(kind), SessionID: p.ID, TrackID: "offered", Kind: kind, At: time.Now().UTC()}
		if _, err := s.RecordDJFeedback(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	p, _, _ = s.DJSession(ctx, p.ID)
	if p.Revision != 1 || len(p.PlayedIDs) != 1 || len(p.SuggestedIDs) != 1 || p.SuggestedIDs[0] != "offered" || p.Intent == "" {
		t.Fatal("technical events changed taste or lost session context", p)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if events, err := s.DJFeedback(ctx, p.ID, 20); err != nil || len(events) != 6 {
		t.Fatal(events, err)
	}
	like := intelligence.FeedbackEvent{ID: "like", SessionID: p.ID, PlanID: "plan", TrackID: "track", Kind: intelligence.FeedbackLike, At: time.Now().UTC()}
	if _, err := s.RecordDJFeedback(ctx, like); err != nil {
		t.Fatal(err)
	}
	if counts, err := s.DJMetrics(ctx); err != nil || counts["completed"] != 1 || counts["playback_error"] != 1 || counts["unfamiliar_artist_likes"] != 1 {
		t.Fatal(counts, err)
	}
	if err := s.EndDJSession(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.DJSession(ctx, p.ID); err != nil || ok {
		t.Fatal(ok, err)
	}
	e.ID = "late"
	if _, err := s.RecordDJFeedback(ctx, e); err == nil {
		t.Fatal("closed session accepted event")
	}
	if err := s.ResetAllData(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"dj_sessions", "dj_feedback", "dj_candidates", "dj_plans"} {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
}

func TestDJMigrationFromV3(t *testing.T) {
	ctx := context.Background()
	s := &Store{dbPath: filepath.Join(t.TempDir(), "old.db")}
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, table := range []string{"dj_plans", "dj_candidates", "dj_feedback", "dj_sessions"} {
		if _, err := s.db.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE schema_migrations SET version=3 WHERE name='core'"); err != nil {
		t.Fatal(err)
	}
	if err := s.SettingSet(ctx, "existing", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if value, ok, err := s.SettingGet(ctx, "existing"); err != nil || !ok || value != "preserved" {
		t.Fatal(value, ok, err)
	}
	if err := s.CreateDJSession(ctx, intelligence.SessionProfile{ID: "new", Mode: intelligence.RadioInfinite}); err != nil {
		t.Fatal(err)
	}
}
