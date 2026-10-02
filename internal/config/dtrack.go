package config

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/dtrack"
)

// SbomGeneratorConfig is AUR-549's own
// quality_gates.ssor_dtrack.sbom_generator section, nested exactly where
// AUR-549.md's own documented example nests it -- under this card's own
// gate, not beside it. AUR-549 owns every behavior this section
// configures (which tool runs, which format/spec version it produces,
// whether it also scans an image); this card reads only OutputFile --
// the path to the already-generated CycloneDX document this card
// uploads. Generating that file is explicitly this card's own Non-goal.
// Field names/shape are coordinated with AUR-549's own builder so the two
// cards converge on the same struct without a later rewrite.
type SbomGeneratorConfig struct {
	Tool        string `yaml:"tool"`
	Format      string `yaml:"format"`
	SpecVersion string `yaml:"spec_version"`
	OutputFile  string `yaml:"output_file"`
}

// SsorDtrackThresholds is quality_gates.ssor_dtrack.thresholds. The zero
// value (every field 0) is the card's own documented default: zero
// criticals, zero highs, zero policy violations tolerated.
type SsorDtrackThresholds struct {
	MaxCritical      int `yaml:"max_critical"`
	MaxHigh          int `yaml:"max_high"`
	PolicyViolations int `yaml:"policy_violations"`
}

// AsClientThresholds converts to internal/dtrack's own Thresholds shape,
// so cmd/aurumcode can call dtrack.Run without this package importing
// cmd/aurumcode or dtrack importing config (avoiding a cycle either way).
func (t SsorDtrackThresholds) AsClientThresholds() dtrack.Thresholds {
	return dtrack.Thresholds{
		MaxCritical:      t.MaxCritical,
		MaxHigh:          t.MaxHigh,
		PolicyViolations: t.PolicyViolations,
	}
}

// DefaultDTrackTimeoutSeconds and DefaultDTrackPollIntervalSeconds are
// AUR-550's own documented/sane defaults, applied only when the section is
// declared (Enabled) and the field was left at its zero value -- an
// explicit 0 is indistinguishable from "not set" for a positive-only
// duration, so this package treats both the same way, exactly like
// internal/llm's own cost/timeout fields elsewhere in this engine.
const (
	DefaultDTrackTimeoutSeconds      = 180
	DefaultDTrackPollIntervalSeconds = 5
)

// SsorDtrackConfig is quality_gates.ssor_dtrack: AUR-550's own submission
// and metrics gate against an OWASP Dependency-Track v5 server. Host,
// project and API key never come from a literal in this program -- Host
// comes from this config (itself precedence-resolved by
// ApplyCentralPolicy), and APIKeySecret/ProjectIDSecret are only the
// NAMES of environment variables cmd/aurumcode reads at runtime (AC-005);
// the actual key and project id are never written anywhere in this
// repository.
//
// Every method here has a pointer receiver and is nil-safe (a nil
// *SsorDtrackConfig reports Declared() == false and defaults everything
// else): QualityGatesConfig.SsorDtrack is nil whenever the section is
// absent from the yaml entirely, which this package's callers must be
// able to pass straight through without a separate existence check.
type SsorDtrackConfig struct {
	Enabled bool `yaml:"enabled"`
	// ServerAPIHost is the Dependency-Track v5 API's base URL. See
	// internal/dtrack.ValidateHost for the HTTPS-only rule Validate below
	// enforces: plain http is accepted only for a loopback IP literal, so
	// a test (or this card's own acceptance script) can point it at an
	// httptest.Server without this program ever accepting a production
	// endpoint over plain HTTP.
	ServerAPIHost   string `yaml:"server_api_host"`
	APIKeySecret    string `yaml:"api_key_secret"`
	ProjectIDSecret string `yaml:"project_id_secret"`

	Thresholds SsorDtrackThresholds `yaml:"thresholds"`

	TimeoutSeconds      int `yaml:"timeout_seconds"`
	PollIntervalSeconds int `yaml:"poll_interval_seconds"`

	// SbomGenerator is AUR-549's own section -- nil when a config declares
	// ssor_dtrack but not yet its sbom_generator child (an incomplete
	// draft, or a run predating AUR-549's own merge). SBOMOutputFile below
	// is the one nil-safe accessor this card's own gate hook ever uses.
	SbomGenerator *SbomGeneratorConfig `yaml:"sbom_generator"`
}

// Declared reports whether this section was actually turned on. Unlike
// GateConfig.Declared (any field counts), this card's own gate is opt-in
// by its own "enabled" key alone -- a repository or policy that sets
// server_api_host/secrets ahead of flipping enabled:true changes nothing
// yet, which keeps a staged rollout inert until the operator is ready. A
// nil receiver (the section absent entirely) is never declared.
func (g *SsorDtrackConfig) Declared() bool {
	return g != nil && g.Enabled
}

// EffectiveTimeoutSeconds and EffectivePollIntervalSeconds apply
// AUR-550's own defaults (180s timeout, 5s poll interval) when the config
// left the field at its zero value, or is nil.
func (g *SsorDtrackConfig) EffectiveTimeoutSeconds() int {
	if g == nil || g.TimeoutSeconds <= 0 {
		return DefaultDTrackTimeoutSeconds
	}
	return g.TimeoutSeconds
}

func (g *SsorDtrackConfig) EffectivePollIntervalSeconds() int {
	if g == nil || g.PollIntervalSeconds <= 0 {
		return DefaultDTrackPollIntervalSeconds
	}
	return g.PollIntervalSeconds
}

// SBOMOutputFile is the nil-safe accessor for AUR-549's own
// sbom_generator.output_file -- "" when either ssor_dtrack or its
// sbom_generator child is absent, which the gate hook (cmd/aurumcode)
// turns into its own "SBOM unavailable" inconclusive reason, never a
// panic and never an upload of an empty/wrong file.
func (g *SsorDtrackConfig) SBOMOutputFile() string {
	if g == nil || g.SbomGenerator == nil {
		return ""
	}
	return g.SbomGenerator.OutputFile
}

// Validate normalizes and validates every field this section declares,
// exactly like GateConfig.Validate: a malformed ssor_dtrack section is a
// loud parse-time error, never a silently-inert or silently-dangerous
// configuration. A nil receiver or a section left disabled
// (Declared() == false) is never validated at all -- an operator staging
// host/secret names ahead of enabling it must not be blocked by an
// incomplete draft.
func (g *SsorDtrackConfig) Validate() error {
	if !g.Declared() {
		return nil
	}
	if strings.TrimSpace(g.ServerAPIHost) == "" {
		return fmt.Errorf("quality_gates.ssor_dtrack.server_api_host is required when enabled")
	}
	if err := dtrack.ValidateHost(g.ServerAPIHost); err != nil {
		return fmt.Errorf("quality_gates.ssor_dtrack.server_api_host: %w", err)
	}
	if strings.TrimSpace(g.APIKeySecret) == "" {
		return fmt.Errorf("quality_gates.ssor_dtrack.api_key_secret is required when enabled")
	}
	if strings.TrimSpace(g.ProjectIDSecret) == "" {
		return fmt.Errorf("quality_gates.ssor_dtrack.project_id_secret is required when enabled")
	}
	if g.Thresholds.MaxCritical < 0 || g.Thresholds.MaxHigh < 0 || g.Thresholds.PolicyViolations < 0 {
		return fmt.Errorf("quality_gates.ssor_dtrack.thresholds: negative threshold is not valid")
	}
	if g.TimeoutSeconds < 0 || g.PollIntervalSeconds < 0 {
		return fmt.Errorf("quality_gates.ssor_dtrack: timeout_seconds/poll_interval_seconds must not be negative")
	}
	return nil
}

// SastConfig is quality_gates.sast -- AUR-548's own section. This card
// does not implement SAST at all; this placeholder exists only so a
// central policy or repo config that already declares quality_gates.sast
// alongside ssor_dtrack decodes under LoadCentralPolicy's KnownFields(true)
// strict parse without waiting on AUR-548's own merge. AUR-548 owns this
// struct's real shape; expect it to replace this placeholder wholesale.
type SastConfig struct {
	Enabled bool `yaml:"enabled"`
}

// SupplyChainConfig is quality_gates.supply_chain -- AUR-551's own
// section (sign_sbom/sign_artifacts, per its own documented example).
// Same placeholder role as SastConfig above: this card implements
// neither signing behavior, it only keeps the shared QualityGatesConfig
// struct decodable ahead of AUR-551's own merge.
type SupplyChainConfig struct {
	Enabled       bool `yaml:"enabled"`
	SignSbom      bool `yaml:"sign_sbom"`
	SignArtifacts bool `yaml:"sign_artifacts"`
}

// QualityGatesConfig is the quality_gates top-level section, shared
// across AUR-548 (Sast), AUR-550 (SsorDtrack, this card) and AUR-551
// (SupplyChain) by coordinated agreement so the three cards' own edits to
// internal/config converge on one struct instead of three incompatible
// ones. Each pointer is nil exactly when its own section is absent from
// the yaml; every accessor on this card's own SsorDtrack is written to
// be nil-safe for exactly that reason.
type QualityGatesConfig struct {
	Sast        *SastConfig        `yaml:"sast"`
	SsorDtrack  *SsorDtrackConfig  `yaml:"ssor_dtrack"`
	SupplyChain *SupplyChainConfig `yaml:"supply_chain"`
}
