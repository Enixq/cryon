package core

import (
	"Cryon2/internal/domain"
	"Cryon2/internal/intelligence"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type djCollection struct {
	ResultID  string         `json:"resultId"`
	JobID     string         `json:"jobId"`
	SessionID string         `json:"sessionId"`
	Job       DJDiagnostic   `json:"job"`
	Tracks    []domain.Track `json:"tracks"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

func (a *App) loadDJCollection(ctx context.Context) djCollection {
	c := djCollection{Tracks: []domain.Track{}}
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, "intelligence.collection")
		_ = json.Unmarshal([]byte(raw), &c)
	}
	if c.Tracks == nil {
		c.Tracks = []domain.Track{}
	}
	if a.store != nil {
		raw, _, _ := a.store.SettingGet(ctx, "intelligence.private.collectionJob")
		_ = json.Unmarshal([]byte(raw), &c.Job)
	}
	return c
}
func (a *App) handleDJCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется GET или POST")
		return
	}
	if r.Method == http.MethodPost {
		cfg := a.djConfig(r.Context())

		repo, ok := a.store.(intelligence.SessionRepository)
		if !ok || a.reco == nil {
			writeAPIError(w, 503, "движок недоступен")
			return
		}
		// A dedicated session for the visible collection does not change active radio.
		raw, _, _ := a.store.SettingGet(r.Context(), "intelligence.collection.session")
		session, exists, err := repo.DJSession(r.Context(), raw)
		if err != nil {
			writeAPIAppError(w, err)
			return
		}
		if !exists {
			session = intelligence.SessionProfile{ID: djID(), Mode: intelligence.RadioInfinite, PlayedIDs: []string{}}
			if err = repo.CreateDJSession(r.Context(), session); err != nil {
				writeAPIAppError(w, err)
				return
			}
			if err = a.store.SettingSet(r.Context(), "intelligence.collection.session", session.ID); err != nil {
				writeAPIAppError(w, err)
				return
			}
		}
		a.dj.Lock()
		busy := a.dj.busy
		cooldown := a.dj.cooldown
		if busy {
			cooldown = time.Time{}
		}
		a.dj.Unlock()
		if busy {
			a.dj.Lock()
			cancel, done := a.dj.cancel, a.dj.done
			a.dj.Unlock()
			if cancel != nil {
				cancel()
			}
			if done != nil {
				select {
				case <-done:
				case <-r.Context().Done():
					return
				case <-time.After(3 * time.Second):
					writeAPIError(w, 409, "Предыдущая задача отменяется")
					return
				}
			}
		}
		if busy {
			a.dj.Lock()
			if !a.dj.busy {
				a.dj.cooldown = time.Time{}
			}
			a.dj.Unlock()
		}

		if time.Now().Before(cooldown) {
			writeAPIError(w, 429, "Предыдущая AI-подборка не завершилась. Проверьте модель и ключ; повторить можно через минуту")
			return
		}
		req := djRadioRequest{Collection: true, SessionID: session.ID, Mode: intelligence.RadioInfinite, Limit: 40}
		if err := a.store.SettingSet(r.Context(), "intelligence.private.failedRevision", ""); err != nil {
			writeAPIAppError(w, err)
			return
		}
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		a.prepareDJ(cfg, session, a.reco.IntelligenceTaste(ctx), req, nil)

	}
	c := a.loadDJCollection(r.Context())
	a.dj.Lock()
	active := a.dj.busy && a.dj.jobID == c.Job.JobID
	a.dj.Unlock()
	if c.Job.JobID != "" && !active && (c.Job.Stage == "planning" || c.Job.Stage == "searching" || c.Job.Stage == "ranking" || c.Job.Stage == "saving") {
		c.Job.Stage = "cancelled"
		c.Job.Error = "Подготовка прервана. Можно запустить новую; предыдущий результат сохранён"
		a.saveDJDiagnostic(r.Context(), c.Job)
	}
	if a.reco != nil {
		c.Tracks = a.reco.Allowed(r.Context(), c.Tracks)
	}
	writeAPIJSON(w, 200, c)
}
