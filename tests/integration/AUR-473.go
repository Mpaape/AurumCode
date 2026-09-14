// See tests/integration/AUR-450.go's selector naming note: the function here
// is IntegrationAUR473 rather than a name derived from a sibling card.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur473Root resolves the repository root the same way the sibling engine
// integration programs do: AURUMCODE_ROOT wins when the acceptance harness
// sets it to the staged materialization root, and a direct run from a full
// checkout climbs two directories back.
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

const (
	aur473SecurityHeader = "Security findings (standards/security-review):"
	aur473CoveragePrefix = "aurumcode review: security pass applied 4 of 8 security rules ("
)

// IntegrationAUR473 builds the real aurumcode binary and proves the card's
// two-sided contract end to end against tests/fixtures/repos/git-demo:
//
//  1. `--modelo <missing> --seguranca` is a USAGE error: exit 1, the model
//     named on stderr, stdout completely empty, and no security section or
//     coverage note anywhere -- the deterministic pass never ran.
//  2. The usage error outranks the --fail-on gate, so it exits 1, never 3.
//  3. A provider that was CONFIGURED and fails at RUNTIME (AUR-458) still
//     prints its already-computed security findings with --seguranca and
//     exits 1, because findings were computed before the failure.
//  4. An unparseable response is the same: the security pass survives.
//
// See docs/specs/AUR-473.md.
func IntegrationAUR473(t *testing.T) {
	root := aur473Root(t)
	demoRepo := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(demoRepo); err != nil {
		t.Fatalf("required input missing: %s: %v", demoRepo, err)
	}
	fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("required input missing: %s: %v", fixture, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur473")
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

	// 1. The named model is unavailable: usage error, nothing computed.
	out, errOut, code := run(nil, "review", "--base", "HEAD~1", "--seguranca", "--modelo", "not-served")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unavailable --modelo, got %d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	if !strings.Contains(errOut, `model "not-served" is unavailable`) {
		t.Fatalf("stderr must name the unavailable model, got:\n%s", errOut)
	}
	if strings.Contains(out, aur473SecurityHeader) || strings.Contains(errOut, aur473CoveragePrefix) {
		t.Fatalf("an unavailable --modelo must fail before the security pass, got:\nstdout=%s\nstderr=%s", out, errOut)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("a usage error must not compute or print a review, got stdout:\n%s", out)
	}

	// 2. The usage error outranks --fail-on: 1, never 3.
	_, _, code = run(nil, "review", "--base", "HEAD~1", "--seguranca", "--modelo", "not-served", "--fail-on", "high")
	if code != 1 {
		t.Fatalf("the usage error must outrank the --fail-on gate and exit 1, got %d", code)
	}

	// 3. A configured provider that fails at runtime: AUR-458 survives.
	out, errOut, code = run(
		[]string{"LLM_API_KEY=k", "LLM_BASE_URL=http://127.0.0.1:9/v1"},
		"review", "--base", "HEAD~1", "--seguranca",
	)
	if code != 1 {
		t.Fatalf("expected exit 1 for a configured provider that fails, got %d\nstderr=%s", code, errOut)
	}
	if !strings.Contains(out, aur473SecurityHeader) {
		t.Fatalf("a configured provider that fails at runtime must still print the security findings, got:\n%s", out)
	}
	if !strings.Contains(errOut, aur473CoveragePrefix) {
		t.Fatalf("the coverage note must still print, got:\n%s", errOut)
	}

	// 4. An unparseable response is the same contract.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("not json at all {{{"), 0o600); err != nil {
		t.Fatalf("writing bad fixture: %v", err)
	}
	out, _, code = run([]string{"AURUMCODE_LLM_FIXTURE=" + bad}, "review", "--base", "HEAD~1", "--seguranca")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unparseable response, got %d", code)
	}
	if !strings.Contains(out, aur473SecurityHeader) {
		t.Fatalf("an unparseable response must not lose the security pass, got:\n%s", out)
	}
}
