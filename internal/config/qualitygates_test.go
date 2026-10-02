package config

import (
	"strings"
	"testing"
)

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

// TestApplyCentralPolicySupplyChainPrecedence is AUR-551's own proof of
// the same per-section precedence TestApplyCentralPolicyQualityGatesPerSectionPrecedence
// already establishes for ssor_dtrack: a policy that declares
// supply_chain wins outright over the repository's own section (with a
// named warning), and a policy that never mentions supply_chain at all
// leaves the repository's own section completely untouched.
func TestApplyCentralPolicySupplyChainPrecedence(t *testing.T) {
	repo := &Config{
		QualityGates: QualityGatesConfig{
			SupplyChain: &SupplyChainConfig{
				Engine: "cosign", SignSBOM: true, SignArtifacts: false,
			},
		},
	}

	t.Run("PolicyDeclaresNothing: repo's own section stands", func(t *testing.T) {
		central := &Config{}
		effective, warnings := ApplyCentralPolicy(repo, central)
		if effective.QualityGates.SupplyChain == nil {
			t.Fatalf("expected repo's own supply_chain section to survive untouched")
		}
		if !effective.QualityGates.SupplyChain.SignSBOM || effective.QualityGates.SupplyChain.SignArtifacts {
			t.Fatalf("got %+v, want the repo's own sign_sbom=true/sign_artifacts=false", effective.QualityGates.SupplyChain)
		}
		if len(warnings) != 0 {
			t.Fatalf("expected no warnings when the policy never mentions supply_chain, got %v", warnings)
		}
	})

	t.Run("PolicyDeclaresSupplyChain: policy wins, repo's dropped with a warning", func(t *testing.T) {
		central := &Config{
			QualityGates: QualityGatesConfig{
				SupplyChain: &SupplyChainConfig{
					Engine: "cosign", SignSBOM: true, SignArtifacts: true,
					Artifacts: []string{"ghcr.io/org/app@sha256:" + strings.Repeat("b", 64)},
				},
			},
		}
		effective, warnings := ApplyCentralPolicy(repo, central)
		if effective.QualityGates.SupplyChain == nil || !effective.QualityGates.SupplyChain.SignArtifacts {
			t.Fatalf("expected the policy's own section to win, got %+v", effective.QualityGates.SupplyChain)
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

	t.Run("PolicyDeclaresOnlySast: repo's own supply_chain is untouched", func(t *testing.T) {
		central := &Config{
			QualityGates: QualityGatesConfig{Sast: &SastConfig{}},
		}
		effective, _ := ApplyCentralPolicy(repo, central)
		if effective.QualityGates.SupplyChain == nil || !effective.QualityGates.SupplyChain.SignSBOM {
			t.Fatalf("a policy opinion on sast must not disable the repo's own, policy-unaddressed supply_chain section: got %+v", effective.QualityGates.SupplyChain)
		}
	})
}

// TestSupplyChainConfigValidate is AUR-551's own config-load proof: a
// declared section with an engine other than "cosign" (including the
// empty string) is refused, and an artifacts entry without a sha256
// digest is refused by name -- both before any cosign call.
func TestSupplyChainConfigValidate(t *testing.T) {
	var nilCfg *SupplyChainConfig
	if err := nilCfg.Validate(); err != nil {
		t.Fatalf("a nil (undeclared) section must validate cleanly: %v", err)
	}

	valid := &SupplyChainConfig{Engine: "cosign", SignSBOM: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error for a valid section: %v", err)
	}

	for _, engine := range []string{"", "notcosign", "COSIGN"} {
		cfg := &SupplyChainConfig{Engine: engine, SignSBOM: true}
		if err := cfg.Validate(); err == nil {
			t.Errorf("engine %q: expected a validation error, got nil", engine)
		}
	}

	badDigest := &SupplyChainConfig{
		Engine: "cosign", SignArtifacts: true,
		Artifacts: []string{"ghcr.io/org/app:latest"},
	}
	err := badDigest.Validate()
	if err == nil {
		t.Fatalf("expected an error for an artifact without a sha256 digest, got nil")
	}
	if !strings.Contains(err.Error(), "ghcr.io/org/app:latest") {
		t.Fatalf("error does not name the offending reference: %v", err)
	}

	goodDigest := &SupplyChainConfig{
		Engine: "cosign", SignArtifacts: true,
		Artifacts: []string{"ghcr.io/org/app@sha256:" + strings.Repeat("c", 64)},
	}
	if err := goodDigest.Validate(); err != nil {
		t.Fatalf("unexpected error for a digest-pinned artifact: %v", err)
	}

	// H3: a leading "-" could be read as a cosign flag depending on argv
	// position (never relied on "--" alone to neutralize it) -- refused
	// here at config-load time, the same way an unpinned reference is.
	leadingDash := &SupplyChainConfig{
		Engine: "cosign", SignArtifacts: true,
		Artifacts: []string{"-ghcr.io/org/app@sha256:" + strings.Repeat("c", 64)},
	}
	if err := leadingDash.Validate(); err == nil {
		t.Fatalf("expected an error for an artifact reference starting with \"-\", got nil")
	}
}
