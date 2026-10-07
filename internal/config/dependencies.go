package config

import (
	"fmt"
	"net/url"
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
	// FailOn lists the advisory severities that fail the check for a
	// vulnerability the pull request introduces; the lowest one is the
	// threshold, as gate.fail_on. Empty: informative only.
	FailOn []string `yaml:"fail_on"`
	// Preexisting is what a vulnerability present on both sides does:
	// warn (the default) passes with an alert, block fails.
	Preexisting string `yaml:"preexisting"`
	// LicensesDenied lists SPDX identifiers no new or updated dependency may
	// be licensed under (evaluated against the SPDX expression).
	LicensesDenied []string `yaml:"licenses_denied"`
	// SuspicionSeverity is the severity of a grounded typosquat suspicion.
	SuspicionSeverity string `yaml:"suspicion_severity"`
	// OSVURL and DepsDevURL override the public sources (a mirror).
	OSVURL     string `yaml:"osv_url"`
	DepsDevURL string `yaml:"deps_dev_url"`
	// MaxSourceAgeHours, when set, bounds how old the advisory answer may be
	// (its Date and Age headers); an older or undated answer is
	// inconclusive.
	MaxSourceAgeHours *int `yaml:"max_source_age_hours"`
	// Scanner is the extraction scanner binary used as a cross-check of the
	// model's extraction; empty means "osv-scanner".
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

// Threshold is the lowest rank of FailOn; ok is false when FailOn is empty.
func (c *DependenciesConfig) Threshold() (rank GateSeverityRank, ok bool, err error) {
	if c == nil || len(c.FailOn) == 0 {
		return 0, false, nil
	}
	for _, level := range c.FailOn {
		r, _, err := NormalizeGateSeverity(level)
		if err != nil {
			return 0, false, fmt.Errorf("dependencies.fail_on: %w", err)
		}
		if rank == 0 || r < rank {
			rank = r
		}
	}
	return rank, true, nil
}

// Validate refuses what would silently weaken the check: an unknown
// severity, an unknown preexisting mode, a non-https source, an empty
// license identifier or an out-of-range age. A nil section is valid.
func (c *DependenciesConfig) Validate() error {
	if c == nil {
		return nil
	}
	if _, _, err := c.Threshold(); err != nil {
		return err
	}
	if _, _, err := NormalizeGateSeverity(c.EffectiveSuspicionSeverity()); err != nil {
		return fmt.Errorf("dependencies.suspicion_severity: %w", err)
	}
	if p := c.EffectivePreexisting(); p != PreexistingWarn && p != PreexistingBlock {
		return fmt.Errorf("dependencies.preexisting: %q is not warn|block", c.Preexisting)
	}
	for key, raw := range map[string]string{"osv_url": c.OSVURL, "deps_dev_url": c.DepsDevURL} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("dependencies.%s: %q is not an http(s) URL", key, raw)
		}
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
