// Package blocking decides what a published review calls blocking. With a
// declared policy gate, blocking means exactly what the gate fails on: the
// review's verdict and its text follow the gate decision, and a finding
// below the threshold is a non-blocking observation. Without a gate the
// historical reading stays: every error or warning blocks.
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

// Fails reports whether the declared gate failed this run.
func (r Rule) Fails() bool { return r.declared && r.fails }

// Blocks reports whether issue blocks the merge under this rule.
func (r Rule) Blocks(issue types.ReviewIssue) bool {
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
		return len(r.keys)
	}
	n := 0
	for _, issue := range issues {
		if severityBlocks(issue.Severity) {
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
	if r.fails {
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
