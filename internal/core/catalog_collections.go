package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/services/yandex"
	"context"
	"encoding/json"
	"net/http"
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
	key := "catalog:v2:" + kind
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
		if service, ok := a.registry[domain.ServiceYandex].(*yandex.Service); ok {
			tracks, err = service.ChartTracks(ctx, 100)
		} else {
			writeAPIError(w, 503, "Российский чарт недоступен. Подключите Яндекс Музыку.")
			return
		}
	} else {
		tracks, err = a.reco.CatalogCollection(ctx, kind, 60)
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
