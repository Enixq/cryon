package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/services/lyrics"
	"context"
	"net/http"
	"strconv"
	"time"
)

func (a *App) handleAPILyrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, 405, "требуется GET")
		return
	}
	q := r.URL.Query()
	provider := q.Get("provider")
	duration, _ := strconv.Atoi(q.Get("duration"))
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	source, ok := a.registry[domain.ServiceID(q.Get("service"))].(interface {
		TrackLyrics(context.Context, string) (lyrics.Result, error)
	})
	if provider == "source" {
		if !ok {
			writeAPIJSON(w, 200, lyrics.Result{})
			return
		}
		result, err := source.TrackLyrics(ctx, q.Get("id"))
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		writeAPIJSON(w, 200, result)
		return
	}
	if provider != "" && provider != "auto" && provider != "lrclib" && provider != "ovh" {
		writeAPIError(w, 400, "неизвестный источник текста")
		return
	}
	if provider == "auto" || provider == "" {
		if ok {
			sourceCtx, sourceCancel := context.WithTimeout(ctx, 8*time.Second)
			result, err := source.TrackLyrics(sourceCtx, q.Get("id"))
			sourceCancel()
			if err == nil && result.Found {
				writeAPIJSON(w, 200, result)
				return
			}
		}
	}
	result, err := a.lyrics.GetProvider(ctx, q.Get("artist"), q.Get("title"), q.Get("album"), duration, provider)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, 200, result)
}
