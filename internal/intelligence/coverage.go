package intelligence

// EnsureCatalogCoverage supplements a narrow model plan with small independent
// catalog probes. It preserves every model query and the original total budget.
// Source coverage never admits a track: selection and quality gates still apply.
func EnsureCatalogCoverage(in PlanningInput, plan SearchPlan, limits Limits) SearchPlan {
	plan.Steps = append([]SearchStep{}, plan.Steps...)
	if len(plan.Steps) == 0 {
		return plan
	}
	used := map[Channel]int{}
	catalogs := map[string]bool{}
	total := 0
	for _, step := range plan.Steps {
		used[step.Channel] += step.Limit
		total += step.Limit
		if step.Channel.Capability == "catalog_search" && step.Channel.Provider != "cryon" {
			catalogs[step.Channel.Provider] = true
		}
	}
	for _, available := range in.Channels {
		ch := available.Channel
		if len(catalogs) >= 2 || len(plan.Steps) >= limits.MaxSteps {
			break
		}
		if ch.Capability != "catalog_search" || ch.Provider == "cryon" || catalogs[ch.Provider] {
			continue
		}
		count := min(8, available.MaxCandidates-used[ch])
		if count <= 0 {
			continue
		}
		// Reallocate only the necessary slots; never exceed validated request budgets.
		deficit := count - (limits.MaxCandidates - total)
		for i := range plan.Steps {
			if deficit <= 0 {
				break
			}
			take := min(deficit, max(0, plan.Steps[i].Limit-1))
			plan.Steps[i].Limit -= take
			total -= take
			used[plan.Steps[i].Channel] -= take
			deficit -= take
		}
		count = min(count, limits.MaxCandidates-total)
		if count <= 0 {
			continue
		}
		plan.Steps = append(plan.Steps, SearchStep{Channel: ch, Query: plan.Steps[0].Query, Limit: count})
		used[ch] += count
		total += count
		catalogs[ch.Provider] = true
	}
	return plan
}
