package gate

import (
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// DisputeKey identifies a disputed finding by its origin as well as its
// rule, path and line, so a dispute of one source's finding can never
// demote another source's finding at the same place.
func DisputeKey(origin, ruleID, path string, line int) string {
	return origin + "|" + FindingOriginKey(ruleID, path, line)
}

// Triage is what the model's assessment of the deterministic evidence may
// change in the gate: nothing without a declared gate or under a central
// policy; otherwise, per source, gate.triage (model unless the repository
// wrote none). Disputed holds the keys (DisputeKey) of the findings the
// model disputed with a justification; BySource the gate.sources names
// whose disputed evidence stops counting. The assembly leaves BySource
// empty whenever a central policy is active or no gate is declared, so
// evidence of policy origin always counts whatever the model concluded.
type Triage struct {
	Disputed map[string]bool
	BySource map[string]bool
}

// Demotion records one finding a dispute removed from the count.
type Demotion struct {
	Source string
	Issue  types.ReviewIssue
}

// keep returns the issues of source (whose findings carry origin) that
// still count, and records on run the ones the model's dispute demoted.
func (run *Run) keep(source, origin string, issues []types.ReviewIssue) []types.ReviewIssue {
	t := run.Triage
	if !t.BySource[source] || len(t.Disputed) == 0 {
		return issues
	}
	kept := make([]types.ReviewIssue, 0, len(issues))
	for _, issue := range issues {
		if t.Disputed[DisputeKey(origin, issue.RuleID, issue.File, issue.Line)] {
			run.Demoted = append(run.Demoted, Demotion{Source: source, Issue: issue})
			continue
		}
		kept = append(kept, issue)
	}
	return kept
}

// keepSkills applies the skills source's triage to the issues citing a
// dynamic rule, leaving every other issue untouched.
func (run *Run) keepSkills(issues []types.ReviewIssue, dynamic func(ruleID string) bool) []types.ReviewIssue {
	var skills, rest []types.ReviewIssue
	for _, issue := range issues {
		if dynamic(issue.RuleID) {
			skills = append(skills, issue)
		} else {
			rest = append(rest, issue)
		}
	}
	return append(rest, run.keep(config.GateSourceSkills, OriginSkills, skills)...)
}
