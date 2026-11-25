package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/store"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (a *App) cachedSourceSearch(ctx context.Context, svc domain.MusicService, query string) ([]domain.Track, error) {
	// Local files can be added or removed at any moment. Never revive an old index.
	if svc.ID() == domain.ServiceLocal {
		return svc.Search(ctx, query)
	}
	version := "v2"
	if svc.ID() == domain.ServiceVK {
		// Previous API snapshots contain web_ ids without website reload hashes.
		version = "vk-site-v1"
	}
	key := fmt.Sprintf("search:%s:%s:%s:%x", version, svc.ID(), a.sourceCacheScope(ctx), sha256.Sum256([]byte(query)))
	if a.store != nil {
		if raw, ok, _ := a.store.RecoCacheGet(ctx, key); ok {
			var tracks []domain.Track
			if json.Unmarshal([]byte(raw), &tracks) == nil {
				return tracks, nil
			}
		}
	}
	tracks, err := svc.Search(ctx, query)
	if len(tracks) == 0 {
		if raw, ok := store.PeekCache(context.WithoutCancel(ctx), a.store, key); ok {
			var cached []domain.Track
			if json.Unmarshal([]byte(raw), &cached) == nil && len(cached) > 0 {
				for i := range cached {
					cached[i].FromCache = true
				}
				return cached, nil
			}
		}
	}
	if err == nil && len(tracks) > 0 && a.store != nil {
		raw, _ := json.Marshal(tracks)
		_ = a.store.RecoCacheSet(ctx, key, string(raw), 6*time.Hour)
	}
	return tracks, err
}

// Separate catalog snapshots when an account or source configuration changes.
// Only the digest is persisted; credentials never appear in cache keys.
func (a *App) sourceCacheScope(ctx context.Context) string {
	parts := []string{}
	for id, svc := range a.registry {
		mode := "public"
		if auth, ok := svc.(interface{ HasToken() bool }); ok {
			mode = fmt.Sprint(auth.HasToken())
		}
		parts = append(parts, string(id)+":"+mode)
	}
	if a.store != nil {
		for _, key := range []string{keyYandexToken, keyVKWebSession, keySpotifyClientID, keySpotifyClientSecret, keySoundCloudClientID, keyYouTubeAPIKey} {
			value, _, _ := a.store.SettingGet(ctx, key)
			parts = append(parts, key+":"+value)
		}
	}
	sort.Strings(parts)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}
