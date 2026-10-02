package config

import (
	"fmt"
	"strings"
)

// QualityGatesConfig is quality_gates: one shared top-level key three
// corporate-adoption cards each write their own subsection into (AUR-548
// Semgrep/SAST, AUR-549 SBOM/Trivy, AUR-550 Dependency-Track). Every
// subsection is a pointer: nil means "not declared at all", distinct from
// "declared with every inner field still at its zero value" -- a central
// policy can enable a subsection with nothing but defaults, and
// ApplyCentralPolicy needs to tell that apart from the key being absent
// altogether (ValidatePolicyOutsideReviewedTree's sibling concern, same
// idea GateConfig.Declared() already uses for a single flat section).
//
// Each card defines only the fields it needs inside its own subsection;
// the rest stay empty structs (or, within SsorDtrackConfig, absent
// fields) for that card's own builder to add as sibling fields, never by
// this card reaching into another card's section.
type QualityGatesConfig struct {
	Sast        *SastConfig        `yaml:"sast"`
	SsorDtrack  *SsorDtrackConfig  `yaml:"ssor_dtrack"`
	SupplyChain *SupplyChainConfig `yaml:"supply_chain"`
}

// Declared reports whether quality_gates had ANY of its three subsections
// written at all.
func (q QualityGatesConfig) Declared() bool {
	return q.Sast != nil || q.SsorDtrack != nil || q.SupplyChain != nil
}

// SastConfig (quality_gates.sast, AUR-548's own section) lives in sast.go.

// SsorDtrackConfig is quality_gates.ssor_dtrack, shared between AUR-549
// (SBOMGenerator: the CycloneDX SBOM this section's sibling generates with
// Trivy) and AUR-550 (every other field: the Dependency-Track upload/gate
// itself). See internal/config/dtrack.go for AUR-550's own
// Declared/Validate/EffectiveTimeoutSeconds/EffectivePollIntervalSeconds/
// SBOMOutputFile methods on this type -- kept in that file, not here,
// because this file only owns the SHARED struct shape the two cards
// converge on, never either card's own behavior.
type SsorDtrackConfig struct {
	// Enabled is AUR-550's own opt-in switch for the Dependency-Track
	// upload/gate half of this section; it does not affect SBOMGenerator
	// below at all -- AUR-549's own `aurumcode sbom` command reads
	// SBOMGenerator.Declared() independently.
	Enabled bool `yaml:"enabled"`
	// ServerAPIHost is the Dependency-Track v5 API's base URL. See
	// internal/dtrack.ValidateHost for the HTTPS-only rule AUR-550's own
	// Validate enforces: plain http is accepted only for a loopback IP
	// literal, so a test (or AUR-550's own acceptance script) can point
	// it at an httptest.Server without this program ever accepting a
	// production endpoint over plain HTTP.
	ServerAPIHost   string `yaml:"server_api_host"`
	APIKeySecret    string `yaml:"api_key_secret"`
	ProjectIDSecret string `yaml:"project_id_secret"`

	Thresholds SsorDtrackThresholds `yaml:"thresholds"`

	TimeoutSeconds      int `yaml:"timeout_seconds"`
	PollIntervalSeconds int `yaml:"poll_interval_seconds"`

	SBOMGenerator SBOMGeneratorConfig `yaml:"sbom_generator"`
}

// SupplyChainConfig is quality_gates.supply_chain, reserved for a future
// xBOM/supply-chain card; no card needs a field here yet.
type SupplyChainConfig struct{}

// SBOMGeneratorConfig is AUR-549's own quality_gates.ssor_dtrack.sbom_generator:
//
//	quality_gates:
//	  ssor_dtrack:
//	    sbom_generator:
//	      tool: trivy
//	      format: cyclonedx
//	      spec_version: "1.6"
//	      output_file: sbom_app_cyclonedx.json
type SBOMGeneratorConfig struct {
	Tool        string `yaml:"tool"`
	Format      string `yaml:"format"`
	SpecVersion string `yaml:"spec_version"`
	OutputFile  string `yaml:"output_file"`
}

// Declared reports whether this section was actually written, as opposed
// to being the absent zero value -- the same "zero value means off"
// contract GateConfig already uses. Any one of the four fields being
// non-blank counts: a config that names only some of them still has to go
// through Validate so the missing ones are reported, never silently
// treated as "nothing declared at all".
func (s SBOMGeneratorConfig) Declared() bool {
	return strings.TrimSpace(s.Tool) != "" ||
		strings.TrimSpace(s.Format) != "" ||
		strings.TrimSpace(s.SpecVersion) != "" ||
		strings.TrimSpace(s.OutputFile) != ""
}

// Validate enforces this card's closed vocabulary. tool/format are not
// free text: the card's Outcome names exactly one tool (trivy) and one
// format (cyclonedx), so anything else is a loud configuration error at
// load time, never a silent no-op and never an attempt to shell out to a
// different, unreviewed tool.
func (s SBOMGeneratorConfig) Validate() error {
	if !s.Declared() {
		return nil
	}
	if strings.TrimSpace(s.Tool) != "trivy" {
		return fmt.Errorf("quality_gates.ssor_dtrack.sbom_generator.tool: only %q is supported, got %q", "trivy", s.Tool)
	}
	if strings.TrimSpace(s.Format) != "cyclonedx" {
		return fmt.Errorf("quality_gates.ssor_dtrack.sbom_generator.format: only %q is supported, got %q", "cyclonedx", s.Format)
	}
	if strings.TrimSpace(s.SpecVersion) == "" {
		return fmt.Errorf("quality_gates.ssor_dtrack.sbom_generator.spec_version: must not be empty")
	}
	// Card v3: spec_version is a MINIMUM ("1.6+"), compared by major.minor
	// (internal/sbom.specVersionAtLeast -- same major, minor at or above
	// this value), because the digest-pinned Trivy emits a newer minor
	// (1.7) with no flag to request an older one. A value that cannot
	// parse that way can never be compared correctly downstream, so it
	// fails HERE, at load time, rather than surfacing as a confusing SBOM
	// rejection later.
	if !isMajorMinorVersion(strings.TrimSpace(s.SpecVersion)) {
		return fmt.Errorf("quality_gates.ssor_dtrack.sbom_generator.spec_version: must be major.minor (e.g. \"1.6\"), got %q", s.SpecVersion)
	}
	if strings.TrimSpace(s.OutputFile) == "" {
		return fmt.Errorf("quality_gates.ssor_dtrack.sbom_generator.output_file: must not be empty")
	}
	return nil
}

// isMajorMinorVersion reports whether v is a strict "major.minor" version
// string: exactly one dot, both sides one or more ASCII decimal digits
// with no leading zero (unless the component is exactly "0"), no sign, no
// extra whitespace, no third component. Duplicated (rather than
// imported) from internal/sbom's own parseMajorMinor: internal/sbom
// already imports this package for GeneratorConfig, so the reverse import
// would be a cycle.
func isMajorMinorVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
