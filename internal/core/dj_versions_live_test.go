package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"Cryon2/internal/logging"
)

// Opt-in only. Point to a disposable SQLite backup's parent, never live data.
// No startup workers, collection API or model director is created.
func TestManualDJVersionSources(t *testing.T) {
	root := os.Getenv("CRYON_DJ_VERSION_PROBE")
	if root == "" {
		t.Skip("requires disposable diagnostic snapshot")
	}
	logging.SetDataDirOverride(root)
	defer logging.SetDataDirOverride("")
	a := NewApp()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a.ctx = ctx
	if err := a.store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if closer, ok := a.store.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	a.loadLocalLibraryCache(ctx)
	a.restoreYandexToken(ctx)
	a.restoreLastFMKey(ctx)
	a.restoreServiceCredentials(ctx)
	if validator, ok := a.registry[domain.ServiceVK].(interface{ ValidateToken(context.Context) error }); ok {
		check, end := context.WithTimeout(ctx, 8*time.Second)
		err := validator.ValidateToken(check)
		end()
		t.Logf("PROBE_VK_SESSION validated=%t error_type=%T", err == nil, err)
	}
	favorites, err := a.store.FavoriteList(ctx)
	if err != nil || len(favorites) == 0 {
		t.Fatal("no diagnostic seed", err)
	}
	taste := a.reco.IntelligenceTaste(ctx)
	cfg := a.djConfig(ctx)
	cfg.APIKey = "" // Explicitly no model calls, even if configured.
	for _, mode := range []intelligence.RadioMode{intelligence.RadioTrack, intelligence.RadioInfinite} {
		session := intelligence.SessionProfile{ID: djID(), Mode: mode}
		if mode == intelligence.RadioTrack {
			seed := djSummary(favorites[0])
			session.Seed = &seed
		}
		in := intelligence.PlanningInput{Scope: intelligence.Scope{RequestID: djID(), SessionID: session.ID, ConfigRevision: cfg.Revision}, Session: session, Taste: taste, Channels: a.reco.DJChannels()}
		started := time.Now()
		d := DJDiagnostic{Mode: string(mode)}
		result := a.coordinateDJ(ctx, cfg, in, djRadioRequest{Mode: mode, Track: favorites[0], Limit: 12}, nil, nil, &d)
		queue, candidates := a.resolveDJVersions(ctx, result.tracks, result.candidates, &d)
		data, _ := json.Marshal(struct {
			Mode                     intelligence.RadioMode
			ElapsedMS                int64
			Ranked, Queued, LLMCalls int
			Sources                  []DJSourceDiagnostic
		}{mode, time.Since(started).Milliseconds(), len(result.tracks), len(queue), d.LLMCalls, d.Sources})
		t.Log("PROBE", string(data))
		for _, c := range candidates {
			for _, v := range c.PlaybackCandidates {
				if v.SelectionReason != "" {
					t.Logf("PROBE_SELECTED %s | %s | %.2f | %s", v.Track.Service, v.Track.Title, v.IdentityConfidence, v.AvailabilityState)
				}
			}
		}
	}
}
