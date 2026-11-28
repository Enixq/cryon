package core

import (
	"Cryon2/internal/intelligence"
	"context"
	"encoding/json"
	"time"
)

type datedDJSuggestion struct {
	Track intelligence.TrackSummary `json:"track"`
	At    time.Time                 `json:"at"`
}

func (a *App) recentDJSuggestions(ctx context.Context) []datedDJSuggestion {
	raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.suggestions.v2")
	var stored []datedDJSuggestion
	_ = json.Unmarshal([]byte(raw), &stored)
	out := []datedDJSuggestion{}
	for _, row := range stored {
		age := time.Since(row.At)
		if age >= 0 && age < 24*time.Hour {
			out = append(out, row)
		}
	}
	return out
}
func (a *App) saveDJSuggestions(ctx context.Context, tracks []intelligence.TrackSummary) {
	rows := a.recentDJSuggestions(ctx)
	replaced := map[string]bool{}
	for _, t := range tracks {
		replaced[t.ID] = true
	}
	keep := rows[:0]
	for _, r := range rows {
		if !replaced[r.Track.ID] {
			keep = append(keep, r)
		}
	}
	for _, t := range tracks {
		keep = append(keep, datedDJSuggestion{t, time.Now().UTC()})
	}
	if len(keep) > 200 {
		keep = keep[len(keep)-200:]
	}
	raw, _ := json.Marshal(keep)
	_ = a.store.SettingSet(ctx, "intelligence.private.suggestions.v2", string(raw))
}
