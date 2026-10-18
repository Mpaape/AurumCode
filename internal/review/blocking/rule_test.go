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

// An explicit --fail-on catches on top of a passing gate: the issue it
// catches blocks, the run fails and the formal event requests changes, so
// the parecer agrees with the --fail-on exit.
func TestThresholdCatchesOnTopOfAPassingGate(t *testing.T) {
	observation := types.ReviewIssue{RuleID: "q/e", File: "a.go", Line: 3, Severity: "error"}
	note := types.ReviewIssue{RuleID: "q/i", File: "a.go", Line: 4, Severity: "info"}
	catches := func(issue types.ReviewIssue) bool { return issue.Severity == "error" }

	passing := FromGate(true, gate.Result{Active: true}).WithThreshold(catches, true)
	if !passing.Blocks(observation) || passing.Blocks(note) {
		t.Error("the threshold's catch blocks; what it misses stays an observation")
	}
	if passing.GateFails() {
		t.Error("the policy gate itself still passed")
	}
	if !passing.Fails() || passing.Count([]types.ReviewIssue{observation, note}) != 1 {
		t.Error("a passing gate under a threshold that caught one fails with one blocking finding")
	}
	if got := passing.Event("APPROVE"); got != EventRequestChanges {
		t.Errorf("event: got %s, want %s", got, EventRequestChanges)
	}
	if FromGate(true, gate.Result{Active: true}).WithThreshold(nil, false).Fails() {
		t.Error("no threshold leaves the gate's reading")
	}
	ungated := Ungated().WithThreshold(func(types.ReviewIssue) bool { return true }, true)
	if ungated.Count([]types.ReviewIssue{observation, note}) != 2 {
		t.Error("ungated: the threshold adds the info it catches to the errors and warnings")
	}
}
