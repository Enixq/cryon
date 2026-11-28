package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Behavior from ordinary playback contributes to taste without inventing an
// active DJ session or calling any model. IDs make repeated delivery idempotent.
func (a *App) handleDJBehavior(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeAPIError(w, 405, "требуется POST")
		return
	}
	var in struct {
		Track domain.Track               `json:"track"`
		Event intelligence.FeedbackEvent `json:"event"`
	}
	if decodeAPIJSON(w, r, &in) != nil {
		return
	}
	e := in.Event
	valid := e.Kind == intelligence.FeedbackLike || e.Kind == intelligence.FeedbackDislike || e.Kind == intelligence.FeedbackSkip || e.Kind == intelligence.FeedbackCompleted || e.Kind == intelligence.FeedbackReplay
	if !valid || e.ID == "" || len(e.ID) > 128 || validateAPITrack(in.Track) != nil || e.PlayedMS < 0 || e.DurationMS < 0 || e.PlayedMS > 24*60*60*1000 || e.DurationMS > 24*60*60*1000 {
		writeAPIError(w, 400, "некорректное событие")
		return
	}
	// Arrival time is authoritative; arbitrary client timestamps cannot amplify weights.
	e.At = time.Now().UTC()
	e.TrackID = djTrackID(in.Track)
	if e.DurationMS > 0 && e.PlayedMS > e.DurationMS {
		e.PlayedMS = e.DurationMS
	}
	if err := a.appendDJReaction(r.Context(), djReaction{e, djSummary(in.Track)}); err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) appendDJReaction(ctx context.Context, row djReaction) error {
	if a.store == nil {
		return fmt.Errorf("хранилище недоступно")
	}
	a.dj.Lock()
	defer a.dj.Unlock()
	raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.reactions")
	rows := []djReaction{}
	_ = json.Unmarshal([]byte(raw), &rows)
	for _, r := range rows {
		if r.Event.ID == row.Event.ID {
			return nil
		}
	}
	rows = append(rows, row)
	if len(rows) > 2000 {
		rows = rows[len(rows)-2000:]
	}
	data, _ := json.Marshal(rows)
	return a.store.SettingSet(ctx, "intelligence.private.reactions", string(data))
}
