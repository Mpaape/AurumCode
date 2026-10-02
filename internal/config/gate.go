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

// InconclusiveMode normalizes gate.inconclusive. "" (absent) means the
// policy did not opt into inconclusive-specific gate behavior at all --
// today's behavior is unchanged. "block" and "warn" are the documented
// values; "bloquear"/"alertar" are accepted as the project's own
// Portuguese aliases, in house style with the rest of this engine's flags.
func (g GateConfig) InconclusiveMode() (mode string, err error) {
	switch strings.ToLower(strings.TrimSpace(g.Inconclusive)) {
	case "":
		return "", nil
	case "block", "bloquear":
		return "block", nil
	case "warn", "alertar":
		return "warn", nil
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
	if _, err := g.InconclusiveMode(); err != nil {
		return err
	}
	return g.ValidateSources()
}
