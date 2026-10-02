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

// SupplyChainConfig is quality_gates.supply_chain (AUR-551): signing the
// SBOM (AUR-549's own output) and/or the artifact image with
// Sigstore/Cosign, verifiable afterwards.
//
//	quality_gates:
//	  supply_chain:
//	    engine: cosign
//	    sign_sbom: true
//	    sign_artifacts: true
//	    artifacts: ["ghcr.io/org/app@sha256:<64 hex>"]
//
// Engine is a closed vocabulary of exactly one value ("cosign") -- like
// SBOMGeneratorConfig.Tool/Format above, anything else (including the
// empty string on a declared-but-incomplete section) is a loud
// configuration error at load time, never a silent no-op and never an
// attempt to shell out to an unreviewed signer. Artifacts is optional: the
// CLI's own --image flags (aur551.go, cmd/aurumcode) may name the image(s)
// to sign instead, but any entry that does appear here is validated the
// same way -- pinned by a sha256 digest, never a mutable tag.
type SupplyChainConfig struct {
	Engine        string   `yaml:"engine"`
	SignSBOM      bool     `yaml:"sign_sbom"`
	SignArtifacts bool     `yaml:"sign_artifacts"`
	Artifacts     []string `yaml:"artifacts"`
}

// Declared reports whether this section was actually written -- a nil
// receiver (the key absent from the yaml entirely) is never declared. The
// pointer's own nilness is QualityGatesConfig's existence signal
// (ApplyCentralPolicy already branches on it, central.go); this method
// exists only so cmd/aurumcode's AUR-551 wiring can ask the same question
// the other two sections' own Declared() methods already answer, with a
// nil-safe receiver instead of a naked "!= nil" scattered at call sites.
func (s *SupplyChainConfig) Declared() bool {
	return s != nil
}

// Validate enforces this card's closed vocabulary and its digest-pinning
// rule. A nil receiver (not declared at all) is never validated -- same
// contract as SBOMGeneratorConfig.Validate and SsorDtrackConfig.Validate
// above.
func (s *SupplyChainConfig) Validate() error {
	if !s.Declared() {
		return nil
	}
	if strings.TrimSpace(s.Engine) != "cosign" {
		return fmt.Errorf("quality_gates.supply_chain.engine: only %q is supported, got %q", "cosign", s.Engine)
	}
	for _, ref := range s.Artifacts {
		if err := validateImageDigestRef(ref); err != nil {
			return fmt.Errorf("quality_gates.supply_chain.artifacts: %w", err)
		}
	}
	return nil
}

// validateImageDigestRef refuses any image reference that is not pinned
// by a full sha256 digest: a tag alone (":latest", ":v1", or nothing at
// all) is exactly the race/tamper window the card's own public contract
// refuses ("imagem a assinar vem por referencia com digest (@sha256:);
// tag sem digest e recusada"). Deliberately duplicated (never imported) in
// internal/supplychain's own ValidateArtifactRef, the same way
// isMajorMinorVersion above is duplicated rather than imported from
// internal/sbom: internal/supplychain's CLI-flag validation (aur551.go)
// must refuse an unpinned --image before this package is even reached,
// and internal/config must never import a package that itself might need
// to import internal/config later.
func validateImageDigestRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("image reference must not be empty")
	}
	// H3: a reference beginning with "-" could be read as a flag by the
	// external cosign binary depending on argv position -- duplicated
	// (never imported) from internal/supplychain.ValidateArtifactRef's
	// own identical check, for the same reason the digest check below is
	// already duplicated rather than imported.
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("image reference %q must not start with \"-\"", ref)
	}
	idx := strings.LastIndex(ref, "@sha256:")
	if idx < 0 {
		return fmt.Errorf("image reference %q must be pinned by digest (@sha256:<64 hex>), not a tag", ref)
	}
	digest := ref[idx+len("@sha256:"):]
	if len(digest) != 64 {
		return fmt.Errorf("image reference %q has a sha256 digest of the wrong length", ref)
	}
	for _, r := range digest {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return fmt.Errorf("image reference %q has a non-hex sha256 digest", ref)
		}
	}
	return nil
}

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
