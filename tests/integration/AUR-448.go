// Package integration holds AUR-448's Integration-layer proof, at the CLI
// boundary: the real aurumcode binary, built from source, shows the
// complete fixture shape (rule_id included, with a real catalog id as the
// example) when no provider is configured, and prints a discard-warning
// line on stderr -- never stdout -- exactly when the rule gate (AUR-434)
// discarded one or more findings, leaving stdout byte-identical to the
// published AUR-434/AUR-426 contract in every case.
//
// This card's TDD proof section names the integration selector
// IntegrationAUR435; that collides with the function
// tests/integration/AUR-435.go already declares in this same package (a
// different card). See docs/specs/AUR-448.md's "A note on this card's own
// TDD-proof identifiers" for why this file declares IntegrationAUR448
// instead, following every sibling card's own numeral-matches-file
// convention (IntegrationAUR434, IntegrationAUR443, ...).
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur448IntegrationRoot resolves the repository root, exactly like
// tests/integration/AUR-434.go does: AURUMCODE_ROOT wins (the acceptance
// harness sets it to the staged materialization root), and a direct run
// from a full checkout climbs two directories back to the root.
func aur448IntegrationRoot(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

const aur448IntegrationMixedFixture = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "grounded"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "unknown rule_id"
    }
  ],
  "summary": "Mixed fixture for AUR-448 integration."
}`

const aur448IntegrationAllDiscardedFixture = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all"
    }
  ],
  "summary": "All-discarded fixture for AUR-448 integration."
}`

// IntegrationAUR448 builds the real aurumcode binary and proves this
// card's outcome at the CLI boundary.
func IntegrationAUR448(t *testing.T) {
	root := aur448IntegrationRoot(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}
	knownProblemFixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(knownProblemFixture); err != nil {
		t.Fatalf("required input missing: %s: %v", knownProblemFixture, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur448")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	// run separates stdout and stderr into independent buffers -- they
	// travel through different writers in cmd/aurumcode/main.go (stdout is
	// a raw *os.File, stderr is wrapped by the AUR-432 redaction writer
	// with its own Flush), so nothing here may assume their relative
	// interleaving in a combined capture.
	run := func(env []string, args ...string) (stdout, stderr string, exitCode int) {
		t.Helper()
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = env
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("running %v: %v", args, err)
			}
		}
		return outBuf.String(), errBuf.String(), code
	}

	baseEnv := func(extra ...string) []string {
		drop := map[string]bool{
			"AURUM_SECRET_CANARY":   true,
			"AURUMCODE_LLM_FIXTURE": true,
			"LLM_API_KEY":           true,
			"LLM_BASE_URL":          true,
			// A cache hit skips GenerateReview entirely (no discard to
			// report even for a fixture that would otherwise discard), so
			// this run must never inherit a shared cache directory from
			// the invoking environment.
			"AURUMCODE_CACHE_DIR": true,
		}
		env := make([]string, 0, len(os.Environ())+len(extra))
		for _, kv := range os.Environ() {
			name, _, _ := strings.Cut(kv, "=")
			if !drop[name] {
				env = append(env, kv)
			}
		}
		return append(env, extra...)
	}

	t.Run("no provider: message shows the complete fixture shape with a real rule_id example", func(t *testing.T) {
		stdout, stderr, code := run(baseEnv(), "review", "--base", "HEAD~1")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stdout != "" {
			t.Fatalf("expected empty stdout, got:\n%s", stdout)
		}
		for _, want := range []string{
			"no LLM provider configured",
			`"issues"`, `"file"`, `"line"`, `"severity"`, `"message"`,
			`"rule_id"`,
			"security/hardcoded-secret",
			"tests/fixtures/review/known-problem-response.json",
		} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("expected %q in the provider-missing message, got:\n%s", want, stderr)
			}
		}
		// security/hardcoded-secret must be a REAL id, not a decorative
		// string: prove it by resolving it against the fixture that
		// actually cites it and that this same test file exercises below.
		if !strings.Contains(stderr, "discarded") {
			t.Fatalf("expected the message to explain why rule_id matters (a discard), got:\n%s", stderr)
		}
	})

	t.Run("happy path: zero discards, stdout AND stderr byte-identical to the published contract", func(t *testing.T) {
		stdout, stderr, code := run(baseEnv("AURUMCODE_LLM_FIXTURE="+knownProblemFixture), "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		wantStdout := "config/demo-tokens.txt:4: [error] A credential-shaped value was committed in plain text (DEMO_API_TOKEN). (rule security/hardcoded-secret: Hardcoded Secrets)\n"
		if stdout != wantStdout {
			t.Fatalf("stdout regressed on the zero-discard path:\ngot:  %q\nwant: %q", stdout, wantStdout)
		}
		if stderr != "" {
			t.Fatalf("expected zero bytes on stderr when nothing was discarded, got:\n%q", stderr)
		}
	})

	t.Run("mixed discard: stdout hides ungrounded findings, stderr names how many and why", func(t *testing.T) {
		fixture := filepath.Join(t.TempDir(), "mixed.json")
		if err := os.WriteFile(fixture, []byte(aur448IntegrationMixedFixture), 0o600); err != nil {
			t.Fatalf("writing mixed fixture: %v", err)
		}
		stdout, stderr, code := run(baseEnv("AURUMCODE_LLM_FIXTURE="+fixture), "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt:3") || !strings.Contains(stdout, "(rule security/hardcoded-secret: Hardcoded Secrets)") {
			t.Fatalf("expected the grounded finding on stdout, got:\n%s", stdout)
		}
		for _, leak := range []string{"no rule_id at all", "unknown rule_id"} {
			if strings.Contains(stdout, leak) {
				t.Fatalf("a discarded finding's message reached stdout: %q, got:\n%s", leak, stdout)
			}
		}
		wantStderr := "aurumcode review: 2 finding(s) discarded: 1 with no rule_id, 1 citing an unknown rule_id (security/definitely-not-a-rule)\n"
		if stderr != wantStderr {
			t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", stderr, wantStderr)
		}

		// Determinism: same input, same stdout AND same stderr.
		stdout2, stderr2, code2 := run(baseEnv("AURUMCODE_LLM_FIXTURE="+fixture), "review", "--base", "HEAD~1")
		if code2 != 0 {
			t.Fatalf("rerun: expected exit 0, got %d", code2)
		}
		if stdout != stdout2 {
			t.Fatalf("stdout is not deterministic:\n1: %q\n2: %q", stdout, stdout2)
		}
		if stderr != stderr2 {
			t.Fatalf("stderr is not deterministic:\n1: %q\n2: %q", stderr, stderr2)
		}
	})

	t.Run("every finding discarded: unchanged No issues found. on stdout, exact discard reason on stderr", func(t *testing.T) {
		fixture := filepath.Join(t.TempDir(), "all-discarded.json")
		if err := os.WriteFile(fixture, []byte(aur448IntegrationAllDiscardedFixture), 0o600); err != nil {
			t.Fatalf("writing all-discarded fixture: %v", err)
		}
		stdout, stderr, code := run(baseEnv("AURUMCODE_LLM_FIXTURE="+fixture), "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		// This is the exact defect the card's Outcome exists to fix: before
		// AUR-448, this run produced "No issues found." with NOTHING on
		// stderr, indistinguishable from a genuinely clean review.
		if stdout != "No issues found.\n" {
			t.Fatalf("expected the unchanged AUR-430/AUR-434 no-findings output, got: %q", stdout)
		}
		wantStderr := "aurumcode review: 1 finding(s) discarded: 1 with no rule_id\n"
		if stderr != wantStderr {
			t.Fatalf("stderr mismatch:\ngot:  %q\nwant: %q", stderr, wantStderr)
		}
	})
}
