// Selector naming note, the same technique tests/unit/AUR-442.go and
// tests/unit/AUR-445.go document: a function named TestAUR435 already
// exists in this package (tests/unit/AUR-435.go) and would collide, so this
// proof is TestAUR449 even though the card text's TDD proof section names
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

// aur449Root resolves the repository root exactly like the sibling engine
// unit programs do (see tests/unit/AUR-436.go's aur436Root): AURUMCODE_ROOT
// wins (the acceptance harness sets it to the staged materialization root),
// and a direct run from a full checkout climbs two directories back to the
// root.
func aur449Root(t *testing.T) string {
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

// aur449BaseEnv returns the process environment with every variable that
// influences cmd/aurumcode's provider selection removed (mirroring
// tests/unit/AUR-436.go's aur436BaseEnv), so each case states its provider
// configuration explicitly and inherits none by accident from the harness.
func aur449BaseEnv() []string {
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
	aur449SecHeader   = "Security findings (standards/security-review):"
	aur449SecCitation = "(rule security/hardcoded-secret: Hardcoded Secrets)"
	aur449NoProvider  = "no LLM provider configured"
)

// TestAUR449 proves the AUR-449 outcome at the CLI boundary through the
// real binary: `aurumcode review --base HEAD~1 --seguranca` run with no LLM
// provider configured at all now runs the deterministic security pass
// (restored by AUR-442) and reports its findings with exit 0, instead of
// refusing outright with `no LLM provider configured` -- because that pass
// is a regex matcher over the diff's added lines and never calls a model.
// The skip is reported plainly on stderr, never silently. Every path this
// card does not own stays exactly as published: --seguranca combined with a
// configured provider is byte-identical (manually verified against a
// parent build during development, sha256
// 63c649af1c90e38b473e1bd45b4152b1f96ecad17d5d9c05c17bb94df7b8240f for
// git-demo's --seguranca stdout with the fixture provider, on both sides of
// this card's change); an explicit --modelo that cannot be served still
// fails loudly (reportModelUnavailable); a provider that WAS attempted but
// is broken (an AURUMCODE_LLM_FIXTURE path that does not exist) still
// fails, because only "nothing configured at all" is eligible for the
// skip; and without --seguranca the pre-existing no-provider refusal is
// untouched. See docs/specs/AUR-449.md.
func TestAUR449(t *testing.T) {
	root := aur449Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur449")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	run := func(extraEnv []string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur449BaseEnv(), extraEnv...)
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

	t.Run("SegurancaAloneRunsWithoutProvider", func(t *testing.T) {
		code, stdout, stderr := run(nil, "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, aur449SecHeader) {
			t.Fatalf("expected the security section, got:\n%s", stdout)
		}
		for _, line := range []string{"config/demo-tokens.txt:4: [error]", "config/demo-tokens.txt:5: [error]", "config/demo-tokens.txt:6: [error]"} {
			if !strings.Contains(stdout, line) {
				t.Fatalf("expected %q in the security section, got:\n%s", line, stdout)
			}
		}
		if strings.Contains(stdout, "No issues found.") {
			t.Fatalf("the never-run quality section must not print a claim about it, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, aur449NoProvider) || !strings.Contains(stderr, "quality review skipped") {
			t.Fatalf("expected an actionable skip explanation on stderr, got:\n%s", stderr)
		}
	})

	t.Run("SegurancaAloneReportsHonestAbsenceWhenNothingMatches", func(t *testing.T) {
		// --base HEAD against itself is an empty diff (no new fixture
		// needed): the security pass still ran and found nothing, which
		// must print as the honest "No security findings." -- not be
		// confused with the quality section's own, unrelated absence
		// wording, and not look like the pass silently never ran.
		code, stdout, stderr := run(nil, "--base", "HEAD", "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, aur449SecHeader) {
			t.Fatalf("expected the security section header even with nothing to report, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "No security findings.") {
			t.Fatalf("expected the honest-absence line, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "No issues found.") {
			t.Fatalf("the never-run quality section must not print a claim about it, got:\n%s", stdout)
		}
	})

	t.Run("SegurancaAloneIsDeterministic", func(t *testing.T) {
		_, first, _ := run(nil, "--seguranca")
		_, second, _ := run(nil, "--seguranca")
		if first != second {
			t.Fatalf("expected identical output across runs:\nfirst=%q\nsecond=%q", first, second)
		}
	})

	t.Run("SegurancaWithFailOnStillClosesTheGateWithoutProvider", func(t *testing.T) {
		code, stdout, stderr := run(nil, "--seguranca", "--fail-on", "high")
		if code != 3 {
			t.Fatalf("expected exit 3 (gate closed by the security findings), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
	})

	t.Run("WithoutSegurancaNoProviderIsUnchanged", func(t *testing.T) {
		code, stdout, stderr := run(nil)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s", code, stdout)
		}
		if stdout != "" {
			t.Fatalf("expected no stdout, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, aur449NoProvider) {
			t.Fatalf("expected the pre-existing AUR-430 error text, got:\n%s", stderr)
		}
		if strings.Contains(stderr, "quality review skipped") {
			t.Fatalf("the AUR-449 skip note must never appear without --seguranca, got:\n%s", stderr)
		}
	})

	t.Run("ExplicitModeloStillFailsLoudlyWithoutProvider", func(t *testing.T) {
		// Non-goal boundary: --modelo is a specific request, so it must
		// never be silently downgraded into the --seguranca-only skip even
		// when --seguranca is also given.
		code, stdout, stderr := run(nil, "--seguranca", "--modelo", "local")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, `model "local" is unavailable`) {
			t.Fatalf("expected the AUR-436 model-unavailable error, got:\n%s", stderr)
		}
		if strings.Contains(stdout, aur449SecHeader) {
			t.Fatalf("an explicit unavailable --modelo must fail before any output, got:\n%s", stdout)
		}
	})

	t.Run("AttemptedButBrokenProviderStillFails", func(t *testing.T) {
		// A caller who tried to configure a fixture and got the path wrong
		// is a different error than "nothing configured" -- it must not be
		// silently downgraded into the skip either.
		bogus := filepath.Join(t.TempDir(), "does-not-exist.json")
		code, stdout, stderr := run([]string{"AURUMCODE_LLM_FIXTURE=" + bogus}, "--seguranca")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if strings.Contains(stderr, "quality review skipped") {
			t.Fatalf("a broken (attempted) provider configuration must not trigger the skip, got:\n%s", stderr)
		}
		if strings.Contains(stdout, aur449SecHeader) {
			t.Fatalf("a broken provider must fail before any output, got:\n%s", stdout)
		}
	})

	t.Run("SecretCanaryNeverLeaksOnTheSkipPath", func(t *testing.T) {
		canary := "aurum-canary-449-unit"
		code, stdout, stderr := run([]string{"AURUM_SECRET_CANARY=" + canary}, "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if strings.Contains(stdout, canary) || strings.Contains(stderr, canary) {
			t.Fatal("the secret canary must never reach stdout or stderr")
		}
	})

	t.Run("SegurancaWithProviderKeepsTheCitationAndStandard", func(t *testing.T) {
		// With a provider present the AUR-442 contract is unaffected: this
		// does not re-assert every byte (tests/integration/AUR-449.go and
		// tests/acceptance/AUR-449.sh do), only that the security section's
		// content is exactly what it already was.
		fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
		code, stdout, stderr := run([]string{"AURUMCODE_LLM_FIXTURE=" + fixture}, "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, aur449SecCitation) {
			t.Fatalf("expected the rule citation, got:\n%s", stdout)
		}
		if strings.Contains(stderr, "quality review skipped") {
			t.Fatalf("the skip note must never appear when a provider is configured, got:\n%s", stderr)
		}
	})
}
