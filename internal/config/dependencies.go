package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// The public sources the dependency check consults when the section does not
// name its own: the OSV API for advisories and deps.dev for registry metadata
// and licenses. Both are public, documented endpoints, overridable per policy.
const (
	DefaultOSVURL     = "https://api.osv.dev"
	DefaultDepsDevURL = "https://api.deps.dev"
	// DefaultSuspicionSeverity is the severity a grounded typosquat or
	// malicious-package suspicion counts with when the section is silent.
	DefaultSuspicionSeverity = "high"
	// MaxDependencySourceAgeHours stops a typo from disabling the freshness
	// limit of the advisory source.
	MaxDependencySourceAgeHours = 24 * 365
)

// The closed vocabulary of dependencies.preexisting.
const (
	PreexistingWarn  = "warn"
	PreexistingBlock = "block"
)

// DependenciesConfig is the `dependencies` section: the dependency check of
// a pull request (AUR-495) and its gate (AUR-527..529). It is a pointer in
// Config: nil means "not declared" and no dependency check runs, so a
// configuration without it behaves exactly as before. Declared without
// fail_on, the check runs and its findings are informative only.
type DependenciesConfig struct {
	// FailOn lists advisory severities (critical, high, medium, low); the
	// lowest one listed is the threshold, as gate.fail_on: an introduced
	// vulnerability at or above it fails, one below alerts. Empty:
	// informative only.
	FailOn []string `yaml:"fail_on"`
	// Preexisting is what a vulnerability present on both sides does:
	// warn (the default) passes with an alert, block fails.
	Preexisting string `yaml:"preexisting"`
	// LicensesDenied lists SPDX identifiers no new or updated dependency may
	// be licensed under (evaluated against the SPDX expression).
	LicensesDenied []string `yaml:"licenses_denied"`
	// SuspicionSeverity is the severity of a grounded typosquat suspicion.
	SuspicionSeverity string `yaml:"suspicion_severity"`
	// OSVURL and DepsDevURL override the public sources (a mirror): https,
	// or http on a loopback host only.
	OSVURL     string `yaml:"osv_url"`
	DepsDevURL string `yaml:"deps_dev_url"`
	// MaxSourceAgeHours, when set, bounds how old the advisory answer may be
	// (its Date and Age headers); an older or undated answer is
	// inconclusive.
	MaxSourceAgeHours *int `yaml:"max_source_age_hours"`
	// Scanner is the extraction scanner binary used as a cross-check of the
	// model's extraction: a command name from PATH or an absolute path, never
	// a relative path (it would resolve inside the reviewed checkout); empty
	// means "osv-scanner".
	Scanner string `yaml:"scanner"`
}

// Declared reports whether the section was written (nil-safe).
func (c *DependenciesConfig) Declared() bool { return c != nil }

// EffectiveOSVURL applies the default.
func (c *DependenciesConfig) EffectiveOSVURL() string {
	if c == nil || strings.TrimSpace(c.OSVURL) == "" {
		return DefaultOSVURL
	}
	return strings.TrimRight(strings.TrimSpace(c.OSVURL), "/")
}

// EffectiveDepsDevURL applies the default.
func (c *DependenciesConfig) EffectiveDepsDevURL() string {
	if c == nil || strings.TrimSpace(c.DepsDevURL) == "" {
		return DefaultDepsDevURL
	}
	return strings.TrimRight(strings.TrimSpace(c.DepsDevURL), "/")
}

// EffectiveScanner applies the default scanner binary.
func (c *DependenciesConfig) EffectiveScanner() string {
	if c == nil || strings.TrimSpace(c.Scanner) == "" {
		return "osv-scanner"
	}
	return strings.TrimSpace(c.Scanner)
}

// EffectivePreexisting applies the default (warn).
func (c *DependenciesConfig) EffectivePreexisting() string {
	if c == nil || strings.TrimSpace(c.Preexisting) == "" {
		return PreexistingWarn
	}
	return strings.ToLower(strings.TrimSpace(c.Preexisting))
}

// EffectiveSuspicionSeverity applies the default.
func (c *DependenciesConfig) EffectiveSuspicionSeverity() string {
	if c == nil || strings.TrimSpace(c.SuspicionSeverity) == "" {
		return DefaultSuspicionSeverity
	}
	return strings.TrimSpace(c.SuspicionSeverity)
}

// SeverityUnknown is an advisory severity the source did not give.
const SeverityUnknown = "unknown"

// NormalizeDependencySeverity reads an advisory severity word (GitHub's
// LOW/MODERATE/HIGH/CRITICAL, or the gate's own spellings) as
// critical/high/medium/low; anything else is SeverityUnknown.
func NormalizeDependencySeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical":
		return "critical"
	case "high", "error":
		return "high"
	case "moderate", "medium", "warning":
		return "medium"
	case "low", "info":
		return "low"
	default:
		return SeverityUnknown
	}
}

// Gated reports a declared fail_on.
func (c *DependenciesConfig) Gated() bool { return c != nil && len(c.FailOn) > 0 }

// dependencySeverityRanks is the advisory severity ladder: unlike the
// gate's three levels, critical and high are distinct ranks.
var dependencySeverityRanks = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

// Fails reports whether an advisory of severity fails the check: its rank
// is at or above the lowest level listed in fail_on, the same "this level
// and above" reading as gate.fail_on. An unknown severity fails whenever
// fail_on is declared: a severity that cannot be read never lets a finding
// pass.
func (c *DependenciesConfig) Fails(severity string) bool {
	if !c.Gated() {
		return false
	}
	sev := NormalizeDependencySeverity(severity)
	if sev == SeverityUnknown {
		return true
	}
	threshold := 0
	for _, level := range c.FailOn {
		if r := dependencySeverityRanks[NormalizeDependencySeverity(level)]; r > 0 && (threshold == 0 || r < threshold) {
			threshold = r
		}
	}
	// No readable level in a declared fail_on (Validate refuses it; a value
	// that bypassed it) fails closed rather than letting everything pass.
	return threshold == 0 || dependencySeverityRanks[sev] >= threshold
}

// Validate refuses what would silently weaken the check: an unknown
// severity, an unknown preexisting mode, a non-https source, an empty
// license identifier or an out-of-range age. A nil section is valid.
func (c *DependenciesConfig) Validate() error {
	if c == nil {
		return nil
	}
	for _, level := range c.FailOn {
		if NormalizeDependencySeverity(level) == SeverityUnknown {
			return fmt.Errorf("dependencies.fail_on: unknown severity %q (accepted: critical, high, medium, low)", level)
		}
	}
	if NormalizeDependencySeverity(c.EffectiveSuspicionSeverity()) == SeverityUnknown {
		return fmt.Errorf("dependencies.suspicion_severity: unknown severity %q (accepted: critical, high, medium, low)", c.SuspicionSeverity)
	}
	if p := c.EffectivePreexisting(); p != PreexistingWarn && p != PreexistingBlock {
		return fmt.Errorf("dependencies.preexisting: %q is not warn|block", c.Preexisting)
	}
	for key, raw := range map[string]string{"osv_url": c.OSVURL, "deps_dev_url": c.DepsDevURL} {
		if err := validateSourceURL(raw); err != nil {
			return fmt.Errorf("dependencies.%s: %w", key, err)
		}
	}
	if err := validateScannerBinary(c.Scanner); err != nil {
		return fmt.Errorf("dependencies.scanner: %w", err)
	}
	for _, id := range c.LicensesDenied {
		if strings.TrimSpace(id) == "" || strings.ContainsAny(id, " ()") {
			return fmt.Errorf("dependencies.licenses_denied: %q is not one SPDX identifier", id)
		}
	}
	if c.MaxSourceAgeHours != nil && (*c.MaxSourceAgeHours < 1 || *c.MaxSourceAgeHours > MaxDependencySourceAgeHours) {
		return fmt.Errorf("dependencies.max_source_age_hours: %d out of range (1..%d)", *c.MaxSourceAgeHours, MaxDependencySourceAgeHours)
	}
	return nil
}

// validateSourceURL accepts an https URL; plain http only for a loopback
// host (a local mirror or a test server), never across a network where the
// advisory answer could be altered in transit.
func validateSourceURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a URL", raw)
	}
	switch {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && loopbackHost(u.Hostname()):
		return nil
	}
	return fmt.Errorf("%q must be https (http only for 127.0.0.1, ::1 or localhost)", raw)
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validateScannerBinary accepts a bare command name (looked up in PATH) or
// an absolute path. A relative path would resolve from the checkout under
// review, which the pull request's author controls: it is refused.
func validateScannerBinary(raw string) error {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return nil
	case filepath.IsAbs(raw):
		return nil
	case strings.ContainsAny(raw, `/\`), strings.HasPrefix(raw, "-"), raw == ".", raw == "..":
		return fmt.Errorf("%q must be a command name from PATH or an absolute path", raw)
	}
	return nil
}

// governDependencies: a policy that declares dependencies decides alone; a
// policy silent on it leaves the repository's.
func governDependencies(effective, repo, central *Config) []ProviderWarning {
	if central.Dependencies == nil {
		return nil
	}
	effective.Dependencies = central.Dependencies
	if repo.Dependencies == nil {
		return nil
	}
	return []ProviderWarning{ignoredWarning("dependencies do config do repositório foi ignorado: a política central decide sozinha")}
}
