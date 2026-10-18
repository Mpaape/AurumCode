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

	passing := FromGate(true, gate.Result{Active: true}).WithThreshold(catches, true, nil)
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
	if FromGate(true, gate.Result{Active: true}).WithThreshold(nil, false, nil).Fails() {
		t.Error("no threshold leaves the gate's reading")
	}
	ungated := Ungated().WithThreshold(func(types.ReviewIssue) bool { return true }, true, nil)
	if ungated.Count([]types.ReviewIssue{observation, note}) != 2 {
		t.Error("ungated: the threshold adds the info it catches to the errors and warnings")
	}
	if !ungated.Fails() || ungated.Event(EventComment) != EventRequestChanges {
		t.Error("ungated: a threshold that caught a finding fails the run and requests changes")
	}
	if Ungated().WithThreshold(catches, false, nil).Event(EventComment) != EventComment {
		t.Error("ungated: a threshold that caught nothing leaves the event")
	}
}

// A caught finding published in a separate section (the security pass
// under --base) counts once, beside the issues; a gate key is never counted
// twice, and only a gate key is a GateBlocks finding.
func TestThresholdCountsSeparateFindings(t *testing.T) {
	security := types.ReviewIssue{RuleID: "security/hardcoded-secret", File: "a.go", Line: 4, Severity: "error"}
	keyed := types.ReviewIssue{RuleID: "policy/x", File: "a.go", Line: 5, Severity: "error"}
	catches := func(issue types.ReviewIssue) bool { return issue.Severity == "error" }
	gated := FromGate(true, gate.Result{Active: true, BlockingFindings: []facts.AuditFinding{{RuleID: keyed.RuleID, Path: keyed.File, Line: keyed.Line}}}).
		WithThreshold(catches, true, []types.ReviewIssue{security, security, keyed})
	if got := gated.Count(nil); got != 2 {
		t.Errorf("count = %d, want 2 (the gate key and the security finding, once each)", got)
	}
	if got := gated.Count([]types.ReviewIssue{security}); got != 2 {
		t.Errorf("count = %d, want 2 (a separate finding also among the issues counts once)", got)
	}
	if !gated.GateBlocks(keyed) || gated.GateBlocks(security) || gated.GateCount() != 1 {
		t.Error("only the gate's own key is a gate blocking finding")
	}
	if got := Ungated().WithThreshold(catches, true, []types.ReviewIssue{security}).Count(nil); got != 1 {
		t.Errorf("ungated count = %d, want 1", got)
	}
}
