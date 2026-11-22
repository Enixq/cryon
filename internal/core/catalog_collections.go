package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/services/deezer"
	"Cryon2/internal/services/yandex"
	"Cryon2/internal/store"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (a *App) handleAPICatalogCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, 405, "требуется GET")
		return
	}
	kind := r.URL.Query().Get("kind")
	limit := apiLimit(r, 40)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	key := "catalog:v4:" + kind + ":" + a.sourceCacheScope(ctx)
	if a.store != nil {
		if raw, ok, _ := a.store.RecoCacheGet(ctx, key); ok {
			var tracks []domain.Track
			if json.Unmarshal([]byte(raw), &tracks) == nil {
				if len(tracks) > limit {
					tracks = tracks[:limit]
				}
				writeAPITracks(w, tracks)
				return
			}
		}
	}
	var tracks []domain.Track
	var err error
	if kind == "russia-chart" {
		service, _ := a.registry[domain.ServiceYandex].(*yandex.Service)
		if service == nil {
			service = yandex.New("")
		}
		chartCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		metadata, chartErr := service.ChartTracks(chartCtx, 60)
		stop()
		if chartErr != nil || len(metadata) == 0 {
			metadata, chartErr = russianAppleChart(ctx)
		}
		if chartErr != nil || len(metadata) == 0 {
			if raw, ok := store.PeekCache(ctx, a.store, "chart:russia:metadata"); ok {
				_ = json.Unmarshal([]byte(raw), &metadata)
			}
		} else if a.store != nil {
			raw, _ := json.Marshal(metadata)
			_ = a.store.RecoCacheSet(ctx, "chart:russia:metadata", string(raw), 24*time.Hour)
		}
		catalog := make([]deezer.CatalogTrack, 0, len(metadata))
		for _, track := range metadata {
			var item deezer.CatalogTrack
			item.Title = track.Title
			item.Artist.Name = strings.Join(track.Artists, ", ")
			item.Duration = track.DurationMs / 1000
			item.Album.Title = track.Album
			item.Album.Cover = track.ArtworkURL
			catalog = append(catalog, item)
		}
		tracks = a.reco.ResolveCatalog(ctx, catalog, limit)

	} else {
		tracks, err = a.reco.CatalogCollection(ctx, kind, 60)
	}
	if (err != nil || len(tracks) == 0) && a.store != nil {
		if raw, ok := store.PeekCache(ctx, a.store, key); ok {
			_ = json.Unmarshal([]byte(raw), &tracks)
			for i := range tracks {
				tracks[i].FromCache = true
			}
			if len(tracks) > 0 {
				err = nil
			}
		}
	}
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if a.store != nil && len(tracks) > 0 {
		data, _ := json.Marshal(tracks)
		_ = a.store.RecoCacheSet(ctx, key, string(data), 3*time.Hour)
	}
	if len(tracks) > limit {
		tracks = tracks[:limit]
	}
	writeAPITracks(w, tracks)
}
