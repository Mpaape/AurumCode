package config

import (
	"fmt"
	"regexp"
)

const (
	// DefaultAnalysisDataMaxAgeDays: the publishing workflow runs daily, so a
	// week tolerates failed runs and a long weekend without letting
	// vulnerability data rot longer than that.
	DefaultAnalysisDataMaxAgeDays = 7
	// MaxAnalysisDataMaxAgeDays stops a typo from silently disabling the limit.
	MaxAnalysisDataMaxAgeDays = 365
	// DefaultAnalysisDataRepository publishes the analysis-data releases.
	DefaultAnalysisDataRepository = "Mpaape/AurumCode"
)

// AnalysisDataConfig is `analysis_data` (AUR-533): how old the published
// analysis-data artifact (OSV copy, scanner pins) may be before a review is
// inconclusive. It is a pointer in Config: nil means "not declared" and the
// review never consults the artifact. Declare it as a mapping (`{}` for all
// defaults); a bare `analysis_data:` YAML null reads as not declared.
type AnalysisDataConfig struct {
	// MaxAgeDays is 1..365; 0 (absent) means the default.
	MaxAgeDays int `yaml:"max_age_days"`
	// Repository is the owner/name publishing the artifact; empty means the
	// default. It is governed by the same per-section precedence as the rest.
	Repository string `yaml:"repository"`
}

var analysisDataRepoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// Declared reports whether the section was written at all (nil-safe).
func (c *AnalysisDataConfig) Declared() bool { return c != nil }

// EffectiveMaxAgeDays applies the default for a declared section.
func (c *AnalysisDataConfig) EffectiveMaxAgeDays() int {
	if c == nil || c.MaxAgeDays == 0 {
		return DefaultAnalysisDataMaxAgeDays
	}
	return c.MaxAgeDays
}

// EffectiveRepository applies the default for a declared section.
func (c *AnalysisDataConfig) EffectiveRepository() string {
	if c == nil || c.Repository == "" {
		return DefaultAnalysisDataRepository
	}
	return c.Repository
}

// Validate rejects an out-of-range age (never "no limit") or a malformed
// repository. A nil section is valid.
func (c *AnalysisDataConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.MaxAgeDays < 0 || c.MaxAgeDays > MaxAnalysisDataMaxAgeDays {
		return fmt.Errorf("analysis_data.max_age_days: %d out of range (1..%d)", c.MaxAgeDays, MaxAnalysisDataMaxAgeDays)
	}
	if c.Repository != "" && !analysisDataRepoRe.MatchString(c.Repository) {
		return fmt.Errorf("analysis_data.repository: %q is not owner/name", c.Repository)
	}
	return nil
}
