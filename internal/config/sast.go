// `quality_gates.sast` is the alias of a SAST engine's entry in
// quality_gates.scanners (scanners.go): its engine must be a registered
// engine of the "sast" category (semgrep), and AsScanner reads it as that
// entry. QualityGatesConfig (the shared `quality_gates:` top-level shape)
// lives in qualitygates.go.
package config

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// sastCategory is the scanner category quality_gates.sast is an alias of.
const sastCategory = "sast"

// DefaultSASTFailOnSeverity is quality_gates.sast's own default threshold
// when fail_on_severity is absent.
const DefaultSASTFailOnSeverity = "ERROR"

// SastConfig is quality_gates.sast: a deterministic, multi-language
// static-analysis pass whose findings are deterministic evidence -- the
// model may explain one but never remove or downgrade it (AC-005).
type SastConfig struct {
	Engine         string   `yaml:"engine"`
	Enabled        bool     `yaml:"enabled"`
	FailOnSeverity string   `yaml:"fail_on_severity"`
	RulePacks      []string `yaml:"rule_packs"`
}

// IsEnabled reports the explicit enabled flag, nil-safe: an entirely
// absent quality_gates.sast section (s == nil) is exactly as "off" as one
// written with "enabled: false" -- neither ever starts running Semgrep on
// its own (AC-004: nothing changes without an explicit "enabled: true").
func (s *SastConfig) IsEnabled() bool {
	return s != nil && s.Enabled
}

// EngineName returns the configured engine, defaulting to "semgrep" when
// absent or when s is nil.
func (s *SastConfig) EngineName() string {
	if s == nil || strings.TrimSpace(s.Engine) == "" {
		return "semgrep"
	}
	return strings.ToLower(strings.TrimSpace(s.Engine))
}

// Threshold normalizes fail_on_severity (defaulting to
// DefaultSASTFailOnSeverity when absent or when s is nil) into the same
// GateSeverityRank ladder gate.fail_on already uses, so quality_gates.sast
// speaks the exact same severity vocabulary as AUR-519's gate.
func (s *SastConfig) Threshold() (rank GateSeverityRank, canonical string, err error) {
	level := DefaultSASTFailOnSeverity
	if s != nil && strings.TrimSpace(s.FailOnSeverity) != "" {
		level = s.FailOnSeverity
	}
	return NormalizeGateSeverity(level)
}

// Validate normalizes and validates every field this section declared
// (a nil s, meaning the section is entirely absent, is always valid), so
// a malformed quality_gates.sast is a loud parse-time error exactly like
// GateConfig.Validate.
func (s *SastConfig) Validate() error {
	if s == nil {
		return nil
	}
	accepted := scanner.NamesIn(sastCategory)
	if e, ok := scanner.Lookup(s.EngineName()); !ok || scanner.Normalize(e.Category) != sastCategory {
		return fmt.Errorf("quality_gates.sast.engine: unsupported engine %q (accepted: %s)", s.Engine, strings.Join(accepted, ", "))
	}
	if _, _, err := s.Threshold(); err != nil {
		return fmt.Errorf("quality_gates.sast.fail_on_severity: %w", err)
	}
	// A pack beginning with "-" reads as a CLI flag, not a rule pack name
	// or path, if it were ever interpolated without its own "--config"
	// prefix elsewhere -- refused here, at parse time, rather than left
	// for the Semgrep invocation to misinterpret at run time.
	for _, p := range s.RulePacks {
		if strings.HasPrefix(strings.TrimSpace(p), "-") {
			return fmt.Errorf("quality_gates.sast.rule_packs: %q looks like a flag, not a rule pack", p)
		}
	}
	return nil
}
