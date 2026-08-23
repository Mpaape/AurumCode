// Selector naming note, the same technique tests/unit/AUR-442.go and
// tests/unit/AUR-449.go document: a function named TestAUR435 already
// exists in this package (tests/unit/AUR-435.go) and would collide, so this
// proof is TestAUR450 even though the card text's TDD proof section names
// TestAUR435 (a copy-paste artifact from the AUR-435 card this one was
// drafted against).
package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur450Root resolves the repository root exactly like the sibling engine
// unit programs do (see tests/unit/AUR-449.go's aur449Root): AURUMCODE_ROOT
// wins (the acceptance harness sets it to the staged materialization root),
// and a direct run from a full checkout climbs two directories back to the
// root.
func aur450Root(t *testing.T) string {
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

// aur450BaseEnv returns the process environment with every variable that
// influences cmd/aurumcode's provider selection removed (mirroring
// tests/unit/AUR-449.go's aur449BaseEnv), so each case states its provider
// configuration explicitly and inherits none by accident from the harness.
func aur450BaseEnv() []string {
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
	aur450SecHeader = "Security findings (standards/security-review):"
	// The coverage note the card's Outcome adds: catalog-wide, so it does
	// not depend on the diff -- proved identical whether the pass found
	// something or found nothing. The embedded catalog (AUR-434/AUR-442,
	// internal/review/rules/security.yml) declares 8 security-category
	// rules; exactly 3 (sql-injection, command-injection, hardcoded-secret)
	// carry a matcher. If a future card adds or removes a matcher these
	// constants -- and the sibling ones in tests/integration/AUR-450.go and
	// tests/e2e/AUR-450.sh -- must be re-derived from the catalog, not
	// patched blindly to make an assertion pass.
	aur450CoveragePrefix = "aurumcode review: security pass applied 4 of 8 security rules ("
	aur450CoveragePtr    = "internal/review/rules/security.yml"
)

var aur450CoverageRules = []string{"security/command-injection", "security/hardcoded-secret", "security/sql-injection"}

// TestAUR450 proves the AUR-450 outcome at the CLI boundary through the
// real binary: when `--seguranca` runs the security pass, stderr now names
// how many and which security-category rules of the embedded catalog the
// pass actually applied (carry a matcher) against how many the category
// declares in total -- identically whether the pass found a match or
// found nothing at all, so "No security findings." is never misread as
// "the code was scanned by the full catalog." Findings themselves (their
// content, their order, the AUR-442/AUR-449 byte contract with a provider
// configured) are completely untouched. See docs/specs/AUR-450.md.
func TestAUR450(t *testing.T) {
	root := aur450Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur450")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	run := func(extraEnv []string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur450BaseEnv(), extraEnv...)
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

	assertCoverageNote := func(t *testing.T, stderr string) {
		t.Helper()
		if !strings.Contains(stderr, aur450CoveragePrefix) {
			t.Fatalf("expected the coverage note, got:\n%s", stderr)
		}
		for _, rule := range aur450CoverageRules {
			if !strings.Contains(stderr, rule) {
				t.Fatalf("expected the coverage note to name %q, got:\n%s", rule, stderr)
			}
		}
		if !strings.Contains(stderr, aur450CoveragePtr) {
			t.Fatalf("expected the coverage note to point at %q, got:\n%s", aur450CoveragePtr, stderr)
		}
	}

	t.Run("CoverageNoteAppearsWhenFindingsExist", func(t *testing.T) {
		code, stdout, stderr := run(nil, "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, aur450SecHeader) {
			t.Fatalf("expected the security section, got:\n%s", stdout)
		}
		for _, line := range []string{"config/demo-tokens.txt:4: [error]", "config/demo-tokens.txt:5: [error]", "config/demo-tokens.txt:6: [error]"} {
			if !strings.Contains(stdout, line) {
				t.Fatalf("expected %q in the security section, got:\n%s", line, stdout)
			}
		}
		assertCoverageNote(t, stderr)
	})

	t.Run("CoverageNoteIsIdenticalWhenNothingMatches", func(t *testing.T) {
		// --base HEAD against itself is an empty diff: the pass still ran
		// and found nothing, "No security findings." -- but the coverage
		// note is catalog-wide, so it must be byte-identical to the
		// findings case above, not merely present.
		_, _, withFindingsStderr := run(nil, "--seguranca")
		code, stdout, stderr := run(nil, "--base", "HEAD", "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "No security findings.") {
			t.Fatalf("expected the honest-absence line, got:\n%s", stdout)
		}
		assertCoverageNote(t, stderr)
		if stderr != withFindingsStderr {
			t.Fatalf("expected the catalog-wide coverage note to be identical regardless of findings:\nempty=%q\nnonempty=%q", stderr, withFindingsStderr)
		}
	})

	t.Run("CoverageNoteIsDeterministic", func(t *testing.T) {
		_, _, first := run(nil, "--seguranca")
		_, _, second := run(nil, "--seguranca")
		if first != second {
			t.Fatalf("expected identical stderr across runs:\nfirst=%q\nsecond=%q", first, second)
		}
	})

	t.Run("CoverageNoteAppearsWithAProviderConfiguredToo", func(t *testing.T) {
		// The AUR-442/AUR-449 stdout contract with a provider configured is
		// this card's own hard constraint (tests/acceptance/AUR-449.sh pins
		// its exact sha256): the coverage note must land on stderr, never
		// touching stdout.
		fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
		code, stdout, stderr := run([]string{"AURUMCODE_LLM_FIXTURE=" + fixture}, "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "(rule security/hardcoded-secret: Hardcoded Secrets)") {
			t.Fatalf("expected the AUR-442 citation on stdout, unaffected, got:\n%s", stdout)
		}
		assertCoverageNote(t, stderr)
	})

	t.Run("CoverageNoteNeverAppearsWithoutSeguranca", func(t *testing.T) {
		fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
		code, stdout, stderr := run([]string{"AURUMCODE_LLM_FIXTURE=" + fixture})
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if strings.Contains(stdout+stderr, aur450CoveragePrefix) {
			t.Fatalf("the coverage note must be scoped to --seguranca, got:\nstdout=%s\nstderr=%s", stdout, stderr)
		}
	})

	t.Run("ExplicitModeloStillFailsLoudlyBeforeAnyCoverageNote", func(t *testing.T) {
		code, stdout, stderr := run(nil, "--seguranca", "--modelo", "local")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if strings.Contains(stderr, aur450CoveragePrefix) {
			t.Fatalf("an explicit unavailable --modelo must fail before the coverage note prints, got:\n%s", stderr)
		}
	})

	t.Run("SecretCanaryNeverLeaksAlongsideTheCoverageNote", func(t *testing.T) {
		canary := "aurum-canary-450-unit"
		code, stdout, stderr := run([]string{"AURUM_SECRET_CANARY=" + canary}, "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if strings.Contains(stdout, canary) || strings.Contains(stderr, canary) {
			t.Fatal("the secret canary must never reach stdout or stderr")
		}
	})
}
