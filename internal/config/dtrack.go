package config

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/dtrack"
)

// DTrackSBOMGeneratorConfig is AUR-549's own
// quality_gates.ssor_dtrack.sbom_generator section, nested exactly where
// AUR-549.md's own documented example nests it -- under this card's own
// gate, not beside it. AUR-549 owns every behavior this section
// configures (which tool runs, which format/spec version it produces,
// whether it also scans an image); AUR-549's own card paths do not
// include internal/config, so this card owns the struct itself, and
// reads only OutputFile -- the path to the already-generated CycloneDX
// document this card uploads. Generating that file is explicitly this
// card's own Non-goal.
type DTrackSBOMGeneratorConfig struct {
	Tool        string `yaml:"tool"`
	Format      string `yaml:"format"`
	SpecVersion string `yaml:"spec_version"`
	OutputFile  string `yaml:"output_file"`
}

// DTrackThresholds is quality_gates.ssor_dtrack.thresholds. The zero value
// (every field 0) is the card's own documented default: zero criticals,
// zero highs, zero policy violations tolerated.
type DTrackThresholds struct {
	MaxCritical      int `yaml:"max_critical"`
	MaxHigh          int `yaml:"max_high"`
	PolicyViolations int `yaml:"policy_violations"`
}

// AsClientThresholds converts to internal/dtrack's own Thresholds shape,
// so cmd/aurumcode can call dtrack.Run without this package importing
// cmd/aurumcode or dtrack importing config (avoiding a cycle either way).
func (t DTrackThresholds) AsClientThresholds() dtrack.Thresholds {
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

// DTrackGateConfig is quality_gates.ssor_dtrack: AUR-550's own submission
// and metrics gate against an OWASP Dependency-Track v5 server. Host,
// project and API key never come from a literal in this program -- Host
// comes from this config (itself precedence-resolved by
// ApplyCentralPolicy, below), and APIKeySecret/ProjectIDSecret are only
// the NAMES of environment variables cmd/aurumcode reads at runtime
// (AC-005); the actual key and project id are never written anywhere in
// this repository.
type DTrackGateConfig struct {
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

	Thresholds DTrackThresholds `yaml:"thresholds"`

	TimeoutSeconds      int `yaml:"timeout_seconds"`
	PollIntervalSeconds int `yaml:"poll_interval_seconds"`

	SBOMGenerator DTrackSBOMGeneratorConfig `yaml:"sbom_generator"`
}

// Declared reports whether this section was actually turned on. Unlike
// GateConfig.Declared (any field counts), this card's own gate is opt-in
// by its own "enabled" key alone -- a repository or policy that sets
// server_api_host/secrets ahead of flipping enabled:true changes nothing
// yet, which keeps a staged rollout inert until the operator is ready.
func (g DTrackGateConfig) Declared() bool {
	return g.Enabled
}

// EffectiveTimeout and EffectivePollInterval apply AUR-550's own defaults
// (180s timeout, 5s poll interval) when the config left the field at its
// zero value.
func (g DTrackGateConfig) EffectiveTimeoutSeconds() int {
	if g.TimeoutSeconds > 0 {
		return g.TimeoutSeconds
	}
	return DefaultDTrackTimeoutSeconds
}

func (g DTrackGateConfig) EffectivePollIntervalSeconds() int {
	if g.PollIntervalSeconds > 0 {
		return g.PollIntervalSeconds
	}
	return DefaultDTrackPollIntervalSeconds
}

// Validate normalizes and validates every field this section declares,
// exactly like GateConfig.Validate: a malformed ssor_dtrack section is a
// loud parse-time error, never a silently-inert or silently-dangerous
// configuration. A section left disabled (Declared() == false) is never
// validated at all -- an operator staging host/secret names ahead of
// enabling it must not be blocked by an incomplete draft.
func (g DTrackGateConfig) Validate() error {
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

// QualityGatesConfig is the quality_gates top-level section. AUR-550 owns
// only its own ssor_dtrack child here; a sibling gate (SAST, image
// signing) is a later card's own addition to this struct, not this one's
// to design.
type QualityGatesConfig struct {
	SSORDTrack DTrackGateConfig `yaml:"ssor_dtrack"`
}
