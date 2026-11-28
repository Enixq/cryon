package core

import (
	"context"
	"encoding/json"
	"net/http"

	"Cryon2/internal/domain"
)

var djVersionSources = []domain.ServiceID{domain.ServiceYandex, domain.ServiceVK, domain.ServiceSoundCloud, domain.ServiceYouTube, domain.ServiceLocal}

const djSourcePolicyKey = "intelligence.playbackSources"

type DJSourcePolicy struct {
	Enabled  map[domain.ServiceID]bool `json:"enabled"`
	Priority []domain.ServiceID        `json:"priority"`
}

type DJSourceDiagnostic struct {
	Source          domain.ServiceID `json:"source"`
	Enabled         bool             `json:"enabled"`
	Auth            string           `json:"auth"`
	SearchAttempted bool             `json:"searchAttempted"`
	Cached          int              `json:"cached"`
	Queries         int              `json:"queries"`
	Found           int              `json:"found"`
	Matched         int              `json:"matched"`
	Playable        int              `json:"playable"`
	Unchecked       int              `json:"unchecked"`
	Chosen          int              `json:"chosen"`
	Rejected        int              `json:"rejected"`
	Reasons         map[string]int   `json:"reasons"`
	ElapsedMS       int64            `json:"elapsedMs"`
	State           string           `json:"state"`
}

func (a *App) djSourcePolicy(ctx context.Context) DJSourcePolicy {
	p := DJSourcePolicy{Enabled: map[domain.ServiceID]bool{}, Priority: []domain.ServiceID{}}
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, djSourcePolicyKey)
		_ = json.Unmarshal([]byte(raw), &p)
	}
	if p.Enabled == nil {
		p.Enabled = map[domain.ServiceID]bool{}
	}
	if p.Priority == nil {
		p.Priority = []domain.ServiceID{}
	}
	for _, id := range djVersionSources {
		if _, ok := p.Enabled[id]; !ok {
			p.Enabled[id] = true
		}
	}
	return p
}

func (a *App) djSourceDiagnostics(p DJSourcePolicy) []DJSourceDiagnostic {
	out := make([]DJSourceDiagnostic, 0, len(djVersionSources))
	for _, id := range djVersionSources {
		d := DJSourceDiagnostic{Source: id, Enabled: p.Enabled[id], Auth: "not_required", State: "ready", Reasons: map[string]int{}}
		svc := a.registry[id]
		if id == domain.ServiceVK || id == domain.ServiceYandex {
			d.Auth = "missing"
			if auth, ok := svc.(interface{ HasToken() bool }); ok && auth.HasToken() {
				d.Auth = "session_present"
			}
		}
		switch {
		case !d.Enabled:
			d.State = "disabled"
		case svc == nil:
			d.State = "unsupported"
		default:
			if d.Auth == "missing" {
				d.State = "no_auth"
			}
		}
		out = append(out, d)
	}
	return out
}

func (a *App) handleDJSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется GET или POST")
		return
	}
	if r.Method == http.MethodPost {
		var p DJSourcePolicy
		if decodeAPIJSON(w, r, &p) != nil {
			return
		}
		valid := map[domain.ServiceID]bool{}
		for _, id := range djVersionSources {
			valid[id] = true
		}
		seen := map[domain.ServiceID]bool{}
		for id := range p.Enabled {
			if !valid[id] {
				writeAPIError(w, 400, "неизвестный источник")
				return
			}
		}
		for _, id := range p.Priority {
			if !valid[id] || seen[id] {
				writeAPIError(w, 400, "некорректный порядок источников")
				return
			}
			seen[id] = true
		}
		if a.store == nil {
			writeAPIError(w, 503, "хранилище недоступно")
			return
		}
		raw, _ := json.Marshal(p)
		if err := a.store.SettingSet(r.Context(), djSourcePolicyKey, string(raw)); err != nil {
			writeAPIError(w, 500, "не удалось сохранить источники")
			return
		}
		// Invalidate pending and prepared queues using the existing revision
		// mechanism. No model request is made by changing playback settings.
		if err := a.ConfigureIntelligence(a.djConfig(r.Context())); err != nil {
			writeAPIError(w, 500, "источники сохранены; обновите настройки DJ")
			return
		}
	}
	p := a.djSourcePolicy(r.Context())
	writeAPIJSON(w, 200, struct {
		DJSourcePolicy
		Sources []DJSourceDiagnostic `json:"sources"`
	}{p, a.djSourceDiagnostics(p)})
}
