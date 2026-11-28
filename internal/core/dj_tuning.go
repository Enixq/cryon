package core

import (
	"Cryon2/internal/intelligence"
	"Cryon2/internal/recommendations"
	"encoding/json"
	"net/http"
)

type djTuning struct {
	Ranking  recommendations.RankingWeights `json:"ranking"`
	Behavior intelligence.BehaviorWeights   `json:"behavior"`
}

func (a *App) handleDJTuning(w http.ResponseWriter, r *http.Request) {
	v := djTuning{recommendations.DefaultRankingWeights(), intelligence.DefaultBehaviorWeights()}
	if r.Method == "GET" {
		raw, _, _ := a.store.SettingGet(r.Context(), "intelligence.ranking.weights")
		_ = json.Unmarshal([]byte(raw), &v.Ranking)
		raw, _, _ = a.store.SettingGet(r.Context(), "intelligence.behavior.weights")
		_ = json.Unmarshal([]byte(raw), &v.Behavior)
		writeAPIJSON(w, 200, v)
		return
	}
	if r.Method != "POST" {
		writeAPIError(w, 405, "требуется GET или POST")
		return
	}
	if decodeAPIJSON(w, r, &v) != nil {
		return
	}
	if !v.Ranking.Valid() || !v.Behavior.Valid() {
		writeAPIError(w, 400, "некорректные веса")
		return
	}
	ranking, _ := json.Marshal(v.Ranking)
	behavior, _ := json.Marshal(v.Behavior)
	if err := a.store.SettingSet(r.Context(), "intelligence.ranking.weights", string(ranking)); err != nil {
		writeAPIAppError(w, err)
		return
	}
	if err := a.store.SettingSet(r.Context(), "intelligence.behavior.weights", string(behavior)); err != nil {
		writeAPIAppError(w, err)
		return
	}
	cfg := a.djConfig(r.Context())
	if err := a.ConfigureIntelligence(cfg); err != nil {
		writeAPIAppError(w, err)
		return
	}
	writeAPIJSON(w, 200, v)
}
