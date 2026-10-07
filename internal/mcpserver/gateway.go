package mcpserver

import "context"

// Gateway is the port the server asks; the command implements it with the
// review session the CLI and CI run. The server never decides a finding or
// a gate on its own.
type Gateway interface {
	// Review runs one review session over the diff between req.Base and
	// HEAD and reports what it decided.
	Review(ctx context.Context, req ReviewRequest) (SessionOutcome, error)
	// Rules lists the rules and skills that apply to paths under the
	// effective configuration (central policy over the repository).
	Rules(ctx context.Context, paths []string) (RuleSet, error)
}

// ReviewRequest is the only thing a client chooses: which ref to compare
// HEAD against. Policy, skills, severities and limits are the server's.
type ReviewRequest struct {
	Base string
}

// SessionOutcome is what one review session produced.
type SessionOutcome struct {
	// Exit is the session's exit code, the same the CLI would return.
	Exit int
	// ChangedFiles counts the files the diff between base and HEAD
	// touched, before any ignore pattern.
	ChangedFiles int
	// InconclusiveReason is the gate's machine-readable motive of an
	// inconclusive run ("provider_failure", "partial_coverage", ...), or
	// "not_reviewed" when the model review did not happen; empty when the
	// run concluded.
	InconclusiveReason string
	// Findings are every finding that reached the gate.
	Findings []Finding
	// Blocking are the findings the gate matched against its threshold.
	Blocking []Finding
	// GateLines are the gate's own decision lines.
	GateLines []string
	// Report and Diagnostics are the session's terminal report (stdout)
	// and its stderr.
	Report      string
	Diagnostics string
	// Redactor, when set, is the session's final filter (it may know a
	// secret learned mid-run); it is applied on top of the server's.
	Redactor Redactor
}

// Finding is one structured finding: where it is, which rule it cites,
// which source raised it, and what to do about it.
type Finding struct {
	ID         string `json:"id"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`
	RuleID     string `json:"rule_id"`
	Origin     string `json:"origin"`
	Message    string `json:"message"`
	Impact     string `json:"impact,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// RuleSet is the rules and skills that apply to a set of paths.
type RuleSet struct {
	PolicyActive string   `json:"central_policy"`
	Skills       []Skill  `json:"skills"`
	Rules        []Rule   `json:"rules"`
	Notices      []string `json:"notices,omitempty"`
}

// Skill is one skill document selected for the paths and the layer it
// belongs to ("policy" or "repository").
type Skill struct {
	Path  string `json:"path"`
	Layer string `json:"layer"`
}

// Rule is one citable rule.
type Rule struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Origin   string `json:"origin"`
}

// Redactor removes secrets from text.
type Redactor interface {
	Redact(string) string
}
