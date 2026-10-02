package config

import (
	"strings"
	"testing"
)

func validException() ExceptionConfig {
	return ExceptionConfig{
		Repo:    "org/repo",
		Rule:    "seguranca.md#sql-injection",
		Path:    "legacy/report.py",
		Owner:   "time-seguranca",
		Reason:  "consulta fixa, sem entrada do usuario",
		Expires: "2026-12-31",
	}
}

// TestAUR520ExceptionConfigValidateRequiresEveryField covers AC-005: any
// of repo/rule/path/owner/reason/expires missing fails closed, naming the
// field, never a silently-permissive exception.
func TestAUR520ExceptionConfigValidateRequiresEveryField(t *testing.T) {
	base := validException()
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() on a complete exception = %v, want nil", err)
	}

	clear := func(mutate func(*ExceptionConfig)) ExceptionConfig {
		e := validException()
		mutate(&e)
		return e
	}
	cases := map[string]ExceptionConfig{
		"repo":    clear(func(e *ExceptionConfig) { e.Repo = "" }),
		"rule":    clear(func(e *ExceptionConfig) { e.Rule = "" }),
		"path":    clear(func(e *ExceptionConfig) { e.Path = "" }),
		"owner":   clear(func(e *ExceptionConfig) { e.Owner = "" }),
		"reason":  clear(func(e *ExceptionConfig) { e.Reason = "" }),
		"expires": clear(func(e *ExceptionConfig) { e.Expires = "" }),
	}
	for name, e := range cases {
		if err := e.Validate(); err == nil {
			t.Fatalf("Validate() with %s blank = nil error, want a fail-closed error", name)
		}
	}
}

// TestAUR520ExceptionConfigValidateRejectsMalformedExpires proves a
// timezone-bearing or wrongly-shaped expires (RFC3339, a slash date, a
// trailing offset) is refused at config load -- MUT-001's own target:
// accepting a non-YYYY-MM-DD spelling here would let the match code's
// date comparison be bent by a format trick.
func TestAUR520ExceptionConfigValidateRejectsMalformedExpires(t *testing.T) {
	for _, bad := range []string{
		"2026-12-31T00:00:00Z",
		"2026-12-31T00:00:00-05:00",
		"31/12/2026",
		"2026/12/31",
		"not-a-date",
		"",
	} {
		e := validException()
		e.Expires = bad
		if err := e.Validate(); err == nil {
			t.Fatalf("Validate() with expires=%q = nil error, want a fail-closed error", bad)
		}
	}
}

// TestAUR520ExceptionConfigExpiresOnIsUTCMidnight proves ExpiresOn parses
// the calendar date as UTC midnight with no time-of-day component, the
// exact normalization matchException's expiry comparison depends on.
func TestAUR520ExceptionConfigExpiresOnIsUTCMidnight(t *testing.T) {
	e := validException()
	got, err := e.ExpiresOn()
	if err != nil {
		t.Fatalf("ExpiresOn() error = %v", err)
	}
	if got.Year() != 2026 || got.Month() != 12 || got.Day() != 31 {
		t.Fatalf("ExpiresOn() = %v, want 2026-12-31", got)
	}
	if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 {
		t.Fatalf("ExpiresOn() = %v, want midnight", got)
	}
	if zone, _ := got.Zone(); zone != "UTC" {
		t.Fatalf("ExpiresOn() zone = %q, want UTC", zone)
	}
}

// TestAUR520ValidateExceptionsNamesTheFailingIndex proves a config with
// several exceptions still points at the exact one that is broken.
func TestAUR520ValidateExceptionsNamesTheFailingIndex(t *testing.T) {
	list := []ExceptionConfig{validException(), {Repo: "org/repo", Rule: "x", Path: "y"}}
	err := ValidateExceptions(list)
	if err == nil {
		t.Fatal("ValidateExceptions() = nil, want an error naming exceptions[1]")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "exceptions[1]") {
		t.Fatalf("ValidateExceptions() error = %q, want it to name exceptions[1]", got)
	}
}

// TestAUR520ParseRejectsMalformedException proves Parse itself (the
// config-load path LoadCentralPolicy and config.Load both share) refuses
// a malformed exceptions entry before any model call, exactly like every
// other section (AC-005).
func TestAUR520ParseRejectsMalformedException(t *testing.T) {
	yaml := "exceptions:\n  - repo: org/repo\n    rule: x\n    path: y\n    owner: time\n    reason: motivo\n    expires: not-a-date\n"
	if _, err := Parse([]byte(yaml), "test.yml"); err == nil {
		t.Fatal("Parse() with a malformed exception = nil error, want fail-closed")
	}
}

// TestAUR520ParseAcceptsValidExceptions proves a well-formed exceptions
// list round-trips through Parse untouched.
func TestAUR520ParseAcceptsValidExceptions(t *testing.T) {
	yaml := "exceptions:\n  - repo: org/repo\n    rule: seguranca.md#sql-injection\n    path: legacy/report.py\n    owner: time-seguranca\n    reason: consulta fixa\n    expires: 2026-12-31\n"
	cfg, err := Parse([]byte(yaml), "test.yml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(cfg.Exceptions) != 1 || cfg.Exceptions[0].Repo != "org/repo" {
		t.Fatalf("Parse() exceptions = %+v, want one entry for org/repo", cfg.Exceptions)
	}
}

// TestAUR520ApplyCentralPolicyException covers AC-004: under a central
// policy, only the policy's own Exceptions ever apply; a repository's own
// exception (even one that would otherwise match a policy-governed
// finding exactly) is dropped with a named warning, never merged with the
// policy's list.
func TestAUR520ApplyCentralPolicyException(t *testing.T) {
	repoExc := ExceptionConfig{Repo: "org/repo", Rule: "security/hardcoded-secret", Path: "a.go", Owner: "dev", Reason: "repo accepted this", Expires: "2099-01-01"}
	policyExc := validException()
	repo := &Config{Exceptions: []ExceptionConfig{repoExc}}
	central := &Config{Exceptions: []ExceptionConfig{policyExc}}

	effective, warnings := ApplyCentralPolicy(repo, central)
	if len(effective.Exceptions) != 1 || effective.Exceptions[0].Rule != policyExc.Rule {
		t.Fatalf("effective.Exceptions = %+v, want only the policy's own", effective.Exceptions)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w.Reason, repoExc.Rule) && strings.Contains(w.Reason, repoExc.Path) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning naming the dropped repo exception, got %+v", warnings)
	}

	// Without a policy, the repo's own exceptions are its own, unchanged.
	effective, warnings = ApplyCentralPolicy(repo, nil)
	if warnings != nil {
		t.Fatalf("no policy: warnings = %+v, want nil", warnings)
	}
	if len(effective.Exceptions) != 1 || effective.Exceptions[0].Rule != repoExc.Rule {
		t.Fatalf("no policy: effective.Exceptions = %+v, want the repo's own unchanged", effective.Exceptions)
	}
}
