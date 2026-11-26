package core

import (
	"context"
	"encoding/json"
	"time"
)

type DJSearchStatus struct {
	Source string `json:"source"`
	Query  string `json:"query"`
	Found  int    `json:"found"`
	State  string `json:"state"`
}
type DJDiagnostic struct {
	ConfigRevision uint64           `json:"configRevision"`
	Stage          string           `json:"stage"`
	Error          string           `json:"error,omitempty"`
	Found          int              `json:"found"`
	Candidates     int              `json:"candidates"`
	Selected       int              `json:"selected"`
	Searches       []DJSearchStatus `json:"searches"`
	UpdatedAt      string           `json:"updatedAt"`
}

func (a *App) saveDJDiagnostic(ctx context.Context, d DJDiagnostic) {
	if a.store == nil {
		return
	}
	d.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if d.Searches == nil {
		d.Searches = []DJSearchStatus{}
	}
	raw, _ := json.Marshal(d)
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = a.store.SettingSet(writeCtx, "intelligence.private.diagnostic", string(raw))
}
func (a *App) loadDJDiagnostic(ctx context.Context, revision uint64) DJDiagnostic {
	d := DJDiagnostic{}
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.diagnostic")
		_ = json.Unmarshal([]byte(raw), &d)
	}
	if d.ConfigRevision != revision {
		return DJDiagnostic{}
	}
	return d
}
