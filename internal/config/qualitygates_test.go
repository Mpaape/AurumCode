package config

import "testing"

// TestSBOMGeneratorConfigValidateSpecVersionFormat is AUR-549 v3: config
// load fails closed when spec_version is not a strict major.minor
// string, since internal/sbom compares it that way (a MINIMUM, same
// major) and could never compare anything else correctly.
func TestSBOMGeneratorConfigValidateSpecVersionFormat(t *testing.T) {
	base := SBOMGeneratorConfig{Tool: "trivy", Format: "cyclonedx", OutputFile: "sbom.json"}

	valid := base
	valid.SpecVersion = "1.6"
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error for a valid major.minor spec_version: %v", err)
	}

	invalid := []string{"1", "1.6.0", "v1.6", "1.", ".6", "1.a"}
	for _, sv := range invalid {
		cfg := base
		cfg.SpecVersion = sv
		if err := cfg.Validate(); err == nil {
			t.Errorf("spec_version %q: expected a validation error, got nil", sv)
		}
	}
}

// TestApplyCentralPolicyQualityGatesPerSectionPrecedence is AUR-549 v2's
// own proof that quality_gates is governed PER SECTION, unlike Gate/
// Exceptions (which a policy governs outright, declared or not): a
// policy that only ever mentions one subsection must not also silently
// turn off a repository's own, policy-unaddressed subsection.
func TestApplyCentralPolicyQualityGatesPerSectionPrecedence(t *testing.T) {
	repo := &Config{
		QualityGates: QualityGatesConfig{
			SsorDtrack: &SsorDtrackConfig{
				SBOMGenerator: SBOMGeneratorConfig{
					Tool: "trivy", Format: "cyclonedx", SpecVersion: "1.6", OutputFile: "repo.json",
				},
			},
		},
	}

	t.Run("PolicyDeclaresNothing: repo's own section stands", func(t *testing.T) {
		central := &Config{}
		effective, warnings := ApplyCentralPolicy(repo, central)
		if effective.QualityGates.SsorDtrack == nil {
			t.Fatalf("expected repo's own ssor_dtrack section to survive untouched")
		}
		if effective.QualityGates.SsorDtrack.SBOMGenerator.OutputFile != "repo.json" {
			t.Fatalf("got output_file %q, want repo.json", effective.QualityGates.SsorDtrack.SBOMGenerator.OutputFile)
		}
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings when the policy never mentions ssor_dtrack, got %v", warnings)
		}
	})

	t.Run("PolicyDeclaresSsorDtrack: policy wins, repo's dropped with a warning", func(t *testing.T) {
		central := &Config{
			QualityGates: QualityGatesConfig{
				SsorDtrack: &SsorDtrackConfig{
					SBOMGenerator: SBOMGeneratorConfig{
						Tool: "trivy", Format: "cyclonedx", SpecVersion: "1.6", OutputFile: "policy.json",
					},
				},
			},
		}
		effective, warnings := ApplyCentralPolicy(repo, central)
		if effective.QualityGates.SsorDtrack == nil || effective.QualityGates.SsorDtrack.SBOMGenerator.OutputFile != "policy.json" {
			t.Fatalf("expected the policy's own output_file to win, got %+v", effective.QualityGates.SsorDtrack)
		}
		found := false
		for _, w := range warnings {
			if w.Provider == "politica central" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected a warning naming the dropped repo section, got %v", warnings)
		}
	})

	t.Run("PolicyDeclaresOnlySast: repo's own ssor_dtrack is untouched", func(t *testing.T) {
		central := &Config{
			QualityGates: QualityGatesConfig{Sast: &SastConfig{}},
		}
		effective, _ := ApplyCentralPolicy(repo, central)
		if effective.QualityGates.SsorDtrack == nil || effective.QualityGates.SsorDtrack.SBOMGenerator.OutputFile != "repo.json" {
			t.Fatalf("a policy opinion on sast must not disable the repo's own, policy-unaddressed ssor_dtrack section: got %+v", effective.QualityGates.SsorDtrack)
		}
	})
}
