package config

import (
	"strings"
	"testing"
)

// TestApplyCentralPolicyQualityGatesPerSection: AUR-549/AUR-550 coordinated
// on PER-SECTION precedence for quality_gates (unlike Gate/Rules/Ignore,
// which are all-or-nothing): the policy's own ssor_dtrack wins over the
// repo's (with a warning), while a repo's own sast section the policy
// never mentions survives untouched.
func TestApplyCentralPolicyQualityGatesPerSection(t *testing.T) {
	repo := &Config{
		QualityGates: QualityGatesConfig{
			Sast:       &SastConfig{},
			SsorDtrack: &SsorDtrackConfig{Enabled: true, ServerAPIHost: "https://repo-controlled.example.invalid"},
		},
	}
	central := &Config{
		QualityGates: QualityGatesConfig{
			SsorDtrack: &SsorDtrackConfig{Enabled: true, ServerAPIHost: "https://policy.example.invalid"},
		},
	}

	effective, warnings := ApplyCentralPolicy(repo, central)

	if effective.QualityGates.SsorDtrack == nil || effective.QualityGates.SsorDtrack.ServerAPIHost != "https://policy.example.invalid" {
		t.Fatalf("ssor_dtrack: a repository must not be able to redirect a policy-enabled section's own server_api_host, got %+v", effective.QualityGates.SsorDtrack)
	}
	if effective.QualityGates.Sast != repo.QualityGates.Sast {
		t.Fatalf("sast: a section the policy never mentions must survive from the repo's own config untouched (same pointer), got %+v", effective.QualityGates.Sast)
	}
	found := false
	for _, w := range warnings {
		if w.Provider == "politica central" && strings.Contains(w.Reason, "ssor_dtrack") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a named warning that the repo's own ssor_dtrack was dropped, got %+v", warnings)
	}
}

// TestApplyCentralPolicyQualityGatesPolicySilentKeepsRepo: when the
// central policy declares quality_gates at all, but never mentions
// ssor_dtrack specifically (its pointer is nil), the repository's own
// ssor_dtrack section must be left completely untouched -- a policy that
// opts into sast alone must never also silently disable an unrelated
// repo's own ssor_dtrack opt-in.
func TestApplyCentralPolicyQualityGatesPolicySilentKeepsRepo(t *testing.T) {
	repo := &Config{
		QualityGates: QualityGatesConfig{
			SsorDtrack: &SsorDtrackConfig{Enabled: true, ServerAPIHost: "https://repo.example.invalid"},
		},
	}
	central := &Config{
		QualityGates: QualityGatesConfig{
			Sast: &SastConfig{},
		},
	}

	effective, _ := ApplyCentralPolicy(repo, central)

	if effective.QualityGates.SsorDtrack == nil || effective.QualityGates.SsorDtrack.ServerAPIHost != "https://repo.example.invalid" {
		t.Fatalf("a policy silent on ssor_dtrack must leave the repo's own section untouched, got %+v", effective.QualityGates.SsorDtrack)
	}
}
