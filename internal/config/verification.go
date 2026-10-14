package config

import "fmt"

// ReviewVerificationConfig is review.verification: the adversarial check
// every model finding that would block the gate goes through before the
// gate counts it. Only a refutation backed by a literal quote of the
// reviewed code demotes a finding; anything else keeps it blocking.
type ReviewVerificationConfig struct {
	// Enabled switches the verification; absent means on.
	Enabled *bool `yaml:"enabled"`
	// MaxCalls bounds the verification calls of one review; absent or zero
	// means no ceiling (issue #179): every blocking finding is verified. A
	// positive value is an explicit ceiling, and a finding beyond it keeps
	// blocking.
	MaxCalls int `yaml:"max_calls"`
}

// Active reports whether the verification runs (the default).
func (v ReviewVerificationConfig) Active() bool { return v.Enabled == nil || *v.Enabled }

// EffectiveMaxCalls is the declared ceiling; 0 means none.
func (v ReviewVerificationConfig) EffectiveMaxCalls() int { return v.MaxCalls }

// Validate refuses a negative ceiling (zero means no ceiling).
func (v ReviewVerificationConfig) Validate() error {
	if v.MaxCalls < 0 {
		return fmt.Errorf("review.verification.max_calls: must be positive (got %d)", v.MaxCalls)
	}
	return nil
}
