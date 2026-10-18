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
}

// WithThreshold adds an explicit --fail-on threshold to r: an issue caught
// blocks, and a run where the threshold caught one fails, whatever the
// declared gate decided. caught nil leaves r unchanged.
func (r Rule) WithThreshold(caught func(types.ReviewIssue) bool, failed bool) Rule {
	if caught == nil {
		return r
	}
	r.caught, r.thresholdFails = caught, failed
	return r
}

func (r Rule) caughtByThreshold(issue types.ReviewIssue) bool {
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
// top of it, failed this run.
func (r Rule) Fails() bool { return r.declared && (r.fails || r.thresholdFails) }

// Blocks reports whether issue blocks the merge under this rule.
func (r Rule) Blocks(issue types.ReviewIssue) bool {
	if r.caughtByThreshold(issue) {
		return true
	}
	if r.declared {
		return r.keys[gate.FindingOriginKey(issue.RuleID, issue.File, issue.Line)]
	}
	return severityBlocks(issue.Severity)
}

// Count is how many blocking findings the run has: with a gate, the gate's
// own distinct blocking findings (a finding without a review issue, such as
// a dependency breach, included); without one, the errors and warnings.
func (r Rule) Count(issues []types.ReviewIssue) int {
	if r.declared {
		n := len(r.keys)
		for _, issue := range issues {
			if r.caughtByThreshold(issue) && !r.keys[gate.FindingOriginKey(issue.RuleID, issue.File, issue.Line)] {
				n++
			}
		}
		return n
	}
	n := 0
	for _, issue := range issues {
		if severityBlocks(issue.Severity) || r.caughtByThreshold(issue) {
			n++
		}
	}
	return n
}

// Event aligns a formal review event with the gate: a failing gate requests
// changes, a passing one never does (its findings are below the threshold,
// so the review is a comment). Without a gate the event is unchanged.
func (r Rule) Event(event string) string {
	if !r.declared {
		return event
	}
	if r.Fails() {
		return EventRequestChanges
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
