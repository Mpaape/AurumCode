package main

// AUR-518 behavior proof: a central policy, when declared with --politica
// (or AURUMCODE_POLICY), takes exclusive authority over a repository's own
// rules and ignore patterns, while the repository's own context (skills)
// and presentation settings keep adding alongside the policy's. These tests
// drive the real `review` command through the offline fixture provider,
// reusing coverageFixture (aur476_test.go) for the repository side of the
// fixture and a small sibling helper for the policy side.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// policyFixture builds a standalone central policy directory: its own
// .aurumcode/config.yml, plus an optional skill file. It deliberately is
// NOT a git repository -- LoadCentralPolicy only ever reads plain files.
func policyFixture(t *testing.T, configYAML string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".aurumcode", "config.yml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(configYAML), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

const aur518FixtureResponse = `{"summary":"reviewed","issues":[]}`

// setAUR518LLMFixture points AURUMCODE_LLM_FIXTURE at a minimal, valid
// offline response so the model half of the review always answers the same
// way; every assertion in this file is about the deterministic rule/ignore
// pipeline, never about model content.
func setAUR518LLMFixture(t *testing.T) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(aur518FixtureResponse), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
}

const hardcodedSecretMessage = "Hardcoded secret or credential assigned inline"

// TestAUR518PolicyKeepsRuleDespiteRepoDisable covers AC-001: with a central
// policy active, the repository's "rules.<id>.enabled: false" is ignored --
// the deterministic analysis/hardcoded-secret finding still appears -- and
// the terminal names the overridden rule.
func TestAUR518PolicyKeepsRuleDespiteRepoDisable(t *testing.T) {
	coverageFixture(t, "rules:\n  analysis/hardcoded-secret:\n    enabled: false\n")
	setAUR518LLMFixture(t)
	policyDir := policyFixture(t, "")

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), hardcodedSecretMessage) {
		t.Fatalf("expected the policy-protected finding to survive the repo's disable:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "analysis/hardcoded-secret") {
		t.Fatalf("expected a terminal warning naming the overridden rule:\n%s", errOut.String())
	}
}

// TestAUR518PolicySeverityOverrideIgnored covers AC-002: a repository
// severity override for a policy-governed rule is dropped -- only the
// catalog's own severity is published -- and the warning names the rule.
func TestAUR518PolicySeverityOverrideIgnored(t *testing.T) {
	coverageFixture(t, "rules:\n  analysis/hardcoded-secret:\n    severity: info\n")
	setAUR518LLMFixture(t)
	policyDir := policyFixture(t, "")

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "[error] "+hardcodedSecretMessage) {
		t.Fatalf("expected the catalog's own severity (error), not the repo's override:\n%s", out.String())
	}
	if strings.Contains(out.String(), "[info] "+hardcodedSecretMessage) {
		t.Fatalf("the repo's severity override must not reach the published finding:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "analysis/hardcoded-secret") {
		t.Fatalf("expected a terminal warning naming the overridden rule:\n%s", errOut.String())
	}
}

// TestAUR518PolicyIgnoreWinsOverRepo covers AC-003: with a central policy
// active, only the policy's own `ignore` filters the diff -- the
// repository's ignore pattern is not applied, and the dropped pattern is
// named in the warning.
func TestAUR518PolicyIgnoreWinsOverRepo(t *testing.T) {
	coverageFixture(t, "ignore:\n  - \"tests/**\"\n")
	setAUR518LLMFixture(t)
	policyDir := policyFixture(t, "")

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String(), "Review coverage") {
		t.Fatalf("the policy declares no ignore list, so nothing should be hidden from coverage:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "tests/**") {
		t.Fatalf("expected a terminal warning naming the repo's dropped ignore pattern:\n%s", errOut.String())
	}
}

// TestAUR518PolicyAndRepoSkillsBothReachPrompt covers AC-004: the policy's
// own skill and the repository's own skill both reach the outbound model
// prompt, additively, with the policy's contribution first.
func TestAUR518PolicyAndRepoSkillsBothReachPrompt(t *testing.T) {
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/repo-skill.md\n")
	setAUR518LLMFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "repo-skill.md"), []byte("REPO-SKILL-MARKER"), 0600); err != nil {
		t.Fatal(err)
	}
	policyDir := policyFixture(t, "review:\n  context:\n    skills:\n      - skills/policy-skill.md\n")
	if err := os.MkdirAll(filepath.Join(policyDir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(policyDir, "skills", "policy-skill.md"), []byte("POLICY-SKILL-MARKER"), 0600); err != nil {
		t.Fatal(err)
	}

	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	captured, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	prompt := string(captured)
	policyIdx := strings.Index(prompt, "POLICY-SKILL-MARKER")
	repoIdx := strings.Index(prompt, "REPO-SKILL-MARKER")
	if policyIdx == -1 {
		t.Fatalf("policy skill did not reach the prompt:\n%s", prompt)
	}
	if repoIdx == -1 {
		t.Fatalf("repository skill did not reach the prompt:\n%s", prompt)
	}
	if policyIdx > repoIdx {
		t.Fatalf("expected the policy's skill before the repository's in the prompt (policy at %d, repo at %d)", policyIdx, repoIdx)
	}
}

// TestAUR518MissingOrInvalidPolicyFailsClosed covers AC-005: a declared
// policy that is missing, invalid, or names a missing context file fails
// the command (non-zero exit) before any model call.
func TestAUR518MissingOrInvalidPolicyFailsClosed(t *testing.T) {
	cases := []struct {
		name      string
		policyDir func(t *testing.T) string
	}{
		{
			name: "missing config.yml",
			policyDir: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name: "invalid yaml",
			policyDir: func(t *testing.T) string {
				return policyFixture(t, "review:\n  language: not-a-real-language\n")
			},
		},
		{
			name: "missing listed skill",
			policyDir: func(t *testing.T) string {
				return policyFixture(t, "review:\n  context:\n    skills:\n      - skills/missing.md\n")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			coverageFixture(t, "")
			setAUR518LLMFixture(t)
			capture := filepath.Join(t.TempDir(), "prompt.txt")
			t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
			policyDir := tc.policyDir(t)

			var out, errOut strings.Builder
			code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter())
			if code == 0 {
				t.Fatalf("expected a non-zero exit for a %s policy, got 0; stdout=%s stderr=%s", tc.name, out.String(), errOut.String())
			}
			if errOut.String() == "" {
				t.Fatal("expected an explanatory error on stderr")
			}
			if _, err := os.Stat(capture); err == nil {
				t.Fatal("expected no model call (no captured prompt) when the policy fails to load")
			}
		})
	}
}

// TestAUR518NoPolicyKeepsRepoRuleOverride covers AC-006: with no policy
// declared (no --politica flag, no AURUMCODE_POLICY), the repository's own
// rule override still applies exactly as it did before this card existed,
// and no policy warning is produced.
func TestAUR518NoPolicyKeepsRepoRuleOverride(t *testing.T) {
	coverageFixture(t, "rules:\n  analysis/hardcoded-secret:\n    enabled: false\n")
	setAUR518LLMFixture(t)
	t.Setenv("AURUMCODE_POLICY", "")

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String(), hardcodedSecretMessage) {
		t.Fatalf("without a policy, the repo's own rule disable must still apply (AC-006):\n%s", out.String())
	}
	if strings.Contains(errOut.String(), "politica central") {
		t.Fatalf("no policy was declared; expected no policy warning:\n%s", errOut.String())
	}
}
