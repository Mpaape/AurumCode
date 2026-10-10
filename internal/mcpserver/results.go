package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
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

// nextStep tells the agent what the decision asks of it, in language.
func nextStep(language string, d Decision, reason string) string {
	if reason == reasonEmptyChange {
		return i18n.Text(language, "mcp.next.empty_change")
	}
	switch d {
	case DecisionPass:
		return i18n.Text(language, "mcp.next.pass")
	case DecisionFail:
		return i18n.Text(language, "mcp.next.fail")
	}
	return withHints(i18n.Text(language, "mcp.next.inconclusive"), inconclusiveHints(language, reason))
}

// providerReasons are the inconclusive reasons a model provider fixes.
var providerReasons = map[string]bool{"provider_failure": true, "quality_skipped": true, "not_reviewed": true}

// scannerMissingReasons are the reasons an installed scanner fixes.
var scannerMissingReasons = map[string]bool{"sast_unavailable": true, "secrets_unavailable": true}

// inconclusiveHints names what to configure for the reasons of an
// inconclusive answer: the provider's variables, or the missing scanner.
// The English answer keeps the bytes of earlier releases, so it gets none.
func inconclusiveHints(language, reason string) []string {
	if i18n.LocaleOf(language) == i18n.English {
		return nil
	}
	var provider, scanner bool
	for _, code := range strings.Split(reason, ",") {
		code = strings.TrimSpace(code)
		provider = provider || providerReasons[code]
		scanner = scanner || scannerMissingReasons[code]
	}
	var hints []string
	if provider {
		hints = append(hints, i18n.Text(language, "mcp.next.configure_provider"))
	}
	if scanner {
		hints = append(hints, i18n.Text(language, "mcp.next.install_scanner"))
	}
	return hints
}

// withHints appends each non-empty hint to text as its own sentence.
func withHints(text string, hints []string) string {
	for _, h := range hints {
		if h != "" {
			text += " " + h
		}
	}
	return text
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
		Next:      nextStep(o.Language, decision, reason),
	}
}

// failedAnswer is the answer when the session could not run at all: always
// inconclusive, never a pass. No session read the configuration, so its
// next step is in the default language.
func failedAnswer(reason string) gateAnswer {
	return gateAnswer{
		Decision:  DecisionInconclusive,
		Reason:    reason,
		ExitCode:  -1,
		Blocking:  []Finding{},
		Findings:  []Finding{},
		GateLines: []string{},
		Next:      nextStep("", DecisionInconclusive, reason),
	}
}
