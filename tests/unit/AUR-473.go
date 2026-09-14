// See tests/unit/AUR-450.go's selector naming note: the function here is
// TestAUR473 rather than a name derived from a sibling card.
package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur473Root resolves the repository root the same way the sibling engine
// unit programs do (tests/unit/AUR-450.go's aur450Root): AURUMCODE_ROOT wins
// when the acceptance harness sets it to the staged materialization root,
// and a direct run from a full checkout climbs two directories back.
func aur473Root(t *testing.T) string {
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

// aur473BaseEnv returns the process environment stripped of every variable
// that influences cmd/aurumcode's provider selection, mirroring
// tests/unit/AUR-450.go's aur450BaseEnv, so each case states its provider
// configuration explicitly instead of inheriting one by accident.
func aur473BaseEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE": true,
		"LLM_API_KEY":           true,
		"LLM_BASE_URL":          true,
		"LLM_MODEL":             true,
		"AURUM_SECRET_CANARY":   true,
	}
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !drop[name] {
			env = append(env, kv)
		}
	}
	return env
}

const (
	aur473SecHeader      = "Security findings (standards/security-review):"
	aur473CoveragePrefix = "aurumcode review: security pass applied 4 of 8 security rules ("
)

// TestAUR473 proves the card's outcome at the CLI boundary through the real
// binary: `--modelo` naming a model the gateway does not serve is a USAGE
// error that fails before any work is computed -- exit 1, the model named on
// stderr, and neither the security section nor its coverage note printed --
// while a provider that WAS configured and fails at runtime with --seguranca
// still delivers its already-computed deterministic findings (AUR-458,
// untouched). See docs/specs/AUR-473.md.
func TestAUR473(t *testing.T) {
	root := aur473Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}
	fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("required input missing: %s: %v", fixture, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur473")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	run := func(extraEnv []string, args ...string) (int, string, string) {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur473BaseEnv(), extraEnv...)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("running %v: %v", args, err)
			}
			code = ee.ExitCode()
		}
		return code, stdout.String(), stderr.String()
	}

	// AC-001: a named model the gateway cannot serve fails as a usage error,
	// before the deterministic security pass -- and therefore before any
	// finding, section or coverage note -- is computed.
	t.Run("UnavailableModeloIsUsageErrorBeforeAnyWork", func(t *testing.T) {
		code, stdout, stderr := run(nil, "review", "--base", "HEAD~1", "--seguranca", "--modelo", "local")
		if code != 1 {
			t.Fatalf("expected exit 1 for an unavailable --modelo, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, `model "local" is unavailable`) {
			t.Fatalf("stderr must name the unavailable model, got:\n%s", stderr)
		}
		if strings.Contains(stdout, aur473SecHeader) {
			t.Fatalf("the security section must NOT print when the named model is unavailable, got:\n%s", stdout)
		}
		if strings.Contains(stderr, aur473CoveragePrefix) {
			t.Fatalf("the coverage note must NOT print when the named model is unavailable, got:\n%s", stderr)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Fatalf("no work should be computed for a usage error, got stdout:\n%s", stdout)
		}
	})

	// AC-002: AUR-458 stays intact. A provider that was configured and fails
	// at runtime with --seguranca still prints its deterministic security
	// findings and exits 1. A candidate that zeroes AC-001 by breaking this
	// must be rejected.
	t.Run("RuntimeProviderFailureStillPrintsSecurityFindings", func(t *testing.T) {
		code, stdout, stderr := run(
			[]string{"LLM_API_KEY=k", "LLM_BASE_URL=http://127.0.0.1:9/v1"},
			"review", "--base", "HEAD~1", "--seguranca",
		)
		if code != 1 {
			t.Fatalf("expected exit 1 for a configured provider that fails, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, aur473SecHeader) {
			t.Fatalf("a configured provider that fails at runtime must still print the security findings, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, aur473CoveragePrefix) {
			t.Fatalf("the coverage note must still print when the security pass really ran, got:\n%s", stderr)
		}
	})

	// Non-regression: a --modelo the fixture CAN serve still reviews.
	t.Run("ServableModeloStillReviews", func(t *testing.T) {
		code, _, stderr := run(
			[]string{"AURUMCODE_LLM_FIXTURE=" + fixture},
			"review", "--base", "HEAD~1", "--seguranca", "--modelo", "demo",
		)
		if code != 0 {
			t.Fatalf("expected exit 0 for a servable --modelo, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, `reviewing with model "demo"`) {
			t.Fatalf("expected the selection note for a servable model, got:\n%s", stderr)
		}
	})
}
