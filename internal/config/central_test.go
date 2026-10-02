package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writePolicyFile writes a file under dir, creating parent directories.
func writePolicyFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestAUR518LoadCentralPolicyMissingFailsClosed covers AC-005: a policy
// directory with no .aurumcode/config.yml at all is a loud error naming the
// path, never a silently-empty, permissive policy.
func TestAUR518LoadCentralPolicyMissingFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadCentralPolicy(dir); err == nil {
		t.Fatal("expected an error for a policy directory with no config.yml")
	}
}

// TestAUR518LoadCentralPolicyInvalidYAMLFailsClosed covers AC-005's other
// half: a config.yml that exists but does not parse is also refused, never
// treated as zero-config.
func TestAUR518LoadCentralPolicyInvalidYAMLFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, DefaultConfigPath, "review:\n  language: not-a-real-language\n")
	if _, err := LoadCentralPolicy(dir); err == nil {
		t.Fatal("expected an error for an invalid policy config.yml")
	}
}

// TestAUR518LoadCentralPolicyMissingSkillFailsClosed covers AC-005: a
// policy that lists a skill file which does not exist under its own root is
// refused, naming the missing file, before any model call.
func TestAUR518LoadCentralPolicyMissingSkillFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, DefaultConfigPath, "review:\n  context:\n    skills:\n      - skills/security.md\n")
	_, err := LoadCentralPolicy(dir)
	if err == nil {
		t.Fatal("expected an error for a policy whose listed skill is missing")
	}
	if !containsAll(err.Error(), "skills/security.md") {
		t.Fatalf("error does not name the missing skill: %v", err)
	}
}

// TestAUR518LoadCentralPolicyValid covers the happy path: a well-formed
// policy with an existing skill loads cleanly.
func TestAUR518LoadCentralPolicyValid(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, DefaultConfigPath, "rules:\n  security/hardcoded-secret:\n    enabled: true\nignore:\n  - \"vendor/**\"\nreview:\n  context:\n    skills:\n      - skills/security.md\n")
	writePolicyFile(t, dir, "skills/security.md", "# security skill\n")
	cfg, err := LoadCentralPolicy(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.Rules["security/hardcoded-secret"]; !ok {
		t.Fatal("expected the policy's rule override to be present")
	}
	if len(cfg.Ignore) != 1 || cfg.Ignore[0] != "vendor/**" {
		t.Fatalf("unexpected ignore list: %v", cfg.Ignore)
	}
}

func containsAll(haystack string, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestAUR518ApplyCentralPolicyNilIsUnchanged covers AC-006: with no policy,
// ApplyCentralPolicy returns the exact same *Config pointer and nil
// warnings, so the effective config is byte-identical to this card never
// having run.
func TestAUR518ApplyCentralPolicyNilIsUnchanged(t *testing.T) {
	repo := &Config{Ignore: []string{"tests/**"}, Rules: map[string]RuleConfig{"x": {}}}
	effective, warnings := ApplyCentralPolicy(repo, nil)
	if effective != repo {
		t.Fatal("expected the exact same *Config pointer when central is nil")
	}
	if warnings != nil {
		t.Fatalf("expected nil warnings when central is nil, got %v", warnings)
	}
}

// TestAUR518ApplyCentralPolicyRuleOverrideIgnored covers AC-001/AC-002: a
// repository's rule override (whether a disable or a severity change) is
// dropped in favor of the policy's own Rules, and one warning names the
// rule id.
func TestAUR518ApplyCentralPolicyRuleOverrideIgnored(t *testing.T) {
	disabled := false
	repo := &Config{Rules: map[string]RuleConfig{
		"security/hardcoded-secret": {Enabled: &disabled},
		"quality/unused-var":        {Severity: "info"},
	}}
	central := &Config{Rules: map[string]RuleConfig{}}

	effective, warnings := ApplyCentralPolicy(repo, central)
	if len(effective.Rules) != 0 {
		t.Fatalf("expected the policy's (empty) rule set to win, got %v", effective.Rules)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected one warning per overridden rule, got %d: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if w.Provider != "politica central" {
			t.Fatalf("unexpected warning provider %q", w.Provider)
		}
	}
	names := warnings[0].Reason + " " + warnings[1].Reason
	if !containsAll(names, "security/hardcoded-secret") || !containsAll(names, "quality/unused-var") {
		t.Fatalf("warnings do not name both overridden rules: %v", warnings)
	}
}

// TestAUR518ApplyCentralPolicyIgnoreOverrideDropped covers AC-003: the
// repository's own ignore patterns are dropped, only the policy's apply,
// and a warning names the dropped pattern.
func TestAUR518ApplyCentralPolicyIgnoreOverrideDropped(t *testing.T) {
	repo := &Config{Ignore: []string{"tests/**"}}
	central := &Config{Ignore: []string{"vendor/**"}}

	effective, warnings := ApplyCentralPolicy(repo, central)
	if len(effective.Ignore) != 1 || effective.Ignore[0] != "vendor/**" {
		t.Fatalf("expected only the policy's ignore list, got %v", effective.Ignore)
	}
	if len(warnings) != 1 || !containsAll(warnings[0].Reason, "tests/**") {
		t.Fatalf("expected one warning naming the dropped repo pattern, got %v", warnings)
	}
}

// TestAUR518ApplyCentralPolicyKeepsRepoContextAndLanguageFallback covers
// AC-004 and the Review.Language/Publication precedence rule: the repo's
// own context stays untouched, and an unset central language/publication
// leaves the repo's choice standing.
func TestAUR518ApplyCentralPolicyKeepsRepoContextAndLanguageFallback(t *testing.T) {
	repo := &Config{Review: ReviewConfig{
		Language:    "pt-BR",
		Publication: "review",
		Context:     ReviewContextConfig{Skills: []string{"skills/convention.md"}},
	}}
	central := &Config{}

	effective, _ := ApplyCentralPolicy(repo, central)
	if effective.Review.Language != "pt-BR" {
		t.Fatalf("expected the repo's language to survive an unset policy language, got %q", effective.Review.Language)
	}
	if effective.Review.Publication != "review" {
		t.Fatalf("expected the repo's publication to survive an unset policy publication, got %q", effective.Review.Publication)
	}
	if len(effective.Review.Context.Skills) != 1 || effective.Review.Context.Skills[0] != "skills/convention.md" {
		t.Fatalf("expected the repo's own context to stay untouched, got %v", effective.Review.Context)
	}
}

// TestAUR518ApplyCentralPolicyLanguageAndPublicationOverride covers the
// precedence direction when the policy DOES set these fields: the policy
// wins.
func TestAUR518ApplyCentralPolicyLanguageAndPublicationOverride(t *testing.T) {
	repo := &Config{Review: ReviewConfig{Language: "pt-BR", Publication: "comments"}}
	central := &Config{Review: ReviewConfig{Language: "en-US", Publication: "review"}}

	effective, _ := ApplyCentralPolicy(repo, central)
	if effective.Review.Language != "en-US" {
		t.Fatalf("expected the policy's language to win, got %q", effective.Review.Language)
	}
	if effective.Review.Publication != "review" {
		t.Fatalf("expected the policy's publication to win, got %q", effective.Review.Publication)
	}
}
