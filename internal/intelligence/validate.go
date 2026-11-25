package intelligence

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Limits are owned by Cryon, never supplied by the model.
type Limits struct {
	MaxSteps       int
	MaxCandidates  int
	MaxDirections  int
	MaxQueryRunes  int
	MaxReasonRunes int
}

func (l Limits) valid() bool {
	return l.MaxSteps > 0 && l.MaxCandidates > 0 && l.MaxDirections >= 0 && l.MaxQueryRunes > 0 && l.MaxReasonRunes > 0
}

func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
func bounded(s string, n int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= n
}

func validateContext(in PlanningInput) error {
	if in.Scope.RequestID == "" || in.Scope.SessionID == "" || in.Scope.SessionID != in.Session.ID || in.Scope.SessionRevision != in.Session.Revision || in.Scope.TasteRevision != in.Taste.Revision {
		return fmt.Errorf("inconsistent planning scope")
	}
	if in.Session.Mode != RadioInfinite && in.Session.Mode != RadioTrack {
		return fmt.Errorf("unknown radio mode")
	}
	if in.Session.Mode == RadioTrack && (in.Session.Seed == nil || in.Session.Seed.ID == "") {
		return fmt.Errorf("track radio requires seed")
	}
	return nil
}

// ValidateSearch rejects the entire proposal on any invalid step. It does not
// silently clamp plans: the existing engine is the fallback for invalid output.
func ValidateSearch(in PlanningInput, plan SearchPlan, limits Limits) error {
	if !limits.valid() {
		return fmt.Errorf("invalid limits")
	}
	if err := validateContext(in); err != nil {
		return err
	}
	if plan.Scope != in.Scope {
		return fmt.Errorf("stale search plan")
	}
	if !unit(plan.Strategy.Exploration) || !unit(plan.Strategy.SeedAdherence) || !unit(plan.Strategy.Diversity) {
		return fmt.Errorf("invalid strategy")
	}
	if len(plan.Steps) == 0 || len(plan.Steps) > limits.MaxSteps {
		return fmt.Errorf("invalid step count")
	}
	available := make(map[Channel]int, len(in.Channels))
	for _, c := range in.Channels {
		if c.Channel.Provider == "" || c.Channel.Capability == "" || c.MaxCandidates <= 0 {
			return fmt.Errorf("invalid channel configuration")
		}
		if _, exists := available[c.Channel]; exists {
			return fmt.Errorf("duplicate channel configuration")
		}
		available[c.Channel] = c.MaxCandidates
	}
	total := 0
	for _, step := range plan.Steps {
		remaining, ok := available[step.Channel]
		if !ok || step.Limit <= 0 || step.Limit > remaining || step.Limit > limits.MaxCandidates-total {
			return fmt.Errorf("unavailable channel or exceeded search budget")
		}
		if !bounded(step.Query, limits.MaxQueryRunes) {
			return fmt.Errorf("invalid search query")
		}
		available[step.Channel] -= step.Limit
		total += step.Limit
	}
	return nil
}

// ValidateSelection verifies membership in the exact pool. Avoid is a temporary
// hint, never permission to persist user dislikes or bypass final engine rules.
func ValidateSelection(in SelectionInput, plan SelectionPlan, limits Limits) error {
	if !limits.valid() {
		return fmt.Errorf("invalid limits")
	}
	if err := validateContext(in.Context); err != nil {
		return err
	}
	if plan.Scope != in.Context.Scope || in.PoolID == "" || plan.PoolID != in.PoolID {
		return fmt.Errorf("stale selection plan")
	}
	if len(in.Candidates) == 0 || len(in.Candidates) > limits.MaxCandidates {
		return fmt.Errorf("invalid candidate pool")
	}
	pool := make(map[string]bool, len(in.Candidates))
	for _, c := range in.Candidates {
		if c.ID == "" || pool[c.ID] {
			return fmt.Errorf("invalid candidate identity")
		}
		pool[c.ID] = true
	}
	if len(plan.Ranked) == 0 || len(plan.Ranked) > len(pool) || len(plan.Avoid) > len(pool) || len(plan.Directions) > limits.MaxDirections {
		return fmt.Errorf("invalid selection size")
	}
	seen := make(map[string]bool)
	previous := 1.0
	for _, decision := range plan.Ranked {
		if !pool[decision.ID] || seen[decision.ID] || !unit(decision.Score) || decision.Score > previous || !bounded(decision.Reason, limits.MaxReasonRunes) {
			return fmt.Errorf("invalid candidate decision")
		}
		seen[decision.ID] = true
		previous = decision.Score
	}
	for _, id := range plan.Avoid {
		if !pool[id] || seen[id] {
			return fmt.Errorf("invalid avoid candidate")
		}
		seen[id] = true
	}
	for _, d := range plan.Directions {
		if !bounded(d.Label, limits.MaxQueryRunes) || !bounded(d.Query, limits.MaxQueryRunes) {
			return fmt.Errorf("invalid direction")
		}
	}
	return nil
}
