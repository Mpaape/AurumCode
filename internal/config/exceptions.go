// AUR-520: an approved exception removes one EXACT finding from the
// AUR-519 policy gate -- a false positive or an accepted risk a human
// (Owner) already signed off, with a Reason and an Expires date, never a
// whole rule or an entire repository (see the card's own Non-goals: no
// blanket, org-wide exception for a rule with no path). This file owns
// only the configuration shape and its fail-closed validation; the actual
// repo/rule/path/expiry comparison against a finding runs in
// cmd/aurumcode (matchException, aur520.go) alongside the gate it
// modifies -- see that file's own doc comment for why the match stays
// there instead of here.
package config

import (
	"fmt"
	"strings"
	"time"
)

// ExceptionConfig is one approved exception. Every field is required
// (Validate fails closed on any blank one, AC-005): Repo, Rule and Path
// together name the one finding this exception covers -- exact strings,
// never a glob, so an exception can never accidentally widen to cover
// more than the human who wrote it reviewed (see docs/configuration.md's
// "Exceções" section for why `path` stays exact). Owner, Reason and
// Expires are the accountability AUR-520's outcome requires of "falso
// positivo ou risco aceito": who accepted it, why, and until when.
type ExceptionConfig struct {
	Repo    string `yaml:"repo"`
	Rule    string `yaml:"rule"`
	Path    string `yaml:"path"`
	Owner   string `yaml:"owner"`
	Reason  string `yaml:"reason"`
	Expires string `yaml:"expires"`
}

// ExceptionDateLayout is the one accepted spelling of `expires`: a plain
// calendar date with no time-of-day and no timezone offset. Accepting
// only this layout at config-load time (Validate) means the expiry
// comparison itself (cmd/aurumcode's matchException) never has to guess a
// caller's intent from an RFC3339 string, a slash date, or a trailing
// "Z"/offset -- every one of those is refused here, loudly, instead of
// silently misread later (MUT-001's own target: a format trick must not
// be a way to dodge expiry).
const ExceptionDateLayout = "2006-01-02"

// Validate fails closed when Repo, Rule, Path, Owner, Reason or Expires is
// blank, or when Expires does not parse under ExceptionDateLayout exactly.
// Repo/Rule/Path being required (not only Owner/Reason/Expires) matters
// for the same reason the match code treats an unverifiable repo identity
// as "" never matching anything: an exception with a blank field could
// otherwise coincide with a finding whose own corresponding value happens
// to be empty or unknown, and match far more broadly than the one,
// human-reviewed finding this exception is meant to cover.
func (e ExceptionConfig) Validate() error {
	if strings.TrimSpace(e.Repo) == "" {
		return fmt.Errorf("repo is required")
	}
	if strings.TrimSpace(e.Rule) == "" {
		return fmt.Errorf("rule is required")
	}
	if strings.TrimSpace(e.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if strings.TrimSpace(e.Owner) == "" {
		return fmt.Errorf("owner is required (exception %s %s)", e.Rule, e.Path)
	}
	if strings.TrimSpace(e.Reason) == "" {
		return fmt.Errorf("reason is required (exception %s %s)", e.Rule, e.Path)
	}
	expires := strings.TrimSpace(e.Expires)
	if expires == "" {
		return fmt.Errorf("expires is required (exception %s %s)", e.Rule, e.Path)
	}
	if _, err := time.Parse(ExceptionDateLayout, expires); err != nil {
		return fmt.Errorf("expires %q (exception %s %s) must be YYYY-MM-DD: %w", e.Expires, e.Rule, e.Path, err)
	}
	return nil
}

// ExpiresOn parses Expires as a UTC calendar date (midnight UTC, no
// time-of-day component at all) -- callers must only call it once
// Validate has already accepted the layout; LoadCentralPolicy and Parse
// both guarantee that before an ExceptionConfig ever reaches
// cmd/aurumcode.
func (e ExceptionConfig) ExpiresOn() (time.Time, error) {
	return time.ParseInLocation(ExceptionDateLayout, strings.TrimSpace(e.Expires), time.UTC)
}

// ValidateExceptions runs Validate over every entry, naming the index of
// the first failing one so a config declaring several exceptions still
// points at the exact one a human needs to fix, exactly like Parse's own
// per-section config errors elsewhere in this package.
func ValidateExceptions(list []ExceptionConfig) error {
	for i, exc := range list {
		if err := exc.Validate(); err != nil {
			return fmt.Errorf("exceptions[%d]: %w", i, err)
		}
	}
	return nil
}
