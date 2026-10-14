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
	// Awaiting is set when the model was to weigh the evidence and did not
	// answer (provider failure, refusal, unparsable reply, a triage call
	// that failed): a finding of a BySource source the model never assessed
	// is then not a static error that fails the gate, but evidence
	// awaiting the model's decision (Run.Awaiting); the run is
	// inconclusive, never approved.
	Awaiting bool
	// Assessed holds the keys (DisputeKey) of the findings the model did
	// assess; under Awaiting only the others wait.
	Assessed map[string]bool
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
	if !t.BySource[source] || (len(t.Disputed) == 0 && !t.Awaiting) {
		return issues
	}
	kept := make([]types.ReviewIssue, 0, len(issues))
	for _, issue := range issues {
		key := DisputeKey(origin, issue.RuleID, issue.File, issue.Line)
		switch {
		case t.Disputed[key]:
			run.Demoted = append(run.Demoted, Demotion{Source: source, Issue: issue})
		case t.Awaiting && !t.Assessed[key]:
			run.Awaiting = append(run.Awaiting, Demotion{Source: source, Issue: issue})
		default:
			kept = append(kept, issue)
		}
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

// keepWeighed is keep for a source whose findings apply folds into the
// gate. The findings left awaiting the model are weighed by the same apply
// on a scratch result: only those that would have blocked stay in
// Run.Awaiting, so a finding below the threshold never makes the run wait.
func (run *Run) keepWeighed(source, origin string, issues []types.ReviewIssue, apply func(*Result, []types.ReviewIssue) error) []types.ReviewIssue {
	before := len(run.Awaiting)
	kept := run.keep(source, origin, issues)
	waiting := append([]Demotion(nil), run.Awaiting[before:]...)
	if len(waiting) == 0 {
		return kept
	}
	candidates := make([]types.ReviewIssue, 0, len(waiting))
	for _, d := range waiting {
		candidates = append(candidates, d.Issue)
	}
	var scratch Result
	if err := apply(&scratch, candidates); err != nil {
		// The real apply reports the same error; every candidate waits.
		return kept
	}
	blocks := make(map[string]bool, len(scratch.BlockingFindings))
	for _, f := range scratch.BlockingFindings {
		blocks[FindingOriginKey(f.RuleID, f.Path, f.Line)] = true
	}
	run.Awaiting = run.Awaiting[:before]
	for _, d := range waiting {
		if blocks[FindingOriginKey(d.Issue.RuleID, d.Issue.File, d.Issue.Line)] {
			run.Awaiting = append(run.Awaiting, d)
		}
	}
	return kept
}
