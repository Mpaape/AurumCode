package config

import "fmt"

// DefaultVerificationMaxCalls is how many verification calls one review may
// make when review.verification.max_calls is absent.
const DefaultVerificationMaxCalls = 8

// ReviewVerificationConfig is review.verification: the adversarial check
// every model finding that would block the gate goes through before the
// gate counts it. Only a refutation backed by a literal quote of the
// reviewed code demotes a finding; anything else keeps it blocking.
type ReviewVerificationConfig struct {
	// Enabled switches the verification; absent means on.
	Enabled *bool `yaml:"enabled"`
	// MaxCalls bounds the verification calls of one review; zero means
	// DefaultVerificationMaxCalls. A finding beyond the ceiling keeps
	// blocking.
	MaxCalls int `yaml:"max_calls"`
}

// Active reports whether the verification runs (the default).
func (v ReviewVerificationConfig) Active() bool { return v.Enabled == nil || *v.Enabled }

// EffectiveMaxCalls is the declared ceiling or the default.
func (v ReviewVerificationConfig) EffectiveMaxCalls() int {
	if v.MaxCalls > 0 {
		return v.MaxCalls
	}
	return DefaultVerificationMaxCalls
}

// Validate refuses a negative ceiling (zero means the default).
func (v ReviewVerificationConfig) Validate() error {
	if v.MaxCalls < 0 {
		return fmt.Errorf("review.verification.max_calls: must be positive (got %d)", v.MaxCalls)
	}
	return nil
}
