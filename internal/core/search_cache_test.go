package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/services/local"
	"Cryon2/internal/store/sqlite"
)

type cacheSearchService struct {
	calls   int
	offline bool
}

func (s *cacheSearchService) ID() domain.ServiceID { return domain.ServiceVK }
func (s *cacheSearchService) Search(context.Context, string) ([]domain.Track, error) {
	s.calls++
	if s.offline {
		return nil, errors.New("offline")
	}
	return []domain.Track{{ID: "one", Service: domain.ServiceVK}}, nil
}

func TestSearchCacheScopesAccountsAndUsesStaleResultsOffline(t *testing.T) {
	logging.SetDataDirOverride(t.TempDir())
	defer logging.SetDataDirOverride("")
	db, err := sqlite.New("Cryon2")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.Init(ctx); err != nil {
		t.Fatal(err)
	}
	svc := &cacheSearchService{}
	app := &App{store: db, registry: map[domain.ServiceID]domain.MusicService{svc.ID(): svc}}
	first, err := app.cachedSourceSearch(ctx, svc, "Song")
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	app.cachedSourceSearch(ctx, svc, "Song")
	if svc.calls != 1 {
		t.Fatal("fresh cache was not used")
	}
	db.SettingSet(ctx, "vk.token", "another-account")
	app.cachedSourceSearch(ctx, svc, "Song")
	if svc.calls != 2 {
		t.Fatal("previous account cache was reused")
	}
	// Expire snapshots without deleting them, then simulate a network outage.
	raw, _ := json.Marshal(first)
	searchKey := fmt.Sprintf("search:v2:%s:%s:%x", svc.ID(), app.sourceCacheScope(ctx), sha256.Sum256([]byte("Song")))
	db.RecoCacheSet(ctx, searchKey, string(raw), time.Nanosecond)
	svc.offline = true
	foundOffline, err := app.cachedSourceSearch(ctx, svc, "Song")
	if err != nil || len(foundOffline) != 1 || !foundOffline[0].FromCache {
		t.Fatal("offline search lost", foundOffline, err)
	}
	key := "offline:recommendations:v2:radio:1:" + app.sourceCacheScope(ctx)
	db.RecoCacheSet(ctx, key, string(raw), time.Nanosecond)
	restored := app.cachedRecommendationResult(ctx, "radio", nil, 1)
	if len(restored) != 1 || !restored[0].FromCache {
		t.Fatal("expired recommendation snapshot was not restored", restored)
	}
	db.SettingSet(ctx, "vk.token", "")
	if len(app.cachedRecommendationResult(ctx, "radio", nil, 1)) != 0 {
		t.Fatal("signed-out account revived")
	}
	localSvc := local.New()
	localSvc.Replace([]local.LocalTrack{{Track: domain.Track{ID: "file", Title: "Song", Service: domain.ServiceLocal}}})
	app.cachedSourceSearch(ctx, localSvc, "Song")
	localSvc.Replace(nil)
	found, err := app.cachedSourceSearch(ctx, localSvc, "Song")
	if err != nil || len(found) != 0 {
		t.Fatal("removed local file restored from cache", found, err)
	}
}
