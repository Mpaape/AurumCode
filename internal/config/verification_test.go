package config

import "testing"

func TestReviewVerificationDefaultsAndValidation(t *testing.T) {
	var absent ReviewVerificationConfig
	if !absent.Active() || absent.EffectiveMaxCalls() != 0 || absent.Validate() != nil {
		t.Fatal("absent review.verification is on without a call ceiling (issue #179)")
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
