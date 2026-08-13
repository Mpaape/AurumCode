package unit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur436Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-436.sh); running the test directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur436Root(t *testing.T) string {
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

// aur436Fixture writes a deterministic offline model response with one
// finding of the given severity and returns its path. The shape matches
// what internal/prompt.ResponseParser validates.
func aur436Fixture(t *testing.T, severity string) string {
	t.Helper()
	content := fmt.Sprintf(`{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": %q,
      "message": "A planted, synthetic problem used to exercise model selection."
    }
  ],
  "summary": "Deterministic offline response for AUR-436."
}`, severity)
	path := filepath.Join(t.TempDir(), "response-"+severity+".json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
	return path
}

// aur436BaseEnv returns the process environment with every variable that
// influences cmd/aurumcode's provider selection removed, so each case
// states its provider configuration explicitly and inherits none by
// accident from the harness.
func aur436BaseEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE": true,
		"LLM_API_KEY":           true,
		"LLM_BASE_URL":          true,
		"LLM_MODEL":             true,
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

// TestAUR436 proves the --modelo model selection of `aurumcode review`
// behaviorally, through the real binary: the flag chooses which model
// reviews (including a local one, exercised offline through the
// deterministic provider), an unavailable model produces a clear,
// actionable error on stderr with exit 1 -- never an empty review with
// exit 0 -- and without the flag every contract published by AUR-430 and
// AUR-431 holds byte for byte. See docs/specs/AUR-436.md.
func TestAUR436(t *testing.T) {
	root := aur436Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur436")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	run := func(extraEnv []string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur436BaseEnv(), extraEnv...)
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

	t.Run("modelo local reviews through the offline provider", func(t *testing.T) {
		fixture := aur436Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr := run(env, "--modelo", "local")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the finding on stdout, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, `reviewing with model "local"`) {
			t.Fatalf("expected the selection note on stderr, got:\n%s", stderr)
		}
		if strings.Contains(stdout, "reviewing with model") {
			t.Fatalf("the selection note must not pollute stdout, got:\n%s", stdout)
		}

		// Determinism: same input, same output, same exit.
		code2, stdout2, _ := run(env, "--modelo", "local")
		if code2 != code || stdout2 != stdout {
			t.Fatalf("selection is not deterministic:\nfirst  exit=%d out=%q\nsecond exit=%d out=%q",
				code, stdout, code2, stdout2)
		}
	})

	t.Run("the flag commands which model is selected", func(t *testing.T) {
		fixture := aur436Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		_, _, stderr := run(env, "--modelo", "qwen2.5-coder")
		if !strings.Contains(stderr, `reviewing with model "qwen2.5-coder"`) {
			t.Fatalf("expected the note to name the chosen model, got:\n%s", stderr)
		}
	})

	t.Run("no --modelo keeps the AUR-430 contract byte for byte", func(t *testing.T) {
		fixture := aur436Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		withCode, withStdout, _ := run(env, "--modelo", "local")
		if withCode != 0 {
			t.Fatalf("expected exit 0 with --modelo, got %d", withCode)
		}
		plainCode, plainStdout, plainStderr := run(env)
		if plainCode != 0 {
			t.Fatalf("expected exit 0 without --modelo, got %d", plainCode)
		}
		if plainStdout != withStdout {
			t.Fatalf("stdout must be identical with and without --modelo:\nwith=%q\nwithout=%q",
				withStdout, plainStdout)
		}
		if strings.Contains(plainStderr, "reviewing with model") {
			t.Fatalf("without --modelo no selection note may appear, got:\n%s", plainStderr)
		}
	})

	t.Run("unavailable model errors clearly instead of an empty review", func(t *testing.T) {
		// No fixture, no LLM_API_KEY/LLM_BASE_URL: nothing can serve the
		// chosen model. MUT-001's target: this must never become "No
		// issues found." with exit 0.
		code, stdout, stderr := run(nil, "--modelo", "local")
		if code != 1 {
			t.Fatalf("expected exit 1 for an unavailable model, got %d\nstdout=%s\nstderr=%s",
				code, stdout, stderr)
		}
		if strings.Contains(stdout, "No issues found.") {
			t.Fatalf("an unavailable model must not report an empty review, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, `model "local" is unavailable`) {
			t.Fatalf("expected the error to name the model, got:\n%s", stderr)
		}
		for _, hint := range []string{"AURUMCODE_LLM_FIXTURE", "LLM_BASE_URL", "LLM_API_KEY"} {
			if !strings.Contains(stderr, hint) {
				t.Fatalf("expected the error to say how to configure (%s), got:\n%s", hint, stderr)
			}
		}
	})

	t.Run("no --modelo keeps the AUR-430 no-provider error", func(t *testing.T) {
		code, _, stderr := run(nil)
		if code != 1 {
			t.Fatalf("expected exit 1 without any provider, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "no LLM provider configured") {
			t.Fatalf("expected the pre-existing AUR-430 error text, got:\n%s", stderr)
		}
		if strings.Contains(stderr, "is unavailable") {
			t.Fatalf("the --modelo error text must not leak into the no-flag path, got:\n%s", stderr)
		}
	})

	t.Run("explicitly empty --modelo is a usage error", func(t *testing.T) {
		fixture := aur436Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		for _, args := range [][]string{
			{"--modelo="},
			{"--modelo", ""},
		} {
			code, stdout, stderr := run(env, args...)
			if code != 2 {
				t.Fatalf("args=%q: expected usage exit 2 for an empty model name, got %d\nstdout=%s\nstderr=%s",
					args, code, stdout, stderr)
			}
		}
	})

	t.Run("composes with the AUR-431 gate", func(t *testing.T) {
		fixture := aur436Fixture(t, "error")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, _ := run(env, "--modelo", "local", "--fail-on", "high")
		if code != 3 {
			t.Fatalf("expected the gate's exit 3 with --modelo, got %d\nstdout=%s", code, stdout)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the finding to still be printed, got:\n%s", stdout)
		}
	})
}
