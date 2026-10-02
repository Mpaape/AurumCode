// QualityGatesConfig is the shared `quality_gates:` top-level shape every
// corporate-adoption card under this office uses: AUR-548 (internal/config/
// sast.go) owns Sast; AUR-549/550 (SBOM + OWASP Dependency-Track) own
// SsorDtrack; AUR-551 (Sigstore/Cosign signing) own SupplyChain. Each
// pointer field is nil when its own section is entirely absent from the
// yml -- the zero-config case every one of these cards preserves.
//
// ApplyCentralPolicy (central.go) governs this struct PER SECTION: a
// central policy that declares quality_gates.sast says nothing at all
// about quality_gates.ssor_dtrack or quality_gates.supply_chain, and a
// repository's own, undeclared sections survive untouched. Only a section
// the policy DOES declare is taken over wholesale (with a named warning
// when the repository had declared that same section), exactly like
// Gate/Rules/Ignore/Exceptions -- but scoped to that one section, never
// the whole QualityGatesConfig at once.
package config

// SsorDtrackConfig is AUR-549/550's own section (SBOM generation + OWASP
// Dependency-Track submission/thresholds). Defined here as an empty
// placeholder only so QualityGatesConfig's shape is stable before those
// cards land; AUR-548 never reads or writes it.
type SsorDtrackConfig struct{}

// SupplyChainConfig is AUR-551's own section (Sigstore/Cosign signing and
// the remaining xBOM preparation). Defined here as an empty placeholder
// only, for the same reason as SsorDtrackConfig; AUR-548 never reads or
// writes it.
type SupplyChainConfig struct{}

// QualityGatesConfig is the `quality_gates:` top-level section.
type QualityGatesConfig struct {
	Sast        *SastConfig        `yaml:"sast"`
	SsorDtrack  *SsorDtrackConfig  `yaml:"ssor_dtrack"`
	SupplyChain *SupplyChainConfig `yaml:"supply_chain"`
}
