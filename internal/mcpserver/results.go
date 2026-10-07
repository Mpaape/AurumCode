package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// originModel names a finding no deterministic analyzer produced.
const originModel = "model"

// gateAnswer is the structured answer of aurum_gate.
type gateAnswer struct {
	Decision  Decision  `json:"decision"`
	Reason    string    `json:"reason,omitempty"`
	ExitCode  int       `json:"exit_code"`
	Blocking  []Finding `json:"blocking_findings"`
	Findings  []Finding `json:"findings"`
	GateLines []string  `json:"gate_lines"`
	Next      string    `json:"next_step"`
}

// reviewAnswer is the structured answer of aurum_review: the gate answer
// plus the session's report.
type reviewAnswer struct {
	gateAnswer
	Report      string `json:"report"`
	Diagnostics string `json:"diagnostics,omitempty"`
}

// nextStep tells the agent what the decision asks of it.
func nextStep(d Decision, reason string) string {
	if reason == reasonEmptyChange {
		return "Nothing changed between base and HEAD, so nothing was reviewed. Commit your change on the working branch first (the review covers commits), or check base, then call aurum_gate again."
	}
	switch d {
	case DecisionPass:
		return "The gate passes. Commit or push."
	case DecisionFail:
		return "Fix every blocking finding (see suggestion, or call aurum_explain with its id), then call aurum_gate again. Never disable or weaken a rule to pass."
	}
	return "The review did not conclude, so this is not a pass. Read reason and fix the cause (configure the model provider, retry), then call aurum_gate again."
}

// identify gives every finding a stable id derived from where it is and
// which rule it cites (never from its text, which may carry a secret),
// sorted by file and line so the same session always answers in the same
// order.
func identify(findings []Finding) []Finding {
	out := make([]Finding, len(findings))
	copy(out, findings)
	for i := range out {
		if out[i].Origin == "" {
			out[i].Origin = originModel
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%s", out[i].File, out[i].Line, out[i].Severity, out[i].RuleID, out[i].Origin)))
		out[i].ID = hex.EncodeToString(sum[:])[:12]
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].RuleID < out[j].RuleID
	})
	if out == nil {
		out = []Finding{}
	}
	return out
}

// answerFor builds the gate answer of a session outcome.
func answerFor(o SessionOutcome) gateAnswer {
	decision, reason := decide(o)
	lines := o.GateLines
	if lines == nil {
		lines = []string{}
	}
	return gateAnswer{
		Decision:  decision,
		Reason:    reason,
		ExitCode:  o.Exit,
		Blocking:  identify(o.Blocking),
		Findings:  identify(o.Findings),
		GateLines: lines,
		Next:      nextStep(decision, reason),
	}
}

// failedAnswer is the answer when the session could not run at all: always
// inconclusive, never a pass.
func failedAnswer(reason string) gateAnswer {
	return gateAnswer{
		Decision:  DecisionInconclusive,
		Reason:    reason,
		ExitCode:  -1,
		Blocking:  []Finding{},
		Findings:  []Finding{},
		GateLines: []string{},
		Next:      nextStep(DecisionInconclusive, reason),
	}
}
