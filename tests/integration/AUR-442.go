// Selector naming note, the same technique tests/integration/AUR-445.go
// documents: a function named IntegrationAUR435 already exists in this
// package (tests/integration/AUR-435.go) and would collide, so this proof
// is IntegrationAUR442 even though the card text's TDD proof section names
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

// aur442Root resolves the repository root exactly like the sibling engine
// integration programs do (see tests/integration/AUR-435.go's aur435Root):
// AURUMCODE_ROOT wins (the acceptance harness sets it to the staged
// materialization root), and a direct run from a full checkout climbs two
// directories back to the root.
func aur442Root(t *testing.T) string {
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

const aur442SecurityHeader = "Security findings (standards/security-review):"
const aur442Citation = "(rule security/hardcoded-secret: Hardcoded Secrets)"
const aur442Standard = "standards/security-review SCR-003"

// IntegrationAUR442 builds the real aurumcode binary and proves AUR-442's
// outcome at the CLI boundary:
//
//  1. A dedicated fixture (tests/fixtures/review/vuln/hardcoded-secret,
//     new under this card, sibling to and never touching AUR-435's own
//     tests/fixtures/review/vuln/repo.git) proves the restored
//     security/hardcoded-secret matcher fires on both shapes it covers --
//     a quoted Python assignment and an unquoted env-style line -- and not
//     on the deliberately planted benign line beside each.
//  2. tests/fixtures/repos/git-demo -- the project's own demo fixture,
//     whose sole purpose is to plant a plaintext secret -- now reports
//     that secret instead of "No security findings.": the exact defect
//     this card's dogfooding measured (see docs/specs/AUR-442.md).
//  3. Without --seguranca, stdout stays byte-identical to the published
//     AUR-430 contract; the secret canary never reaches a sink; the run
//     is deterministic.
//
// Known, expected consequence (not a defect of this card, not owned by
// its `paths`): tests/acceptance/AUR-435.sh's nominal_case
// ("absence-not-reported") and tests/integration/AUR-435.go's
// IntegrationAUR435 ("expected an honest empty security section on
// git-demo") both hardcode that git-demo's --seguranca run prints "No
// security findings." That assertion encoded the very defect this card
// fixes -- AUR-435 shipped before hardcoded-secret had a matcher, when
// git-demo genuinely matched nothing -- and neither file is in AUR-442's
// `paths`, so this card cannot update them. AUR-435's own MUT-001
// mutation_case (which runs against the unrelated
// tests/fixtures/review/vuln/repo.git, not git-demo) is unaffected and
// stays green. A follow-up card should re-baseline AUR-435's two git-demo
// assertions now that the fixture's plaintext secret is, correctly,
// found.
func IntegrationAUR442(t *testing.T) {
	root := aur442Root(t)
	secretRepo := filepath.Join(root, "tests/fixtures/review/vuln/hardcoded-secret/repo.git")
	demoRepo := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	for _, dir := range []string{secretRepo, demoRepo} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("required input missing: %s: %v", dir, err)
		}
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur442")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("required input missing: %s: %v", fixture, err)
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

	// Without the flag: the published contract, byte for byte, no security
	// section anywhere -- on BOTH repos, since the flag alone gates the
	// section's existence.
	for _, dir := range []string{secretRepo, demoRepo} {
		base, _, code := run(dir, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("review without --seguranca must exit 0 in %s, got %d", dir, code)
		}
		if strings.Contains(base, aur442SecurityHeader) {
			t.Fatalf("without --seguranca no security section may exist in %s:\n%s", dir, base)
		}
	}

	// The dedicated fixture: both planted shapes are found, the two
	// deliberately benign lines beside them are not.
	sec, _, code := run(secretRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the hardcoded-secret fixture must exit 0, got %d", code)
	}
	if !strings.Contains(sec, aur442SecurityHeader) {
		t.Fatalf("expected the security section header, got:\n%s", sec)
	}
	if !strings.Contains(sec, "config/secrets.env:4: [error]") {
		t.Fatalf("expected the unquoted env-style secret to be found, got:\n%s", sec)
	}
	if !strings.Contains(sec, "src/config.py:4: [error]") {
		t.Fatalf("expected the quoted code-style secret to be found, got:\n%s", sec)
	}
	_, secSection, foundHeader := strings.Cut(sec, aur442SecurityHeader)
	if !foundHeader {
		t.Fatalf("expected the security section header, got:\n%s", sec)
	}
	if strings.Count(secSection, aur442Citation) != 2 {
		t.Fatalf("expected exactly two hardcoded-secret citations in the security section (the benign SERVICE_NAME and timeout_seconds lines must not match; the model's own quality-block citation, if any, is out of scope here), got security section:\n%s", secSection)
	}
	if !strings.Contains(secSection, aur442Standard) {
		t.Fatalf("the finding must cite the project security standard, got:\n%s", secSection)
	}
	if strings.Contains(sec, "AURUM-FAKE-KEY-9000-2222") || strings.Contains(sec, "AURUM-FAKE-PASSWORD-9000-1111") {
		t.Fatalf("the matched secret VALUE must never reach stdout, got:\n%s", sec)
	}

	// Determinism: same input, same bytes.
	again, _, code := run(secretRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 || sec != again {
		t.Fatalf("review --seguranca is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code, sec, again)
	}

	// The card's central proof: git-demo's own planted plaintext secret,
	// which existed specifically to be found and never was before this
	// card, is now reported -- not invented, not silent.
	demo, _, code := run(demoRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on git-demo must exit 0, got %d", code)
	}
	if !strings.Contains(demo, aur442SecurityHeader) {
		t.Fatalf("expected the security section header on git-demo, got:\n%s", demo)
	}
	if strings.Contains(demo, "No security findings.") {
		t.Fatalf("git-demo plants a plaintext secret; \"No security findings.\" is exactly the defect this card fixes, got:\n%s", demo)
	}
	_, demoSection, foundDemoHeader := strings.Cut(demo, aur442SecurityHeader)
	if !foundDemoHeader {
		t.Fatalf("expected the security section header on git-demo, got:\n%s", demo)
	}
	for _, line := range []int{4, 5, 6} {
		want := "config/demo-tokens.txt:" + strconv.Itoa(line) + ": [error]"
		if !strings.Contains(demoSection, want) {
			t.Fatalf("expected a finding at %s in the security section, got:\n%s", want, demoSection)
		}
	}
	if strings.Count(demoSection, aur442Citation) != 3 {
		t.Fatalf("expected exactly three hardcoded-secret citations in git-demo's security section (one per planted line; the model's own quality-block citation, if any, is out of scope here), got:\n%s", demoSection)
	}
	for _, secretValue := range []string{
		"AURUM-FAKE-TOKEN-0000-0001",
		"AURUM-FAKE-PASSWORD-0000-0002",
		"AURUM-FAKE-WEBHOOK-0000-0003",
	} {
		if strings.Contains(demo, secretValue) {
			t.Fatalf("the matched secret VALUE %q must never reach stdout, got:\n%s", secretValue, demo)
		}
	}

	// Composed with --fail-on: a matched secret is severity error, so it
	// now closes the gate on git-demo -- it did not before this card,
	// because nothing matched.
	_, _, code = run(demoRepo, "review", "--base", "HEAD~1", "--seguranca", "--fail-on", "high")
	if code != 3 {
		t.Fatalf("the hardcoded-secret finding must close the --fail-on high gate on git-demo with exit 3, got %d", code)
	}

	// The secret canary never reaches a sink.
	canary := "aurum-canary-442-integration"
	cmd := exec.Command(binPath, "review", "--base", "HEAD~1", "--seguranca")
	cmd.Dir = demoRepo
	cmd.Env = append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixture, "AURUM_SECRET_CANARY="+canary)
	var canOut, canErr bytes.Buffer
	cmd.Stdout = &canOut
	cmd.Stderr = &canErr
	if err := cmd.Run(); err != nil {
		t.Fatalf("canary run failed: %v\nstderr=%s", err, canErr.String())
	}
	if strings.Contains(canOut.String(), canary) || strings.Contains(canErr.String(), canary) {
		t.Fatal("the secret canary must never reach stdout or stderr")
	}
}
