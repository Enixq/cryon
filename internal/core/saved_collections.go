package core

import (
	"Cryon2/internal/domain"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

var savedCollectionsMu sync.Mutex

const savedCollectionsKey = "library.savedCollections.v1"

type SavedCollection struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title"`
	Service    string         `json:"service"`
	CatalogID  string         `json:"catalogId,omitempty"`
	Artist     string         `json:"artist,omitempty"`
	ArtworkURL string         `json:"artworkUrl,omitempty"`
	Tracks     []domain.Track `json:"tracks"`
}

func (a *App) handleAPISavedCollections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		writeAPIError(w, 405, "требуется GET, PUT или DELETE")
		return
	}
	if a.store == nil {
		writeAPIError(w, 503, "хранилище недоступно")
		return
	}
	savedCollectionsMu.Lock()
	defer savedCollectionsMu.Unlock()
	raw, _, err := a.store.SettingGet(r.Context(), savedCollectionsKey)
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	items := []SavedCollection{}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &items); err != nil {
			writeAPIAppError(w, err)
			return
		}
	}
	if r.Method == http.MethodGet {
		writeAPIJSON(w, 200, items)
		return
	}
	id := r.URL.Query().Get("id")
	var item SavedCollection
	if r.Method == http.MethodPut {
		if decodeAPIJSON(w, r, &item) != nil {
			return
		}
		id = strings.TrimSpace(item.ID)
		if id == "" || len(id) > 512 || strings.TrimSpace(item.Title) == "" || (item.Kind != "album" && item.Kind != "playlist") || len(item.Tracks) > 1000 {
			writeAPIError(w, 400, "некорректная коллекция")
			return
		}
		if _, ok := a.registry[domain.ServiceID(item.Service)]; !ok {
			writeAPIError(w, 400, "неизвестный источник")
			return
		}
	}
	if id == "" {
		writeAPIError(w, 400, "нужен id")
		return
	}
	out := []SavedCollection{}
	if r.Method == http.MethodPut {
		out = append(out, item)
	}
	for _, saved := range items {
		if saved.ID != id {
			out = append(out, saved)
		}
	}
	if len(out) > 500 {
		writeAPIError(w, 400, "достигнут предел коллекций")
		return
	}
	data, err := json.Marshal(out)
	if err == nil {
		err = a.store.SettingSet(r.Context(), savedCollectionsKey, string(data))
	}
	if err != nil {
		writeAPIAppError(w, err)
		return
	}
	if a.platform != nil {
		a.platform.Emit("collections:changed")
	}
	writeAPIJSON(w, 200, out)
}
