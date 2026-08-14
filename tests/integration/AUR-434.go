package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur434IntegrationRoot resolves the repository root, exactly like
// tests/integration/AUR-430.go does: AURUMCODE_ROOT wins (the acceptance
// harness sets it to the staged materialization root), and a direct run
// from a full checkout climbs two directories back to the root.
func aur434IntegrationRoot(t *testing.T) string {
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

const aur434IntegrationMixedFixture = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "UNGROUNDED-BAD-RULE planted finding citing a nonexistent rule."
    }
  ],
  "summary": "Mixed fixture for AUR-434."
}`

const aur434IntegrationUngroundedFixture = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    }
  ],
  "summary": "Only ungrounded findings."
}`

// IntegrationAUR434 builds the real aurumcode binary and proves AUR-434's
// outcome at the CLI boundary: every printed problem cites the sustaining
// rule from the embedded project review standard, an ungrounded finding
// never reaches stdout, and the output is deterministic across runs.
func IntegrationAUR434(t *testing.T) {
	root := aur434IntegrationRoot(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur434")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	fixtureDir := t.TempDir()
	mixedFixture := filepath.Join(fixtureDir, "mixed.json")
	if err := os.WriteFile(mixedFixture, []byte(aur434IntegrationMixedFixture), 0o600); err != nil {
		t.Fatalf("writing mixed fixture: %v", err)
	}
	ungroundedFixture := filepath.Join(fixtureDir, "ungrounded.json")
	if err := os.WriteFile(ungroundedFixture, []byte(aur434IntegrationUngroundedFixture), 0o600); err != nil {
		t.Fatalf("writing ungrounded fixture: %v", err)
	}

	runOnce := func(fixture string) string {
		cmd := exec.Command(binPath, "review", "--base", "HEAD~1")
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixture)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("aurumcode review --base HEAD~1 failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
		}
		return stdout.String()
	}

	first := runOnce(mixedFixture)
	if !strings.Contains(first, "config/demo-tokens.txt:3") {
		t.Fatalf("expected the grounded finding at config/demo-tokens.txt:3, got:\n%s", first)
	}
	if !strings.Contains(first, "(rule security/hardcoded-secret: Hardcoded Secrets)") {
		t.Fatalf("every printed problem must cite its sustaining rule, got:\n%s", first)
	}
	for _, marker := range []string{"UNGROUNDED-NO-RULE", "UNGROUNDED-BAD-RULE"} {
		if strings.Contains(first, marker) {
			t.Fatalf("ungrounded finding %s reached the user:\n%s", marker, first)
		}
	}

	second := runOnce(mixedFixture)
	if first != second {
		t.Fatalf("aurumcode review is not deterministic across runs:\nfirst=%q\nsecond=%q", first, second)
	}

	// A review where no finding can cite a rule reports the unchanged
	// AUR-430 no-findings output, not a fabricated problem.
	empty := runOnce(ungroundedFixture)
	if !strings.Contains(empty, "No issues found.") {
		t.Fatalf("expected the unchanged no-findings output when every finding is ungrounded, got:\n%s", empty)
	}
	if strings.Contains(empty, "UNGROUNDED-NO-RULE") {
		t.Fatalf("ungrounded finding reached the user:\n%s", empty)
	}
}
