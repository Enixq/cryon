package core

import (
	"net/http"
	"strings"
)

func (a *App) handleAPIRecommendationBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, 405, "требуется POST")
		return
	}
	var req struct {
		Artist  string `json:"artist"`
		Blocked bool   `json:"blocked"`
	}
	if decodeAPIJSON(w, r, &req) != nil {
		return
	}
	req.Artist = strings.TrimSpace(req.Artist)
	if req.Artist == "" || len(req.Artist) > 500 {
		writeAPIError(w, 400, "укажите исполнителя")
		return
	}
	if a.reco == nil {
		writeAPIError(w, 503, "движок недоступен")
		return
	}
	score := 0
	if req.Blocked {
		score = -1
	}
	state, err := a.reco.SetFeedback(r.Context(), req.Artist, "*", score)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, 200, state)
}
