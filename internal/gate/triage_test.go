package gate

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A dispute is matched by origin as well as rule, path and line: disputing
// the security finding never demotes an analysis finding at the same place.
func TestTriageMatchesTheDisputeByOrigin(t *testing.T) {
	issue := types.ReviewIssue{RuleID: "r", File: "app.go", Line: 6}
	run := &Run{Triage: Triage{
		Disputed: map[string]bool{DisputeKey(OriginSecurity, "r", "app.go", 6): true},
		BySource: map[string]bool{config.GateSourceAnalysis: true},
	}}
	if kept := run.keep(config.GateSourceAnalysis, OriginAnalysis, []types.ReviewIssue{issue}); len(kept) != 1 {
		t.Fatal("a dispute of another origin demoted the finding")
	}
	if kept := run.keep(config.GateSourceAnalysis, OriginSecurity, []types.ReviewIssue{issue}); len(kept) != 0 || len(run.Demoted) != 1 {
		t.Fatal("the disputed security finding must be demoted under analysis: model")
	}
}

// A model that did not answer leaves the unassessed findings of a triaged
// source awaiting its decision, never counting as a static error; what the
// model did assess keeps its effect, and a source without triage counts.
func TestTriageAwaitingKeepsUnassessedOutOfTheCount(t *testing.T) {
	waiting := types.ReviewIssue{RuleID: "gitleaks:generic-api-key", File: "f.json", Line: 17}
	confirmed := types.ReviewIssue{RuleID: "gitleaks:generic-api-key", File: "f.json", Line: 50}
	run := &Run{Triage: Triage{
		Awaiting: true,
		Assessed: map[string]bool{DisputeKey("gitleaks", confirmed.RuleID, confirmed.File, confirmed.Line): true},
		BySource: map[string]bool{"secrets": true},
	}}
	kept := run.keep("secrets", "gitleaks", []types.ReviewIssue{waiting, confirmed})
	if len(kept) != 1 || kept[0].Line != 50 || len(run.Awaiting) != 1 || run.Awaiting[0].Issue.Line != 17 {
		t.Fatalf("kept %v, awaiting %v", kept, run.Awaiting)
	}
	if kept := run.keep("sast", "semgrep", []types.ReviewIssue{waiting}); len(kept) != 1 {
		t.Fatal("a source the model may not triage keeps counting")
	}
}
