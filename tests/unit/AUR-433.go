package unit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// aur433Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-433.sh); running the test directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur433Root(t *testing.T) string {
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

// aur433Fixture writes a deterministic offline model response with one
// finding of the given severity and returns its path. The shape matches
// what internal/prompt.ResponseParser validates, and the finding cites an
// embedded rule so it survives the AUR-434 rule-citation gate.
func aur433Fixture(t *testing.T, severity string) string {
	t.Helper()
	content := fmt.Sprintf(`{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": %q,
      "rule_id": "security/hardcoded-secret",
      "message": "A planted, synthetic problem used to exercise --limite."
    }
  ],
  "summary": "Deterministic offline response for AUR-433."
}`, severity)
	path := filepath.Join(t.TempDir(), "response-"+severity+".json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
	return path
}

// aur433BaseEnv returns the process environment with every variable that
// influences cmd/aurumcode's provider selection or cost pricing removed,
// so each case states its configuration explicitly and inherits nothing
// from the harness by accident.
func aur433BaseEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":           true,
		"AURUMCODE_PROMPT_CAPTURE":        true,
		"LLM_API_KEY":                     true,
		"LLM_BASE_URL":                    true,
		"LLM_MODEL":                       true,
		"AURUMCODE_LLM_INPUT_USD_PER_1K":  true,
		"AURUMCODE_LLM_OUTPUT_USD_PER_1K": true,
		"AURUM_SECRET_CANARY":             true,
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

var (
	aur433EstimatedRe = regexp.MustCompile(`estimated cost \$[0-9]+\.[0-9]{4}, diff-only pre-flight \(--limite \$[0-9]+\.[0-9]{4}\)`)
	aur433ActualRe    = regexp.MustCompile(`actual cost \$[0-9]+\.[0-9]{4} \(--limite \$[0-9]+\.[0-9]{4}\)`)
)

// TestAUR433 proves the --limite cost cap of `aurumcode review`
// behaviorally, through the real binary: the estimated cost prints before
// the model can have been called, an over-limit run refuses -- calling the
// model zero times, spending nothing -- a within-limit run reports both
// the estimate and the real cost, --limite composes with --fail-on,
// --modelo and --seguranca, and without the flag every prior contract
// holds byte for byte. See docs/specs/AUR-433.md.
func TestAUR433(t *testing.T) {
	root := aur433Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur433")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	run := func(extraEnv []string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur433BaseEnv(), extraEnv...)
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

	// callCount runs with AURUMCODE_PROMPT_CAPTURE pointed at a fresh path
	// inside t.TempDir() and reports whether the model was called: with one
	// provider and no fallback configured, the capture file's existence
	// after the run IS this card's call counter (see
	// tests/acceptance/AUR-433.sh's header note).
	callCount := func(extraEnv []string, extraArgs ...string) (code int, stdout, stderr string, called bool) {
		capturePath := filepath.Join(t.TempDir(), "capture.txt")
		env := append([]string{"AURUMCODE_PROMPT_CAPTURE=" + capturePath}, extraEnv...)
		code, stdout, stderr = run(env, extraArgs...)
		_, statErr := os.Stat(capturePath)
		called = statErr == nil
		return
	}

	var baselineStdout string

	t.Run("no --limite keeps the published contract byte for byte", func(t *testing.T) {
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr := run(env)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the finding on stdout, got:\n%s", stdout)
		}
		if strings.Contains(stderr, "cost") {
			t.Fatalf("without --limite no cost line may appear, got:\n%s", stderr)
		}
		baselineStdout = stdout
	})

	t.Run("--limite well above the cost proceeds and reports both figures", func(t *testing.T) {
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr, called := callCount(env, "--limite", "0.50")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !called {
			t.Fatalf("expected the model to be called when the estimate fits --limite")
		}
		if !aur433EstimatedRe.MatchString(stderr) {
			t.Fatalf("expected an estimated-cost line on stderr, got:\n%s", stderr)
		}
		if !aur433ActualRe.MatchString(stderr) {
			t.Fatalf("expected an actual-cost line on stderr, got:\n%s", stderr)
		}
		if stdout != baselineStdout {
			t.Fatalf("stdout must stay byte-identical with --limite:\nwith=%q\nwithout=%q", stdout, baselineStdout)
		}

		// Determinism: AC-001 requires the same input to produce the same
		// output, and stderr is where --limite's own new output lands, so
		// it must be compared too, not only stdout.
		code2, stdout2, stderr2, _ := callCount(env, "--limite", "0.50")
		if code2 != code || stdout2 != stdout {
			t.Fatalf("not deterministic: first exit=%d out=%q, second exit=%d out=%q", code, stdout, code2, stdout2)
		}
		if stderr2 != stderr {
			t.Fatalf("cost lines on stderr are not deterministic:\nfirst=%q\nsecond=%q", stderr, stderr2)
		}
	})

	t.Run("--limite far below the cost refuses and calls the model zero times", func(t *testing.T) {
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr, called := callCount(env, "--limite", "0.0001")
		if code != 1 {
			t.Fatalf("expected exit 1 for an over-limit run, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if called {
			t.Fatalf("MUT-001: the model must not be called when the estimate exceeds --limite, but it was")
		}
		if strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("an over-limit run must not print a finding it never paid for, got:\n%s", stdout)
		}
		if !aur433EstimatedRe.MatchString(stderr) {
			t.Fatalf("expected the pre-flight estimate still on stderr, got:\n%s", stderr)
		}
		if strings.Contains(stderr, "actual cost") {
			t.Fatalf("nothing was spent, so no actual-cost line may appear, got:\n%s", stderr)
		}
		if !strings.Contains(stderr, "refusing to call the model") {
			t.Fatalf("expected a clear refusal message, got:\n%s", stderr)
		}
	})

	t.Run("a --limite between the pre-flight estimate and the enforced check still refuses", func(t *testing.T) {
		// On the git-demo fixture with the default price the diff-only
		// pre-flight estimate is ~$0.0914, but the enforced check (the
		// larger, fully assembled prompt) does not admit a request until
		// --limite reaches ~$0.11: $0.10 must still refuse even though the
		// PRINTED estimate is smaller than $0.10. This is the exact margin
		// printCostEstimate/reportBudgetExceeded's wording is designed not
		// to contradict (see docs/specs/AUR-433.md).
		//
		// $0.10 is pinned to the CURRENT size of the prompt template
		// internal/prompt.PromptBuilder assembles. If that template grows or
		// shrinks in a future card, this boundary moves and a failure here
		// means "re-derive the boundary", not "AUR-433 regressed".
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr, called := callCount(env, "--limite", "0.10")
		if code != 1 {
			t.Fatalf("expected exit 1 at the boundary, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if called {
			t.Fatalf("MUT-001: the model must not be called at the boundary --limite, but it was")
		}
		if !aur433EstimatedRe.MatchString(stderr) {
			t.Fatalf("expected the pre-flight estimate on stderr, got:\n%s", stderr)
		}
		if !strings.Contains(stderr, "refusing to call the model") {
			t.Fatalf("expected a refusal, got:\n%s", stderr)
		}
		// The refusal line must not itself restate a specific dollar amount
		// as "the" cost that was exceeded -- the printed pre-flight
		// estimate ($0.0914) is smaller than this --limite ($0.10), so a
		// message like "estimated cost exceeds --limite" would read as
		// self-contradicting next to the estimate line above it.
		if strings.Contains(stderr, "estimated cost exceeds") {
			t.Fatalf("refusal message must not attribute the decision to the printed estimate, got:\n%s", stderr)
		}
	})

	t.Run("bad --limite values are usage errors", func(t *testing.T) {
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		for _, args := range [][]string{
			{"--limite="},
			{"--limite", ""},
			{"--limite", "not-a-number"},
			{"--limite", "0"},
			{"--limite", "-1"},
		} {
			code, stdout, stderr := run(env, args...)
			if code != 2 {
				t.Fatalf("args=%q: expected usage exit 2, got %d\nstdout=%s\nstderr=%s", args, code, stdout, stderr)
			}
		}
	})

	t.Run("the secret canary never reaches the new cost lines", func(t *testing.T) {
		// The card's security clause: AURUM_SECRET_CANARY must not appear
		// in stdout or stderr. This card's only new output is the two cost
		// lines and the refusal message, both plain numbers and static
		// text with no diff or prompt content in them, but the property is
		// checked directly here rather than assumed from that shape --
		// once for the allowed path (cost lines print) and once for the
		// refused path (only the estimate and the refusal print).
		canary := "AURUM-CANARY-AUR433-SPOT-CHECK-0099"
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture, "AURUM_SECRET_CANARY=" + canary}

		code, stdout, stderr := run(env, "--limite", "0.50")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if strings.Contains(stdout, canary) || strings.Contains(stderr, canary) {
			t.Fatalf("canary leaked on the allowed path:\nstdout=%s\nstderr=%s", stdout, stderr)
		}

		code, stdout, stderr = run(env, "--limite", "0.0001")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if strings.Contains(stdout, canary) || strings.Contains(stderr, canary) {
			t.Fatalf("canary leaked on the refused path:\nstdout=%s\nstderr=%s", stdout, stderr)
		}
	})

	t.Run("composes with --fail-on", func(t *testing.T) {
		fixture := aur433Fixture(t, "error")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr := run(env, "--limite", "0.50", "--fail-on", "high")
		if code != 3 {
			t.Fatalf("expected the gate's exit 3 with --limite, got %d\nstdout=%s", code, stdout)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the finding to still be printed, got:\n%s", stdout)
		}
		if !aur433ActualRe.MatchString(stderr) {
			t.Fatalf("expected the actual-cost line even when the gate closes, got:\n%s", stderr)
		}
	})

	t.Run("composes with --modelo", func(t *testing.T) {
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, _, stderr := run(env, "--limite", "0.50", "--modelo", "local")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, `reviewing with model "local"`) {
			t.Fatalf("expected the AUR-436 selection note, got:\n%s", stderr)
		}
		if !aur433EstimatedRe.MatchString(stderr) {
			t.Fatalf("expected the cost estimate alongside the selection note, got:\n%s", stderr)
		}
	})

	t.Run("composes with --seguranca", func(t *testing.T) {
		fixture := aur433Fixture(t, "warning")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, stderr := run(env, "--limite", "0.50", "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stdout, "Security findings (standards/security-review):") {
			t.Fatalf("expected the AUR-435 section, got:\n%s", stdout)
		}
	})
}
