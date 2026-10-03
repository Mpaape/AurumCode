package config

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/dtrack"
)

// SsorDtrackThresholds is quality_gates.ssor_dtrack.thresholds. The zero
// value (every field 0) is the card's own documented default: zero
// criticals, zero highs, zero policy violations tolerated.
type SsorDtrackThresholds struct {
	MaxCritical      int `yaml:"max_critical"`
	MaxHigh          int `yaml:"max_high"`
	PolicyViolations int `yaml:"policy_violations"`
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

// This file owns AUR-550's own behavior on the SHARED SsorDtrackConfig
// type (qualitygates.go): the Dependency-Track upload/gate half of the
// section (enabled, server_api_host, api_key_secret, project_id_secret,
// thresholds, timeout_seconds, poll_interval_seconds). It never touches
// SBOMGenerator's own fields or Validate/Declared (AUR-549's own,
// unrelated behavior on the same struct) beyond reading
// SBOMGenerator.OutputFile as the path to the already-generated SBOM this
// card uploads as-is -- generating that file is this card's own
// Non-goal.
//
// Host, project and API key never come from a literal in this program --
// Host comes from this config (itself precedence-resolved by
// ApplyCentralPolicy, central.go), and APIKeySecret/ProjectIDSecret are
// only the NAMES of environment variables cmd/aurumcode reads at runtime
// (AC-005); the actual key and project id are never written anywhere in
// this repository.
//
// Every method below has a pointer receiver and is nil-safe (a nil
// *SsorDtrackConfig reports Declared() == false and defaults everything
// else): QualityGatesConfig.SsorDtrack is nil whenever the section is
// absent from the yaml entirely, which this package's callers must be
// able to pass straight through without a separate existence check.

// Declared reports whether AUR-550's own half of this section was
// actually turned on. Unlike GateConfig.Declared (any field counts),
// this card's own gate is opt-in by its own "enabled" key alone -- a
// repository or policy that sets server_api_host/secrets ahead of
// flipping enabled:true changes nothing yet, which keeps a staged
// rollout inert until the operator is ready. A nil receiver (the section
// absent entirely) is never declared. It does not report anything about
// SBOMGenerator's own Declared() -- the two halves are independent.
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
// sbom_generator.output_file -- "" when ssor_dtrack itself is absent,
// which the gate hook (cmd/aurumcode) turns into its own "SBOM
// unavailable" inconclusive reason, never a panic and never an upload of
// an empty/wrong file. SBOMGenerator is a value (never nil) once
// ssor_dtrack itself is declared, so no second nil check is needed on it.
func (g *SsorDtrackConfig) SBOMOutputFile() string {
	if g == nil {
		return ""
	}
	return g.SBOMGenerator.OutputFile
}

// Validate normalizes and validates AUR-550's own fields on this shared
// section, exactly like GateConfig.Validate: a malformed ssor_dtrack
// section is a loud parse-time error, never a silently-inert or
// silently-dangerous configuration. A nil receiver or a section left
// disabled (Declared() == false) is never validated at all -- an
// operator staging host/secret names ahead of enabling it must not be
// blocked by an incomplete draft. SBOMGenerator's own fields are AUR-549's
// own Validate (qualitygates.go), called independently from `aurumcode
// sbom`'s own entry point -- this function never re-derives that check.
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
