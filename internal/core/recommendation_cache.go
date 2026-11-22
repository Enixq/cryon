package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/store"
	"context"
	"encoding/json"
	"strconv"
	"time"
)

func (a *App) cachedRecommendationResult(ctx context.Context, kind string, tracks []domain.Track, limit int) []domain.Track {
	key := "offline:recommendations:v2:" + kind + ":" + strconv.Itoa(limit) + ":" + a.sourceCacheScope(ctx)
	if len(tracks) == 0 {
		if raw, ok := store.PeekCache(context.WithoutCancel(ctx), a.store, key); ok {
			json.Unmarshal([]byte(raw), &tracks)
			for i := range tracks {
				tracks[i].FromCache = true
			}
		}
	} else if a.store != nil {
		raw, _ := json.Marshal(tracks)
		a.store.RecoCacheSet(context.WithoutCancel(ctx), key, string(raw), 24*time.Hour)
	}
	if a.reco != nil {
		return a.reco.Allowed(ctx, tracks)
	}
	return tracks
}
