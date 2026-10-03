package config

import (
	"fmt"
	"strings"
)

// GateConfig is AUR-519's gate: which severities fail the check (FailOn)
// and what an inconclusive review does (Inconclusive). The zero value
// (both fields empty/absent) means no gate at all -- this card adds no new
// behavior over today's review until a config explicitly declares one. See
// ApplyCentralPolicy: under a central policy, only the policy's own
// GateConfig ever applies (AC-005); a repository's own GateConfig is its
// explicit opt-in only when no central policy is in play.
type GateConfig struct {
	FailOn       []string `yaml:"fail_on"`
	Inconclusive string   `yaml:"inconclusive"`
	// Sources (AUR-556) restricts which finding origins count toward the
	// gate: a closed list of GateSourceSkills, GateSourceAnalysis,
	// GateSourceSAST. Empty (absent) means all three.
	Sources []string `yaml:"sources"`
	// Triage lets the model's assessment of a source's evidence demote it,
	// per source: GateSourceSkills/Analysis/SAST -> TriageModel or
	// TriageNone (the default). It is honoured only without a central
	// policy: evidence of policy origin counts whatever the model says.
	Triage map[string]string `yaml:"triage"`
}

// The closed vocabulary of a gate.triage value.
const (
	// TriageNone: the model's assessment never changes what the gate counts.
	TriageNone = "none"
	// TriageModel: evidence the model disputes stops counting for that
	// source (repository configuration only).
	TriageModel = "model"
)

// ValidateTriage rejects a gate.triage key outside gate.sources'
// vocabulary or a value other than model/none.
func (g GateConfig) ValidateTriage() error {
	for source, mode := range g.Triage {
		switch strings.ToLower(strings.TrimSpace(source)) {
		case GateSourceSkills, GateSourceAnalysis, GateSourceSAST:
		default:
			return fmt.Errorf("gate.triage: unknown source %q (accepted: skills, analysis, sast)", source)
		}
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case TriageNone, TriageModel:
		default:
			return fmt.Errorf("gate.triage.%s: unknown value %q (accepted: model, none)", source, mode)
		}
	}
	return nil
}

// TriageByModel reports whether gate.triage lets the model's dispute demote
// evidence of the named source.
func (g GateConfig) TriageByModel(source string) bool {
	for s, mode := range g.Triage {
		if strings.EqualFold(strings.TrimSpace(s), source) && strings.EqualFold(strings.TrimSpace(mode), TriageModel) {
			return true
		}
	}
	return false
}

// The closed vocabulary of gate.sources (AUR-556).
const (
	GateSourceSkills   = "skills"
	GateSourceAnalysis = "analysis"
	GateSourceSAST     = "sast"
)

// ValidateSources rejects any gate.sources entry outside the closed list.
func (g GateConfig) ValidateSources() error {
	for _, s := range g.Sources {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case GateSourceSkills, GateSourceAnalysis, GateSourceSAST:
		default:
			return fmt.Errorf("gate.sources: unknown source %q (accepted: skills, analysis, sast)", s)
		}
	}
	return nil
}

// SourceEnabled reports whether findings of the named origin count toward
// the gate: always true when no sources list was declared.
func (g GateConfig) SourceEnabled(name string) bool {
	if len(g.Sources) == 0 {
		return true
	}
	for _, s := range g.Sources {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return true
		}
	}
	return false
}

// Declared reports whether this GateConfig was actually written, as
// opposed to being the absent zero value. Either field alone is enough:
// a config can declare only fail_on, or only inconclusive.
func (g GateConfig) Declared() bool {
	return len(g.FailOn) > 0 || strings.TrimSpace(g.Inconclusive) != ""
}

// GateSeverityRank mirrors cmd/aurumcode's own --fail-on ladder (rankInfo <
// rankWarning < rankError, cmd/aurumcode/main.go) so gate.fail_on can be
// compared against a finding's severity with the same meaning, without
// this package importing cmd/aurumcode (a cycle) or duplicating a second,
// divergent severity vocabulary there.
type GateSeverityRank int

const (
	GateSeverityInfo GateSeverityRank = iota + 1
	GateSeverityWarning
	GateSeverityError
)

// NormalizeGateSeverity accepts the same spellings --fail-on already does
// (high/error, medium/warning, low/info), plus "critical" -- the spelling
// the card's own documented example uses -- mapped onto the same top rank
// as high/error: this package never introduces a fourth severity the rest
// of the engine would not also recognize.
func NormalizeGateSeverity(level string) (GateSeverityRank, string, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical", "high", "error":
		return GateSeverityError, "error", nil
	case "medium", "warning":
		return GateSeverityWarning, "warning", nil
	case "low", "info":
		return GateSeverityInfo, "info", nil
	default:
		return 0, "", fmt.Errorf("gate.fail_on: unknown severity %q (accepted: critical|high|error, medium|warning, low|info)", level)
	}
}

// Threshold returns the lowest rank among FailOn's entries -- a finding at
// or above it fails the gate (AC-001), mirroring --fail-on's own "minimum
// severity" reading applied to a list instead of a single level. ok is
// false when FailOn is empty (no severity gate declared).
func (g GateConfig) Threshold() (rank GateSeverityRank, canonical string, ok bool, err error) {
	if len(g.FailOn) == 0 {
		return 0, "", false, nil
	}
	var min GateSeverityRank
	var minName string
	for _, level := range g.FailOn {
		r, name, err := NormalizeGateSeverity(level)
		if err != nil {
			return 0, "", false, err
		}
		if min == 0 || r < min {
			min, minName = r, name
		}
	}
	return min, minName, true, nil
}

// The two values gate.inconclusive resolves to.
const (
	InconclusiveBlock = "block"
	InconclusiveWarn  = "warn"
)

// InconclusiveMode resolves gate.inconclusive for this gate section alone: an
// absent value is block when the gate is declared (fail_on written), so a
// declared gate never approves an inconclusive review by omission; with no
// gate at all it is "". Config.InconclusiveMode adds the scanner context.
func (g GateConfig) InconclusiveMode() (mode string, err error) {
	if g.Declared() {
		return g.resolveInconclusive(InconclusiveBlock)
	}
	return g.resolveInconclusive("")
}

// resolveInconclusive normalizes gate.inconclusive. An absent value resolves
// to fallback: the caller owns the context that decides what silence means.
// "block" and "warn" are the documented values; "bloquear"/"alertar" are
// accepted as the project's own Portuguese aliases.
func (g GateConfig) resolveInconclusive(fallback string) (mode string, err error) {
	switch strings.ToLower(strings.TrimSpace(g.Inconclusive)) {
	case "":
		return fallback, nil
	case "block", "bloquear":
		return InconclusiveBlock, nil
	case "warn", "alertar":
		return InconclusiveWarn, nil
	default:
		return "", fmt.Errorf("gate.inconclusive: unknown value %q (accepted: block, warn)", g.Inconclusive)
	}
}

// Validate normalizes and validates every field this GateConfig declared,
// so a malformed gate section is a loud parse-time error exactly like every
// other optional section of Config (see Parse).
func (g GateConfig) Validate() error {
	if _, _, _, err := g.Threshold(); err != nil {
		return err
	}
	if _, err := g.resolveInconclusive(""); err != nil {
		return err
	}
	if err := g.ValidateSources(); err != nil {
		return err
	}
	return g.ValidateTriage()
}
