package config

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
	// The closed list of engines compiled into the binary: Parse validates
	// every `engine` against it.
	_ "github.com/Mpaape/AurumCode/internal/scanner/engines"
)

// DefaultScannerFailOn is a scanner's own threshold when fail_on is absent.
const DefaultScannerFailOn = "ERROR"

// ScannerConfig is one entry of quality_gates.scanners: a registered engine,
// whether it runs (an entry runs unless enabled: false), whether a central
// policy requires it (the repository can then neither remove nor relax it),
// the severity at or above which its findings fail the gate, and the
// engine's own options.
type ScannerConfig struct {
	Engine   string          `yaml:"engine"`
	Enabled  *bool           `yaml:"enabled"`
	Required bool            `yaml:"required"`
	FailOn   string          `yaml:"fail_on"`
	Options  scanner.Options `yaml:"options"`

	// label names the entry in messages: its YAML key.
	label string
}

// Name is the entry's normalized engine name.
func (s ScannerConfig) Name() string { return scanner.Normalize(s.Engine) }

// IsEnabled reports whether the entry runs.
func (s ScannerConfig) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// Label is the entry's YAML key, for messages.
func (s ScannerConfig) Label() string {
	if s.label != "" {
		return s.label
	}
	return fmt.Sprintf("quality_gates.scanners[%s]", s.Name())
}

// Threshold normalizes fail_on onto the gate's severity ladder.
func (s ScannerConfig) Threshold() (rank GateSeverityRank, canonical string, err error) {
	level := DefaultScannerFailOn
	if strings.TrimSpace(s.FailOn) != "" {
		level = s.FailOn
	}
	return NormalizeGateSeverity(level)
}

// Lookup returns the entry's registered engine.
func (s ScannerConfig) Lookup() (scanner.Engine, bool) { return scanner.Lookup(s.Name()) }

// validate checks the engine, the threshold and the engine's options.
func (s ScannerConfig) validate() error {
	e, ok := s.Lookup()
	if !ok {
		return fmt.Errorf("%s.engine: unknown engine %q (registered: %s)", s.Label(), s.Engine, strings.Join(scanner.Names(), ", "))
	}
	if _, _, err := s.Threshold(); err != nil {
		return fmt.Errorf("%s.fail_on: %w", s.Label(), err)
	}
	if err := e.ValidateOptions(s.Options); err != nil {
		return fmt.Errorf("%s.options: %w", s.Label(), err)
	}
	return nil
}

// AsScanner is quality_gates.sast read as the scanner entry it is an alias
// of: its engine, enabled flag, fail_on_severity and rule_packs. A policy's
// quality_gates.sast was always decided by the policy alone, so the alias
// is required. nil when the section is absent.
func (s *SastConfig) AsScanner() *ScannerConfig {
	if s == nil {
		return nil
	}
	enabled := s.Enabled
	entry := &ScannerConfig{Engine: s.EngineName(), Enabled: &enabled, Required: true, FailOn: s.FailOnSeverity, label: "quality_gates.sast"}
	if len(s.RulePacks) > 0 {
		packs := make([]any, 0, len(s.RulePacks))
		for _, p := range s.RulePacks {
			packs = append(packs, p)
		}
		entry.Options = scanner.Options{sastRulePacksOption: packs}
	}
	return entry
}

// sastRulePacksOption is the engine option quality_gates.sast.rule_packs
// maps to.
const sastRulePacksOption = "rule_packs"

// AllScanners is every declared scanner entry, the quality_gates.sast alias
// first, enabled or not.
func (q QualityGatesConfig) AllScanners() []ScannerConfig {
	var out []ScannerConfig
	if alias := q.Sast.AsScanner(); alias != nil {
		out = append(out, *alias)
	}
	return append(out, q.Scanners...)
}

// EnabledScanners is the entries that run, in declaration order.
func (q QualityGatesConfig) EnabledScanners() []ScannerConfig {
	var out []ScannerConfig
	for _, s := range q.AllScanners() {
		if s.IsEnabled() {
			out = append(out, s)
		}
	}
	return out
}

// Scanner returns the declared entry for engine.
func (q QualityGatesConfig) Scanner(engine string) (ScannerConfig, bool) {
	for _, s := range q.AllScanners() {
		if s.Name() == scanner.Normalize(engine) {
			return s, true
		}
	}
	return ScannerConfig{}, false
}

// FromPolicy reports whether the effective entry for engine is the central
// policy's (set by ApplyCentralPolicy).
func (q QualityGatesConfig) FromPolicy(engine string) bool {
	return q.policyEngines[scanner.Normalize(engine)]
}

// ValidateScanners refuses an unknown engine, an engine declared twice
// (quality_gates.sast counts as semgrep), a bad fail_on and bad options.
func (q QualityGatesConfig) ValidateScanners() error {
	seen := map[string]string{}
	for _, s := range q.AllScanners() {
		if strings.TrimSpace(s.Engine) == "" {
			return fmt.Errorf("%s.engine: must not be empty", s.Label())
		}
		if err := s.validate(); err != nil {
			return err
		}
		if first, dup := seen[s.Name()]; dup {
			return fmt.Errorf("%s: engine %q is already declared by %s", s.Label(), s.Name(), first)
		}
		seen[s.Name()] = s.Label()
	}
	return nil
}
