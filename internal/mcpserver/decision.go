package mcpserver

import "fmt"

// Decision is the gate's answer to an agent.
type Decision string

const (
	// DecisionPass: the session concluded and the gate let the change
	// through.
	DecisionPass Decision = "pass"
	// DecisionFail: the gate blocked the change; the findings say why.
	DecisionFail Decision = "fail"
	// DecisionInconclusive: the review did not conclude (no provider, a
	// failed source, a timeout). It never counts as a pass.
	DecisionInconclusive Decision = "inconclusive"
)

// Exit codes of a review session that carry a gate decision.
const (
	exitClean    = 0
	exitBlocked  = 1
	exitFindings = 3
)

// reasonEmptyChange names a session that had nothing to review: base and
// HEAD carry the same tree. An agent asking before it committed gets this,
// never a pass.
const reasonEmptyChange = "empty_change"

// reasonNoConclusion names a session that ended without a gate decision
// and without a declared motive.
const reasonNoConclusion = "session_exit_%d"

// decide reads the session's own decision; it never recomputes the gate.
// Only a clean exit of a conclusive run over a non-empty change passes.
func decide(o SessionOutcome) (Decision, string) {
	switch {
	case o.InconclusiveReason != "":
		return DecisionInconclusive, o.InconclusiveReason
	case o.Exit == exitClean && o.ChangedFiles == 0:
		return DecisionInconclusive, reasonEmptyChange
	case o.Exit == exitClean:
		return DecisionPass, ""
	case o.Exit == exitFindings, o.Exit == exitBlocked && len(o.Blocking) > 0:
		return DecisionFail, ""
	}
	return DecisionInconclusive, fmt.Sprintf(reasonNoConclusion, o.Exit)
}
