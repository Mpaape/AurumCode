package verify

import "github.com/Mpaape/AurumCode/pkg/types"

// Verdicts the verifier may answer.
const (
	VerdictConfirmed = "confirmed"
	VerdictRefuted   = "refuted"
	VerdictUncertain = "uncertain"
)

// Outcome says what the verification did to one finding.
type Outcome string

// The outcomes. Only OutcomeRefuted demotes; every other keeps the finding
// blocking and names why.
const (
	OutcomeRefuted           Outcome = "refuted"
	OutcomeConfirmed         Outcome = "confirmed"
	OutcomeUncertain         Outcome = "uncertain"
	OutcomeQuoteNotFound     Outcome = "quote_not_found"
	OutcomeInvalidAnswer     Outcome = "invalid_answer"
	OutcomeProviderError     Outcome = "provider_error"
	OutcomeCallLimit         Outcome = "call_limit"
	OutcomeSourceUnavailable Outcome = "source_unavailable"
)

// Record is one verified finding as the audit keeps it.
type Record struct {
	RuleID  string  `json:"rule_id"`
	Path    string  `json:"path"`
	Line    int     `json:"line"`
	Outcome Outcome `json:"outcome"`
	Demoted bool    `json:"demoted"`
	// Blocking says whether the finding would have blocked the run: a
	// refuted blocking finding leaves the gate, a refuted observation is
	// dropped from the parecer.
	Blocking bool `json:"blocking"`
	// Verdict is the verifier's own answer; empty when there was none.
	Verdict string `json:"verdict,omitempty"`
	// Reason is the verifier's reason, or why no answer could be used.
	Reason string `json:"reason,omitempty"`
	Quote  string `json:"quote,omitempty"`
}

// Demotion is a finding the verification refuted: no longer counted, kept
// visible with the record that refuted it.
type Demotion struct {
	Issue  types.ReviewIssue
	Record Record
}

// Result is the verification of one review's findings.
type Result struct {
	// Kept is every finding that still counts, in input order.
	Kept []types.ReviewIssue
	// Demoted is every refuted finding, in input order.
	Demoted []Demotion
	// Records is every finding sent (or due) to the verifier.
	Records []Record
	// Calls is how many verification calls were made.
	Calls int
}
