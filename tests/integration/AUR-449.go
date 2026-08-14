// Selector naming note, the same technique tests/integration/AUR-442.go and
// tests/integration/AUR-445.go document: a function named
// IntegrationAUR435 already exists in this package
// (tests/integration/AUR-435.go) and would collide, so this proof is
// IntegrationAUR449 even though the card text's TDD proof section names
// IntegrationAUR435 (a copy-paste artifact from the AUR-435 card this one
// was drafted against).
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// aur449Root resolves the repository root exactly like the sibling engine
// integration programs do (see tests/integration/AUR-442.go's aur442Root):
// AURUMCODE_ROOT wins (the acceptance harness sets it to the staged
// materialization root), and a direct run from a full checkout climbs two
// directories back to the root.
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

const (
	aur449SecurityHeader = "Security findings (standards/security-review):"
	aur449Citation       = "(rule security/hardcoded-secret: Hardcoded Secrets)"
	aur449Standard       = "standards/security-review SCR-003"
)

// IntegrationAUR449 builds the real aurumcode binary and proves AUR-449's
// outcome end to end, against tests/fixtures/repos/git-demo (the project's
// own demo fixture, which plants three synthetic secrets AUR-442 already
// taught the security pass to find):
//
//  1. With no LLM provider configured at all, `--seguranca` alone now runs
//     the deterministic security pass and reports its findings with exit
//     0, instead of refusing with "no LLM provider configured" -- because
//     the pass is a regex matcher over the diff's added lines and calls no
//     model. The skip is explained on stderr, never silent, and the run is
//     deterministic.
//  2. Composed with --fail-on: the matched secrets (severity error) still
//     close the gate even though no quality review ran.
//  3. With a provider configured (the offline fixture), `--seguranca`'s
//     output is exactly what AUR-442 already proved: unaffected by this
//     card.
//  4. --seguranca combined with an explicit --modelo that cannot be served
//     still fails loudly (AUR-436's reportModelUnavailable) -- an explicit
//     model choice is never silently downgraded into the skip.
//  5. Without --seguranca, the pre-existing no-provider refusal
//     (AUR-430's contract) is untouched, byte for byte.
//  6. The secret canary never reaches a sink on the skip path.
//
// See docs/specs/AUR-449.md.
func IntegrationAUR449(t *testing.T) {
	root := aur449Root(t)
	demoRepo := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(demoRepo); err != nil {
		t.Fatalf("required input missing: %s: %v", demoRepo, err)
	}
	fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("required input missing: %s: %v", fixture, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur449")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	run := func(extraEnv []string, args ...string) (string, string, int) {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = demoRepo
		env := os.Environ()
		env = append(env, "AURUMCODE_LLM_FIXTURE=", "LLM_API_KEY=", "LLM_BASE_URL=")
		env = append(env, extraEnv...)
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("running %v: %v\nstderr=%s", args, err, stderr.String())
			}
			code = exitErr.ExitCode()
		}
		return stdout.String(), stderr.String(), code
	}

	// 1. No provider at all: the security pass runs alone.
	out, errOut, code := run(nil, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("expected exit 0 with no provider and --seguranca, got %d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, aur449SecurityHeader) {
		t.Fatalf("expected the security section, got:\n%s", out)
	}
	for _, line := range []int{4, 5, 6} {
		want := "config/demo-tokens.txt:" + strconv.Itoa(line) + ": [error]"
		if !strings.Contains(out, want) {
			t.Fatalf("expected a finding at %s, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No issues found.") {
		t.Fatalf("the never-run quality section must not claim an empty result, got:\n%s", out)
	}
	if !strings.Contains(errOut, "no LLM provider configured") || !strings.Contains(errOut, "quality review skipped") {
		t.Fatalf("expected an actionable skip explanation on stderr, got:\n%s", errOut)
	}

	// Determinism.
	again, _, code2 := run(nil, "review", "--base", "HEAD~1", "--seguranca")
	if code2 != 0 || again != out {
		t.Fatalf("--seguranca without a provider is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code2, out, again)
	}

	// 1b. Honest absence on the skip path: --base HEAD against itself is
	// an empty diff (no new fixture needed) -- the pass still ran and
	// found nothing, which prints as "No security findings.", not as a
	// silently-skipped section.
	cleanOut, _, code := run(nil, "review", "--base", "HEAD", "--seguranca")
	if code != 0 {
		t.Fatalf("expected exit 0 for an empty diff with --seguranca and no provider, got %d", code)
	}
	if !strings.Contains(cleanOut, aur449SecurityHeader) {
		t.Fatalf("expected the security section header even with nothing to report, got:\n%s", cleanOut)
	}
	if !strings.Contains(cleanOut, "No security findings.") {
		t.Fatalf("expected the honest-absence line, got:\n%s", cleanOut)
	}

	// 2. --fail-on still closes the gate off the security findings alone.
	_, _, code = run(nil, "review", "--base", "HEAD~1", "--seguranca", "--fail-on", "high")
	if code != 3 {
		t.Fatalf("expected exit 3 (gate closed by the security findings with no quality review), got %d", code)
	}

	// 3. With a provider configured, the AUR-442 contract holds exactly.
	withProv, _, code := run([]string{"AURUMCODE_LLM_FIXTURE=" + fixture}, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("expected exit 0 with a provider configured, got %d", code)
	}
	if !strings.Contains(withProv, aur449SecurityHeader) {
		t.Fatalf("expected the security section with a provider configured, got:\n%s", withProv)
	}
	_, section, foundHeader := strings.Cut(withProv, aur449SecurityHeader)
	if !foundHeader {
		t.Fatalf("expected the security section header, got:\n%s", withProv)
	}
	if strings.Count(section, aur449Citation) != 3 {
		t.Fatalf("expected exactly three hardcoded-secret citations, got:\n%s", section)
	}
	if !strings.Contains(section, aur449Standard) {
		t.Fatalf("expected the project standard citation, got:\n%s", section)
	}
	for _, secretValue := range []string{
		"AURUM-FAKE-TOKEN-0000-0001",
		"AURUM-FAKE-PASSWORD-0000-0002",
		"AURUM-FAKE-WEBHOOK-0000-0003",
	} {
		if strings.Contains(withProv, secretValue) {
			t.Fatalf("the matched secret VALUE %q must never reach stdout, got:\n%s", secretValue, withProv)
		}
	}

	// 4. An explicit --modelo that cannot be served still fails loudly:
	// never silently downgraded into the skip just because --seguranca is
	// also present.
	modOut, modErr, code := run(nil, "review", "--base", "HEAD~1", "--seguranca", "--modelo", "local")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unavailable --modelo, got %d\nstdout=%s\nstderr=%s", code, modOut, modErr)
	}
	if !strings.Contains(modErr, `model "local" is unavailable`) {
		t.Fatalf("expected the AUR-436 model-unavailable error, got:\n%s", modErr)
	}
	if strings.Contains(modOut, aur449SecurityHeader) {
		t.Fatalf("an explicit unavailable --modelo must fail before any output, got:\n%s", modOut)
	}

	// 5. Without --seguranca, the pre-existing no-provider refusal is
	// untouched.
	plainOut, plainErr, code := run(nil, "review", "--base", "HEAD~1")
	if code != 1 {
		t.Fatalf("expected exit 1 without --seguranca and without a provider, got %d", code)
	}
	if plainOut != "" {
		t.Fatalf("expected no stdout, got:\n%s", plainOut)
	}
	if !strings.Contains(plainErr, "no LLM provider configured") {
		t.Fatalf("expected the pre-existing AUR-430 error text, got:\n%s", plainErr)
	}
	if strings.Contains(plainErr, "quality review skipped") {
		t.Fatalf("the AUR-449 skip note must never appear without --seguranca, got:\n%s", plainErr)
	}

	// 6. The secret canary never reaches a sink on the skip path.
	canary := "aurum-canary-449-integration"
	canOut, canErr, code := run([]string{"AURUM_SECRET_CANARY=" + canary}, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("canary run failed: exit %d\nstderr=%s", code, canErr)
	}
	if strings.Contains(canOut, canary) || strings.Contains(canErr, canary) {
		t.Fatal("the secret canary must never reach stdout or stderr")
	}
}
