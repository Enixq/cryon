package intelligence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictPlanJSON(t *testing.T) {
	scope := Scope{RequestID: "r", SessionID: "s"}
	raw, _ := json.Marshal(SearchPlan{Scope: scope, Steps: []SearchStep{}})
	tests := map[string]string{"valid": string(raw), "duplicate": strings.Replace(string(raw), `"request_id":"r"`, `"request_id":"r","request_id":"x"`, 1), "missing": strings.Replace(string(raw), `"steps":[]`, `"other":[]`, 1), "trailing": string(raw) + `{}`, "null": strings.Replace(string(raw), `"steps":[]`, `"steps":null`, 1)}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			var out SearchPlan
			err := DecodeStrict([]byte(text), &out)
			if (err == nil) != (name == "valid") {
				t.Fatal(err)
			}
		})
	}
}
