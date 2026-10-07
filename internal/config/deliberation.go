package config

import (
	"fmt"
	"time"
)

// Conservative defaults of the deliberation limits: a few rounds, a token
// ceiling of the order of one review prompt, and a tool timeout that fits a
// local scan.
const (
	DefaultDeliberationMaxRounds         = 3
	DefaultDeliberationMaxCostTokens     = 60000
	DefaultDeliberationToolTimeoutSecond = 120
)

// DeliberationConfig is the deliberation section (nil = not declared): when
// enabled, the model may ask for the tools the engine offers (scanners not
// required by the configuration, codebase context, skill sections) within
// these limits. A limit that is exceeded makes the review inconclusive.
// Absent or `enabled: false`, nothing is offered and every enabled scanner
// runs before the model exactly as before.
type DeliberationConfig struct {
	Enabled               bool `yaml:"enabled"`
	MaxRounds             int  `yaml:"max_rounds"`
	MaxCostTokens         int  `yaml:"max_cost_tokens"`
	PerToolTimeoutSeconds int  `yaml:"per_tool_timeout_seconds"`
	// MaxReadBytes bounds what the repository tools (AUR-526) return to the
	// model over one review; 0 means DefaultDeliberationMaxReadBytes.
	MaxReadBytes int `yaml:"max_read_bytes"`
	// SecretPaths are globs of secret files the repository tools refuse,
	// added to the embedded catalog (secret_paths.yml), never replacing it.
	SecretPaths []string `yaml:"secret_paths"`
}

// Active reports whether the model may ask for tools.
func (d *DeliberationConfig) Active() bool { return d != nil && d.Enabled }

// EffectiveLimits returns the limits with each absent value defaulted.
func (d *DeliberationConfig) EffectiveLimits() (maxRounds, maxCostTokens int, perTool time.Duration) {
	maxRounds, maxCostTokens, seconds := DefaultDeliberationMaxRounds, DefaultDeliberationMaxCostTokens, DefaultDeliberationToolTimeoutSecond
	if d != nil {
		if d.MaxRounds != 0 {
			maxRounds = d.MaxRounds
		}
		if d.MaxCostTokens != 0 {
			maxCostTokens = d.MaxCostTokens
		}
		if d.PerToolTimeoutSeconds != 0 {
			seconds = d.PerToolTimeoutSeconds
		}
	}
	return maxRounds, maxCostTokens, time.Duration(seconds) * time.Second
}

// Validate refuses a negative limit (zero means the default).
func (d *DeliberationConfig) Validate() error {
	if d == nil {
		return nil
	}
	for _, f := range []struct {
		key   string
		value int
	}{
		{"max_rounds", d.MaxRounds},
		{"max_cost_tokens", d.MaxCostTokens},
		{"per_tool_timeout_seconds", d.PerToolTimeoutSeconds},
		{"max_read_bytes", d.MaxReadBytes},
	} {
		if f.value < 0 {
			return fmt.Errorf("deliberation.%s: must be positive (got %d)", f.key, f.value)
		}
	}
	return nil
}

// mergeDeliberation governs the section like analysis_data: a policy that
// declares it decides alone; a policy silent on it leaves the repository's.
func mergeDeliberation(effective *Config, repo, central *Config) []ProviderWarning {
	if central.Deliberation == nil {
		return nil
	}
	effective.Deliberation = central.Deliberation
	if repo.Deliberation == nil {
		return nil
	}
	return []ProviderWarning{{
		Provider: centralPolicyProvider,
		Reason:   "deliberation do config do repositório foi ignorado: a política central decide sozinha",
	}}
}
