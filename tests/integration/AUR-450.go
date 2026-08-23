// Selector naming note, the same technique tests/integration/AUR-442.go and
// tests/integration/AUR-449.go document: a function named IntegrationAUR435
// already exists in this package (tests/integration/AUR-435.go) and would
// collide, so this proof is IntegrationAUR450 even though the card text's
// TDD proof section names IntegrationAUR435 (a copy-paste artifact from the
// AUR-435 card this one was drafted against).
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

// aur450Root resolves the repository root exactly like the sibling engine
// integration programs do (see tests/integration/AUR-449.go's aur449Root):
// AURUMCODE_ROOT wins (the acceptance harness sets it to the staged
// materialization root), and a direct run from a full checkout climbs two
// directories back to the root.
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

const (
	aur450SecurityHeader = "Security findings (standards/security-review):"
	aur450Citation       = "(rule security/hardcoded-secret: Hardcoded Secrets)"
	// See tests/unit/AUR-450.go's own constant comment: catalog-wide, not
	// diff-derived, so it is proved identical whether the pass matched
	// something or nothing.
	aur450CoveragePrefix = "aurumcode review: security pass applied 4 of 8 security rules ("
	aur450CoveragePtr    = "internal/review/rules/security.yml"
)

var aur450CoverageRules = []string{"security/command-injection", "security/hardcoded-secret", "security/sql-injection"}

// IntegrationAUR450 builds the real aurumcode binary and proves AUR-450's
// outcome end to end, against tests/fixtures/repos/git-demo (the project's
// own demo fixture, which plants three synthetic secrets AUR-442 already
// taught the security pass to find):
//
//  1. `--seguranca` prints, on stderr, how many and which security-category
//     rules of the embedded catalog it actually applied against how many
//     the category declares in total.
//  2. The same note, byte-identical, appears when the pass finds nothing
//     (an empty diff) -- the exact case the card's Outcome names: "No
//     security findings." must never be misread as full coverage.
//  3. The note is deterministic across reruns.
//  4. With a provider configured, the AUR-442/AUR-449 stdout contract is
//     completely unaffected -- the note lands on stderr only.
//  5. --fail-on still closes the gate exactly as before.
//  6. An explicit --modelo that cannot be served still fails loudly,
//     before any --seguranca output (coverage note included) ever prints.
//  7. Without --seguranca, no coverage note appears anywhere.
//  8. The secret canary never reaches a sink alongside the new note.
//
// See docs/specs/AUR-450.md.
func IntegrationAUR450(t *testing.T) {
	root := aur450Root(t)
	demoRepo := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(demoRepo); err != nil {
		t.Fatalf("required input missing: %s: %v", demoRepo, err)
	}
	fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("required input missing: %s: %v", fixture, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur450")
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

	assertCoverageNote := func(t *testing.T, errOut string) {
		t.Helper()
		if !strings.Contains(errOut, aur450CoveragePrefix) {
			t.Fatalf("expected the coverage note, got:\n%s", errOut)
		}
		for _, rule := range aur450CoverageRules {
			if !strings.Contains(errOut, rule) {
				t.Fatalf("expected the coverage note to name %q, got:\n%s", rule, errOut)
			}
		}
		if !strings.Contains(errOut, aur450CoveragePtr) {
			t.Fatalf("expected the coverage note to point at %q, got:\n%s", aur450CoveragePtr, errOut)
		}
	}

	// 1. The pass finds something: the coverage note is on stderr.
	out, errOut, code := run(nil, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, aur450SecurityHeader) {
		t.Fatalf("expected the security section, got:\n%s", out)
	}
	for _, line := range []int{4, 5, 6} {
		want := "config/demo-tokens.txt:" + strconv.Itoa(line) + ": [error]"
		if !strings.Contains(out, want) {
			t.Fatalf("expected a finding at %s, got:\n%s", want, out)
		}
	}
	assertCoverageNote(t, errOut)

	// 2. Honest absence: identical coverage note when nothing matches.
	cleanOut, cleanErr, code := run(nil, "review", "--base", "HEAD", "--seguranca")
	if code != 0 {
		t.Fatalf("expected exit 0 for an empty diff with --seguranca, got %d", code)
	}
	if !strings.Contains(cleanOut, "No security findings.") {
		t.Fatalf("expected the honest-absence line, got:\n%s", cleanOut)
	}
	assertCoverageNote(t, cleanErr)
	if cleanErr != errOut {
		t.Fatalf("expected the catalog-wide coverage note to be identical regardless of findings:\nempty=%q\nnonempty=%q", cleanErr, errOut)
	}

	// 3. Determinism.
	again, againErr, code2 := run(nil, "review", "--base", "HEAD~1", "--seguranca")
	if code2 != 0 || again != out || againErr != errOut {
		t.Fatalf("--seguranca is not deterministic (exit %d):\nstdout first=%q second=%q\nstderr first=%q second=%q", code2, out, again, errOut, againErr)
	}

	// 4. With a provider configured, stdout stays exactly the AUR-442
	// contract; the note lands on stderr only.
	withProv, withProvErr, code := run([]string{"AURUMCODE_LLM_FIXTURE=" + fixture}, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("expected exit 0 with a provider configured, got %d", code)
	}
	_, section, foundHeader := strings.Cut(withProv, aur450SecurityHeader)
	if !foundHeader {
		t.Fatalf("expected the security section header, got:\n%s", withProv)
	}
	if strings.Count(section, aur450Citation) != 3 {
		t.Fatalf("expected exactly three hardcoded-secret citations, got:\n%s", section)
	}
	assertCoverageNote(t, withProvErr)
	for _, secretValue := range []string{
		"AURUM-FAKE-TOKEN-0000-0001",
		"AURUM-FAKE-PASSWORD-0000-0002",
		"AURUM-FAKE-WEBHOOK-0000-0003",
	} {
		if strings.Contains(withProv+withProvErr, secretValue) {
			t.Fatalf("the matched secret VALUE %q must never reach a sink, got stdout:\n%s\nstderr:\n%s", secretValue, withProv, withProvErr)
		}
	}

	// 5. --fail-on still closes the gate exactly as before.
	_, _, code = run(nil, "review", "--base", "HEAD~1", "--seguranca", "--fail-on", "high")
	if code != 3 {
		t.Fatalf("expected exit 3 (gate closed by the security findings), got %d", code)
	}

	// 6. An explicit --modelo that cannot be served still fails loudly,
	// before any --seguranca output -- coverage note included -- prints.
	modOut, modErr, code := run(nil, "review", "--base", "HEAD~1", "--seguranca", "--modelo", "local")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unavailable --modelo, got %d\nstdout=%s\nstderr=%s", code, modOut, modErr)
	}
	if strings.Contains(modOut, aur450SecurityHeader) || strings.Contains(modErr, aur450CoveragePrefix) {
		t.Fatalf("an explicit unavailable --modelo must fail before any --seguranca output, got:\nstdout=%s\nstderr=%s", modOut, modErr)
	}

	// 7. Without --seguranca, no coverage note appears anywhere.
	plainOut, plainErr, code := run([]string{"AURUMCODE_LLM_FIXTURE=" + fixture}, "review", "--base", "HEAD~1")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.Contains(plainOut+plainErr, aur450CoveragePrefix) {
		t.Fatalf("the coverage note must be scoped to --seguranca, got:\nstdout=%s\nstderr=%s", plainOut, plainErr)
	}

	// 8. The secret canary never reaches a sink alongside the new note.
	canary := "aurum-canary-450-integration"
	canOut, canErr, code := run([]string{"AURUM_SECRET_CANARY=" + canary}, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("canary run failed: exit %d\nstderr=%s", code, canErr)
	}
	if strings.Contains(canOut, canary) || strings.Contains(canErr, canary) {
		t.Fatal("the secret canary must never reach stdout or stderr")
	}
}
