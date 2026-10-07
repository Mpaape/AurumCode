package cistatus

import (
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Bases of a CI analysis item: what it rests on.
const (
	// BasisCI is an item about a concluded check of the CI context: its
	// state and link are observed, never the model's.
	BasisCI = "ci"
	// BasisModel is an item that matches no concluded check: only the
	// model's text supports it, so its status is never published as a
	// state of CI.
	BasisModel = "model"
)

// passingStates are the concluded states that need no diagnosis.
var passingStates = map[string]bool{"SUCCESS": true, "SKIPPED": true, "NEUTRAL": true}

// Passed reports a concluded state that needs no diagnosis.
func Passed(state string) bool { return passingStates[normalizeState(state)] }

// Verify annotates each item with what this execution observed. An item
// whose check is a concluded check of the context gets BasisCI, the
// observed state (the worst one when the name repeats) and link, and
// Grounded when its evidence quotes the check's log excerpt. Any other item
// gets BasisModel and no observed state: the model's status is never copied
// into Observed. The input slice is not modified.
func (c Context) Verify(items []types.CIAnalysis) []types.CIAnalysis {
	out := make([]types.CIAnalysis, len(items))
	for i, item := range items {
		item.Basis, item.Observed, item.Link, item.Grounded = BasisModel, "", "", false
		if check, ok := c.observed(item.Check); ok {
			item.Basis = BasisCI
			item.Observed = strings.ToLower(normalizeState(check.State))
			item.Link = strings.TrimSpace(check.Link)
			item.Grounded = quotes(check.Excerpt, item.Evidence)
		}
		out[i] = item
	}
	return out
}

// observed returns the concluded check of the context named name. When the
// name repeats, a failing entry wins over a passing one, so a failure is
// never reported as passed.
func (c Context) observed(name string) (Check, bool) {
	key := normalizeName(name)
	var found Check
	ok := false
	for _, check := range c.concluded {
		if normalizeName(check.Name) != key {
			continue
		}
		if !ok || (Passed(found.State) && !Passed(check.State)) {
			found, ok = check, true
		}
	}
	return found, ok
}

// quotes reports that evidence, whitespace-folded, appears in excerpt: the
// model cited what the log shows instead of describing it.
func quotes(excerpt, evidence string) bool {
	ev := strings.Join(strings.Fields(evidence), " ")
	if ev == "" {
		return false
	}
	return strings.Contains(strings.Join(strings.Fields(excerpt), " "), ev)
}
