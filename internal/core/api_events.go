package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type apiEventEnvelope struct {
	Event string        `json:"event"`
	Data  []interface{} `json:"data"`
}

// apiEventHub fans out idempotent state-change notifications to API v1
// clients. Consumers reload the affected resource after an event, therefore a
// slow client can safely miss an event instead of blocking application work.
type apiEventHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newAPIEventHub() *apiEventHub {
	return &apiEventHub{clients: make(map[chan []byte]struct{})}
}

func (h *apiEventHub) publish(event string, data ...interface{}) {
	if data == nil {
		data = []interface{}{}
	}
	payload, err := json.Marshal(apiEventEnvelope{Event: event, Data: data})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		select {
		case client <- payload:
		default:
		}
	}
}

func (h *apiEventHub) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := make(chan []byte, 32)
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		close(client)
		h.mu.Unlock()
	}()

	fmt.Fprint(w, ": ok\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-client:
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (a *App) eventHub() *apiEventHub {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	if a.events == nil {
		a.events = newAPIEventHub()
	}
	return a.events
}

func (a *App) emit(event string, data ...interface{}) {
	a.eventHub().publish(event, data...)
	if a.platform != nil {
		a.platform.Emit(event, data...)
	}
}
