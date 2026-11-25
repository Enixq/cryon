package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"Cryon2/internal/intelligence"
)

func TestProviderFailuresAndDeadline(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"message":"secret must not escape"}}`))
			}))
			defer s.Close()
			d, err := New("compatible", "model", "test-key", s.URL)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if _, err = d.PlanSearch(ctx, intelligence.PlanningInput{}); err == nil {
				t.Fatal("error accepted")
			}
			if calls.Load() != 1 {
				t.Fatal("retried HTTP request", calls.Load())
			}
		})
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer s.Close()
	d, _ := New("compatible", "model", "test", s.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := d.PlanSearch(ctx, intelligence.PlanningInput{}); err == nil || time.Since(start) > time.Second {
		t.Fatal("deadline ignored", err)
	}
}
func TestProviderStrictOutput(t *testing.T) {
	for _, provider := range []string{"openai", "compatible", "anthropic"} {
		t.Run(provider, func(t *testing.T) {
			plan := intelligence.SearchPlan{Scope: intelligence.Scope{RequestID: "request"}, Steps: []intelligence.SearchStep{}}
			raw, _ := json.Marshal(plan)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if provider == "anthropic" {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg", "type": "message", "role": "assistant", "model": "model", "stop_reason": "end_turn", "content": []map[string]any{{"type": "text", "text": string(raw)}}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 1}})
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "response", "choices": []map[string]any{{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(raw)}}}})
				}
			}))
			defer server.Close()
			d, err := New(provider, "model", "test", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.PlanSearch(context.Background(), intelligence.PlanningInput{})
			if err != nil || got.Scope.RequestID != "request" {
				t.Fatal(got, err)
			}
		})
	}
}
