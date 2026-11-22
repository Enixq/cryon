package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/services/lyrics"
	"Cryon2/internal/store"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	key := fmt.Sprintf("lyrics:v2:%x", sha256.Sum256([]byte(q.Get("artist")+"\x00"+q.Get("title")+"\x00"+provider+"\x00"+q.Get("id"))))
	if raw, found := store.PeekCache(ctx, a.store, key); found {
		var cached lyrics.Result
		if json.Unmarshal([]byte(raw), &cached) == nil && cached.Found {
			writeAPIJSON(w, 200, cached)
			return
		}
	}
	reply := func(result lyrics.Result) {
		if result.Found && a.store != nil {
			raw, _ := json.Marshal(result)
			_ = a.store.RecoCacheSet(ctx, key, string(raw), 30*24*time.Hour)
		}
		writeAPIJSON(w, 200, result)
	}

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
		reply(result)
		return
	}
	if provider != "" && provider != "auto" && provider != "lrclib" && provider != "ovh" {
		writeAPIError(w, 400, "неизвестный источник текста")
		return
	}
	if provider == "auto" || provider == "" {
		result, err := a.autoLyrics(ctx, domain.Track{ID: q.Get("id"), Service: domain.ServiceID(q.Get("service")), Artists: []string{q.Get("artist")}, Title: q.Get("title"), Album: q.Get("album"), DurationMs: duration * 1000})
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		reply(result)
		return
	}

	result, err := a.lyrics.GetProvider(ctx, q.Get("artist"), q.Get("title"), q.Get("album"), duration, provider)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	reply(result)
}
