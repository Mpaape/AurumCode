// Package blocking decides what a published review calls blocking. With a
// declared policy gate, blocking means exactly what the gate fails on: the
// review's verdict and its text follow the gate decision, and a finding
// below the threshold is a non-blocking observation. Without a gate the
// historical reading stays: every error or warning blocks. An explicit
// --fail-on adds its own catch on top of either reading (WithThreshold), so
// the parecer never reads "passed" on a run whose exit is that failure.
package blocking

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// GitHub's formal review events the rule aligns.
const (
	EventRequestChanges = "REQUEST_CHANGES"
	EventComment        = "COMMENT"
)

// Rule is the blocking reading of one review run.
type Rule struct {
	declared bool
	fails    bool
	keys     map[string]bool
	// caught is the --fail-on threshold's own reading (nil without one), and
	// thresholdFails whether it caught a finding this run.
	caught         func(types.ReviewIssue) bool
	thresholdFails bool
	// separate are the run's findings published in a section of their own
	// (the security pass under --base), never in the issues Count reads:
	// the ones the threshold caught still count.
	separate []types.ReviewIssue
}

// WithThreshold adds an explicit --fail-on threshold to r: an issue caught
// blocks, and a run where the threshold caught one fails and requests
// changes, whatever the declared gate decided and with no gate at all.
// separate are the findings published apart from the issues Count reads
// (the security pass's under --base): the caught ones count too. caught
// nil leaves r unchanged.
func (r Rule) WithThreshold(caught func(types.ReviewIssue) bool, failed bool, separate []types.ReviewIssue) Rule {
	if caught == nil {
		return r
	}
	r.caught, r.thresholdFails, r.separate = caught, failed, separate
	return r
}

// CaughtByThreshold reports whether the explicit --fail-on threshold
// caught issue (always false without one).
func (r Rule) CaughtByThreshold(issue types.ReviewIssue) bool {
	return r.caught != nil && r.caught(issue)
}

// Ungated is the rule of a run without a declared gate.
func Ungated() Rule { return Rule{} }

// FromGate is the rule of a run whose gate was declared (or not) and
// decided res. An undeclared gate yields Ungated.
func FromGate(declared bool, res gate.Result) Rule {
	if !declared {
		return Ungated()
	}
	keys := make(map[string]bool, len(res.BlockingFindings))
	for _, f := range res.BlockingFindings {
		keys[gate.FindingOriginKey(f.RuleID, f.Path, f.Line)] = true
	}
	return Rule{declared: true, fails: res.Fail, keys: keys}
}

// Gated reports whether a declared gate decides what blocks.
func (r Rule) Gated() bool { return r.declared }

// GateFails reports whether the declared policy gate itself failed this run,
// whatever the --fail-on threshold caught: the "policy gate" line of the
// parecer and the aurumcode/policy-gate status say this.
func (r Rule) GateFails() bool { return r.declared && r.fails }

// Fails reports whether the declared gate, or the --fail-on threshold on
// top of it or without a gate, failed this run.
func (r Rule) Fails() bool { return r.GateFails() || r.thresholdFails }

// GateBlocks reports whether issue is one of the declared gate's own
// blocking findings, the ones Count counts whether or not they are
// published.
func (r Rule) GateBlocks(issue types.ReviewIssue) bool {
	return r.declared && r.keys[issueKey(issue)]
}

// GateCount is how many distinct blocking findings the declared gate named
// (0 without a gate).
func (r Rule) GateCount() int { return len(r.keys) }

// Blocks reports whether issue blocks the merge under this rule.
func (r Rule) Blocks(issue types.ReviewIssue) bool {
	if r.CaughtByThreshold(issue) {
		return true
	}
	if r.declared {
		return r.keys[issueKey(issue)]
	}
	return severityBlocks(issue.Severity)
}

func issueKey(issue types.ReviewIssue) string {
	return gate.FindingOriginKey(issue.RuleID, issue.File, issue.Line)
}

// Count is how many blocking findings the run has: with a gate, the gate's
// own distinct blocking findings (a finding without a review issue, such as
// a dependency breach, included); without one, the errors and warnings.
// The findings the --fail-on threshold caught are added once, the ones
// published in a separate section included.
func (r Rule) Count(issues []types.ReviewIssue) int {
	n := len(r.keys)
	counted := map[string]bool{}
	for _, issue := range issues {
		if r.declared && r.keys[issueKey(issue)] {
			continue
		}
		if (!r.declared && severityBlocks(issue.Severity)) || r.CaughtByThreshold(issue) {
			n++
			counted[issueKey(issue)] = true
		}
	}
	for _, issue := range r.separate {
		key := issueKey(issue)
		if r.CaughtByThreshold(issue) && !r.keys[key] && !counted[key] {
			n++
			counted[key] = true
		}
	}
	return n
}

// Event aligns a formal review event with the gate: a failing gate requests
// changes, a passing one never does (its findings are below the threshold,
// so the review is a comment). Without a gate the event is unchanged,
// unless the --fail-on threshold caught a finding: that requests changes.
func (r Rule) Event(event string) string {
	if r.Fails() {
		return EventRequestChanges
	}
	if !r.declared {
		return event
	}
	if event == EventRequestChanges {
		return EventComment
	}
	return event
}

// severityBlocks is the historical, gate-free reading of a severity.
func severityBlocks(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error", "warning":
		return true
	}
	return false
}
