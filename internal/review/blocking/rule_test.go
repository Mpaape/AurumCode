package blocking

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestRuleFollowsDeclaredGate(t *testing.T) {
	warning := types.ReviewIssue{RuleID: "q/w", File: "a.go", Line: 1, Severity: "warning"}
	failure := types.ReviewIssue{RuleID: "s/e", File: "a.go", Line: 2, Severity: "error"}
	issues := []types.ReviewIssue{warning, failure}

	passing := FromGate(true, gate.Result{Active: true})
	if passing.Blocks(warning) || passing.Count(issues) != 0 || passing.Fails() {
		t.Error("a passing gate has no blocking finding")
	}
	breach := FromGate(true, gate.Result{Active: true, Fail: true, BlockingFindings: []facts.AuditFinding{
		{RuleID: "s/e", Path: "a.go", Line: 2}, {RuleID: "s/e", Path: "a.go", Line: 2},
	}})
	if !breach.Blocks(failure) || breach.Blocks(warning) || breach.Count(issues) != 1 {
		t.Error("only the gate's own blocking finding blocks, counted once")
	}
	ungated := FromGate(false, gate.Result{Fail: true})
	if ungated.Gated() || ungated.Count(issues) != 2 || !ungated.Blocks(warning) {
		t.Error("without a gate every error and warning blocks")
	}
	for _, tc := range []struct {
		rule        Rule
		event, want string
	}{
		{ungated, EventRequestChanges, EventRequestChanges},
		{passing, EventRequestChanges, EventComment},
		{passing, "APPROVE", "APPROVE"},
		{breach, EventComment, EventRequestChanges},
	} {
		if got := tc.rule.Event(tc.event); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.event, got, tc.want)
		}
	}
}
