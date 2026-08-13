package unit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur431Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-431.sh); running the test directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur431Root(t *testing.T) string {
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

// aur431Fixture writes a deterministic offline model response whose single
// finding has the given severity, and returns its path. The response shape
// matches what internal/prompt.ResponseParser validates: file, line,
// severity in {error, warning, info}, message.
func aur431Fixture(t *testing.T, severity string) string {
	t.Helper()
	content := fmt.Sprintf(`{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": %q,
      "message": "A planted, synthetic problem used to exercise the severity gate."
    }
  ],
  "summary": "Deterministic offline response for AUR-431."
}`, severity)
	path := filepath.Join(t.TempDir(), "response-"+severity+".json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
	return path
}

// TestAUR431 proves the --fail-on severity gate of `aurumcode review`
// behaviorally, through the real binary: the command exits 3 exactly when a
// finding sits at the chosen severity or above, exits 0 when none does,
// rejects unknown levels with a usage error, and leaves the pre-existing
// no-flag contract (exit 0, identical stdout) untouched. Levels are the
// engine's own severity names (error, warning, info) plus the
// CI-conventional aliases high, medium, low. See docs/specs/AUR-431.md.
func TestAUR431(t *testing.T) {
	root := aur431Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur431")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	run := func(fixture string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixture)
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

	cases := []struct {
		name     string
		severity string
		failOn   string
		wantExit int
	}{
		{"error finding trips --fail-on high", "error", "high", 3},
		{"error finding trips its own name as level", "error", "error", 3},
		{"error finding trips the lowest threshold too", "error", "low", 3},
		{"warning finding stays below high", "warning", "high", 0},
		{"warning finding trips --fail-on medium", "warning", "medium", 3},
		{"warning finding trips --fail-on warning", "warning", "warning", 3},
		{"info finding stays below medium", "info", "medium", 0},
		{"info finding trips --fail-on low", "info", "low", 3},
		{"info finding trips --fail-on info", "info", "info", 3},
		{"levels are case-insensitive", "error", "HIGH", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := aur431Fixture(t, tc.severity)
			code, stdout, stderr := run(fixture, "--fail-on", tc.failOn)
			if code != tc.wantExit {
				t.Fatalf("severity=%s --fail-on=%s: expected exit %d, got %d\nstdout=%s\nstderr=%s",
					tc.severity, tc.failOn, tc.wantExit, code, stdout, stderr)
			}
			if !strings.Contains(stdout, "config/demo-tokens.txt") {
				t.Fatalf("expected the finding to still be printed on stdout, got:\n%s", stdout)
			}
		})
	}

	t.Run("no --fail-on keeps the AUR-430 contract", func(t *testing.T) {
		fixture := aur431Fixture(t, "error")
		gatedCode, gatedStdout, _ := run(fixture, "--fail-on", "high")
		if gatedCode != 3 {
			t.Fatalf("expected gated exit 3, got %d", gatedCode)
		}
		plainCode, plainStdout, _ := run(fixture)
		if plainCode != 0 {
			t.Fatalf("expected exit 0 without --fail-on, got %d", plainCode)
		}
		if plainStdout != gatedStdout {
			t.Fatalf("stdout must be identical with and without --fail-on:\nwith=%q\nwithout=%q",
				gatedStdout, plainStdout)
		}
	})

	t.Run("gate reports itself on stderr, never stdout", func(t *testing.T) {
		fixture := aur431Fixture(t, "error")
		_, stdout, stderr := run(fixture, "--fail-on", "high")
		if !strings.Contains(stderr, "--fail-on") {
			t.Fatalf("expected the gate to explain itself on stderr, got:\n%s", stderr)
		}
		if strings.Contains(stdout, "--fail-on") {
			t.Fatalf("the gate note must not pollute stdout, got:\n%s", stdout)
		}
	})

	t.Run("unknown level is a usage error", func(t *testing.T) {
		fixture := aur431Fixture(t, "error")
		code, _, stderr := run(fixture, "--fail-on", "catastrophic")
		if code != 2 {
			t.Fatalf("expected usage exit 2 for an unknown level, got %d\nstderr=%s", code, stderr)
		}
	})
}
