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
