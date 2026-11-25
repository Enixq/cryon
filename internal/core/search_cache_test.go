package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/logging"
	"Cryon2/internal/services/local"
	"Cryon2/internal/store/sqlite"
)

type cancelledSearchService struct{ started chan struct{} }

func (s *cancelledSearchService) ID() domain.ServiceID { return domain.ServiceYandex }
func (s *cancelledSearchService) Search(ctx context.Context, _ string) ([]domain.Track, error) {
	close(s.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAPISearchCancellationStopsProvider(t *testing.T) {
	svc := &cancelledSearchService{started: make(chan struct{})}
	app := &App{ctx: context.Background(), registry: map[domain.ServiceID]domain.MusicService{svc.ID(): svc}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.handleAPISearch(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/search?service=yandex&q=Song", nil).WithContext(ctx))
	}()
	select {
	case <-svc.started:
	case <-time.After(time.Second):
		t.Fatal("search did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled HTTP search continued querying the provider")
	}
}

type cacheSearchService struct {
	calls     int
	offline   bool
	serviceID domain.ServiceID
}

func (s *cacheSearchService) ID() domain.ServiceID {
	if s.serviceID != "" {
		return s.serviceID
	}
	return domain.ServiceYandex
}
func (s *cacheSearchService) Search(context.Context, string) ([]domain.Track, error) {
	s.calls++
	if s.offline {
		return nil, errors.New("offline")
	}
	return []domain.Track{{ID: "one", Service: domain.ServiceYandex}}, nil
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
	db.SettingSet(ctx, keyYandexToken, "another-account")
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
	db.SettingSet(ctx, keyYandexToken, "")
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

func TestVKWebsiteSearchDoesNotRevivePreviousAPISnapshots(t *testing.T) {
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
	svc := &cacheSearchService{serviceID: domain.ServiceVK}
	app := &App{store: db, registry: map[domain.ServiceID]domain.MusicService{svc.ID(): svc}}
	oldKey := fmt.Sprintf("search:v2:%s:%s:%x", svc.ID(), app.sourceCacheScope(ctx), sha256.Sum256([]byte("Song")))
	raw, _ := json.Marshal([]domain.Track{{ID: "web_1_2", Service: domain.ServiceVK}})
	if err := db.RecoCacheSet(ctx, oldKey, string(raw), 6*time.Hour); err != nil {
		t.Fatal(err)
	}
	tracks, err := app.cachedSourceSearch(ctx, svc, "Song")
	if err != nil || len(tracks) != 1 || tracks[0].ID == "web_1_2" || svc.calls != 1 {
		t.Fatal("legacy API cache reused", tracks, err)
	}
	if _, err = app.cachedSourceSearch(ctx, svc, "Song"); err != nil || svc.calls != 1 {
		t.Fatal("website snapshot not cached", err)
	}
}
