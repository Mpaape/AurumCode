package config

import "testing"

func TestReviewVerificationDefaultsAndValidation(t *testing.T) {
	var absent ReviewVerificationConfig
	if !absent.Active() || absent.EffectiveMaxCalls() != DefaultVerificationMaxCalls || absent.Validate() != nil {
		t.Fatal("absent review.verification is on with the default ceiling")
	}
	off := false
	if (ReviewVerificationConfig{Enabled: &off}).Active() {
		t.Fatal("enabled: false turns the verification off")
	}
	if (ReviewVerificationConfig{MaxCalls: 2}).EffectiveMaxCalls() != 2 {
		t.Fatal("max_calls is the declared ceiling")
	}
	if (ReviewVerificationConfig{MaxCalls: -1}).Validate() == nil {
		t.Fatal("a negative max_calls must be refused")
	}
}
