package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur435Root resolves the repository root exactly like the sibling engine
// integration programs do: AURUMCODE_ROOT wins (the acceptance harness sets
// it to the staged materialization root), and a direct run from a full
// checkout climbs two directories back to the root.
func aur435Root(t *testing.T) string {
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

// aur435QualityFixture is a deterministic model response holding exactly one
// QUALITY finding, so the acceptance can tell the quality block and the
// security section apart. The marker string exists only here.
const aur435QualityFixture = `{
  "issues": [
    {
      "file": "src/db.py",
      "line": 4,
      "severity": "info",
      "rule_id": "quality/poor-naming",
      "message": "PLANTED-QUALITY-435 finding for the separation proof."
    }
  ],
  "summary": "Quality-only fixture for AUR-435."
}`

const aur435SecurityHeader = "Security findings (standards/security-review):"
const aur435SecurityCitation = "(rule security/sql-injection: SQL Injection Vulnerability)"

// aur435HardcodedSecretCitation is AUR-442's addition: security/hardcoded-
// secret got a matcher (internal/review/rules/security.yml), so git-demo's
// planted plaintext secret -- three lines, config/demo-tokens.txt:4-6 --
// is now found instead of the honest-looking absence this test originally
// measured. See docs/specs/AUR-442.md.
const aur435HardcodedSecretCitation = "(rule security/hardcoded-secret: Hardcoded Secrets)"

// IntegrationAUR435 builds the real aurumcode binary and proves AUR-435's
// outcome at the CLI boundary: `review --base HEAD~1 --seguranca` runs a
// security pass that reports the planted synthetic SQL injection in a
// section separate from the quality findings, citing the security-category
// rule and the project security standard; without the flag stdout is
// byte-identical to the published AUR-430 contract; and the gate composed
// with --fail-on fails closed on the security finding.
func IntegrationAUR435(t *testing.T) {
	root := aur435Root(t)
	vulnRepo := filepath.Join(root, "tests/fixtures/review/vuln/repo.git")
	demoRepo := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	for _, dir := range []string{vulnRepo, demoRepo} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("required input missing: %s: %v", dir, err)
		}
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur435")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	fixture := filepath.Join(t.TempDir(), "quality.json")
	if err := os.WriteFile(fixture, []byte(aur435QualityFixture), 0o600); err != nil {
		t.Fatalf("writing quality fixture: %v", err)
	}

	run := func(dir string, args ...string) (string, string, int) {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixture)
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

	// Without the flag: the published contract, byte for byte, and no
	// security section anywhere.
	base, _, code := run(vulnRepo, "review", "--base", "HEAD~1")
	if code != 0 {
		t.Fatalf("review without --seguranca must exit 0, got %d", code)
	}
	if strings.Contains(base, aur435SecurityHeader) || strings.Contains(base, aur435SecurityCitation) {
		t.Fatalf("without --seguranca no security section may exist:\n%s", base)
	}
	if !strings.Contains(base, "PLANTED-QUALITY-435") {
		t.Fatalf("the quality finding must print in the base contract:\n%s", base)
	}

	// With the flag: the security pass reports the planted vulnerability in
	// its own section, after the unchanged quality block.
	sec, _, code := run(vulnRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca must exit 0, got %d", code)
	}
	if !strings.HasPrefix(sec, base) {
		t.Fatalf("--seguranca must extend the base output, never rewrite it:\nbase=%q\nsec=%q", base, sec)
	}
	if !strings.Contains(sec, aur435SecurityHeader) {
		t.Fatalf("expected the security section header, got:\n%s", sec)
	}
	quality, security, found := strings.Cut(sec, aur435SecurityHeader)
	if !found {
		t.Fatalf("expected the security section header, got:\n%s", sec)
	}
	if !strings.Contains(security, "src/db.py:8: [error]") || !strings.Contains(security, aur435SecurityCitation) {
		t.Fatalf("the security section must report the planted vulnerability with its rule citation, got:\n%s", security)
	}
	if !strings.Contains(security, "standards/security-review SCR-001") {
		t.Fatalf("the security finding must cite the project security standard, got:\n%s", security)
	}
	if strings.Contains(quality, aur435SecurityCitation) {
		t.Fatalf("the security finding leaked into the quality block:\n%s", quality)
	}
	if strings.Contains(security, "PLANTED-QUALITY-435") {
		t.Fatalf("the quality finding leaked into the security section:\n%s", security)
	}

	// Determinism: same input, same bytes.
	again, _, code := run(vulnRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 || sec != again {
		t.Fatalf("review --seguranca is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code, sec, again)
	}

	// git-demo plants a plaintext secret specifically to be found (three
	// lines: config/demo-tokens.txt:4-6). AUR-442 gave security/hardcoded-
	// secret a matcher, so the deterministic pass now reports exactly that
	// planted secret -- never the SQL-injection rule this repository has
	// nothing to trigger, and never the honest-looking absence this test
	// measured before that matcher existed.
	demo, _, code := run(demoRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on git-demo must exit 0, got %d", code)
	}
	if !strings.Contains(demo, aur435SecurityHeader) {
		t.Fatalf("expected the security section header on git-demo, got:\n%s", demo)
	}
	if strings.Contains(demo, aur435SecurityCitation) {
		t.Fatalf("git-demo has no SQL injection; nothing may be invented:\n%s", demo)
	}
	if strings.Contains(demo, "No security findings.") {
		t.Fatalf("git-demo plants a plaintext secret; \"No security findings.\" is exactly the defect AUR-442 fixes:\n%s", demo)
	}
	if !strings.Contains(demo, aur435HardcodedSecretCitation) {
		t.Fatalf("expected the hardcoded-secret rule citation on git-demo, got:\n%s", demo)
	}
	if !strings.Contains(demo, "standards/security-review SCR-003") {
		t.Fatalf("the hardcoded-secret finding must cite the project security standard, got:\n%s", demo)
	}
	for _, line := range []string{"4", "5", "6"} {
		want := "config/demo-tokens.txt:" + line + ": [error]"
		if !strings.Contains(demo, want) {
			t.Fatalf("expected a finding at %s on git-demo, got:\n%s", want, demo)
		}
	}

	// Composed with --fail-on: the security finding (severity error) closes
	// the gate even though the only quality finding is info-level.
	_, _, code = run(vulnRepo, "review", "--base", "HEAD~1", "--seguranca", "--fail-on", "high")
	if code != 3 {
		t.Fatalf("the security finding must close the --fail-on high gate with exit 3, got %d", code)
	}
	// Without --seguranca the same gate stays open: the published AUR-431
	// behavior is untouched.
	_, _, code = run(vulnRepo, "review", "--base", "HEAD~1", "--fail-on", "high")
	if code != 0 {
		t.Fatalf("without --seguranca the info-only review must keep the gate open, got %d", code)
	}
}
