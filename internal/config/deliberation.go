package config

import (
	"fmt"
	"time"
)

// Defaults of the limits that guard the conversation itself: a few rounds,
// which guarantees it ends, and a tool timeout that fits a local scan. There
// are no default size ceilings: max_cost_tokens, max_read_bytes and
// max_cache_bytes bound the work only when the configuration declares a
// positive value (zero or absent means no ceiling).
const (
	DefaultDeliberationMaxRounds         = 3
	DefaultDeliberationToolTimeoutSecond = 120
)

// DeliberationConfig is the deliberation section (nil = not declared): when
// enabled, the model may ask for the tools the engine offers (scanners not
// required by the configuration, codebase context, skill sections) within
// these limits. A limit that is exceeded makes the review inconclusive.
// Absent or `enabled: false`, nothing is offered and every enabled scanner
// runs before the model exactly as before.
type DeliberationConfig struct {
	Enabled   bool `yaml:"enabled"`
	MaxRounds int  `yaml:"max_rounds"`
	// MaxCostTokens bounds the tokens the deliberation adds beyond the base
	// prompt (Transcript.CostTokens). Absent or 0 means no ceiling; a positive
	// value is an explicit ceiling, and exceeding it makes the review
	// inconclusive (deliberation_limit:max_cost_tokens).
	MaxCostTokens         int `yaml:"max_cost_tokens"`
	PerToolTimeoutSeconds int `yaml:"per_tool_timeout_seconds"`
	// MaxReadBytes bounds what the repository tools (AUR-526) return to the
	// model over one review. Absent or 0 means no ceiling; a positive value is
	// an explicit one (deliberation_limit:max_read_bytes).
	MaxReadBytes int `yaml:"max_read_bytes"`
	// MaxCacheBytes bounds the file bytes the repository tools keep in
	// memory over one review. Absent or 0 means no ceiling; a positive value is
	// an explicit one (deliberation_limit:max_cache_bytes).
	MaxCacheBytes int `yaml:"max_cache_bytes"`
	// SecretPaths are globs of secret files the repository tools refuse,
	// added to the embedded catalog (secret_paths.yml), never replacing it.
	SecretPaths []string `yaml:"secret_paths"`
	// DependencyReachability (AUR-531) asks the model, with the repository
	// tools, whether the code uses the vulnerable part of each dependency
	// advisory; the explanation travels beside the finding and never
	// changes it. Off unless written.
	DependencyReachability bool `yaml:"dependency_reachability"`
}

// Active reports whether the model may ask for tools.
func (d *DeliberationConfig) Active() bool { return d != nil && d.Enabled }

// EffectiveLimits returns the limits with max_rounds and the tool timeout
// defaulted. maxCostTokens is 0, which means no ceiling, unless
// max_cost_tokens declares a positive one.
func (d *DeliberationConfig) EffectiveLimits() (maxRounds, maxCostTokens int, perTool time.Duration) {
	maxRounds, seconds := DefaultDeliberationMaxRounds, DefaultDeliberationToolTimeoutSecond
	if d != nil {
		if d.MaxRounds != 0 {
			maxRounds = d.MaxRounds
		}
		maxCostTokens = d.MaxCostTokens
		if d.PerToolTimeoutSeconds != 0 {
			seconds = d.PerToolTimeoutSeconds
		}
	}
	return maxRounds, maxCostTokens, time.Duration(seconds) * time.Second
}

// Validate refuses a negative limit. Zero is valid for every key: the
// default for max_rounds and per_tool_timeout_seconds, and no ceiling for
// max_cost_tokens, max_read_bytes and max_cache_bytes.
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
		{"max_cache_bytes", d.MaxCacheBytes},
	} {
		if f.value < 0 {
			return fmt.Errorf("deliberation.%s: must not be negative (got %d)", f.key, f.value)
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
