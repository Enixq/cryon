package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"Cryon2/internal/intelligence"
)

func TestCompatibleJSONAndForcedSSE(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			plan := intelligence.SearchPlan{Scope: intelligence.Scope{RequestID: "request"}, Steps: []intelligence.SearchStep{}}
			raw, _ := json.Marshal(plan)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request map[string]any
				_ = json.NewDecoder(r.Body).Decode(&request)
				if request["max_completion_tokens"] != float64(2500) || request["reasoning_effort"] != "low" || request["stream"] != false {
					t.Error("wrong reasoning request", request["max_completion_tokens"], request["reasoning_effort"])
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": string(raw)}, "finish_reason": "stop"}}, "usage": map[string]int{"total_tokens": 123}})
					_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 23}})
				}
			}))
			defer server.Close()
			d, _ := New("compatible", "gpt-5.6-terra", "mock", server.URL)
			got, err := d.PlanSearch(context.Background(), intelligence.PlanningInput{})
			if err != nil || got.Scope.RequestID != "request" || d.TokenUsage() != 123 || calls.Load() != 1 {
				t.Fatal(got, err, d.TokenUsage(), calls.Load())
			}
		})
	}
}

func TestInvalidOrEmptyProviderResponseIsActionable(t *testing.T) {
	for _, body := range []string{`<html>secret api-key</html>`, `{"choices":[]}`, `{"choices":[{"message":{"content":""},"finish_reason":"stop"}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) }))
		d, _ := New("compatible", "mock", "private-key", server.URL)
		_, err := d.PlanSearch(context.Background(), intelligence.PlanningInput{})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private-key") || strings.Contains(err.Error(), "проверьте соединение") {
			t.Fatal("unsafe or generic failure", err)
		}
	}
}
