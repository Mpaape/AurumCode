package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func aur431IntegrationRoot(t *testing.T) string {
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

// IntegrationAUR431 exercises the --fail-on CI gate exactly as a pipeline
// would: build the real cmd/aurumcode binary, run it against the
// tests/fixtures/repos/git-demo bare repository with a deterministic
// offline model response (AURUMCODE_LLM_FIXTURE, see review.FakeProvider),
// and assert on nothing but the observable contract -- exit codes and
// output. See docs/specs/AUR-431.md.
func IntegrationAUR431(t *testing.T) {
	root := aur431IntegrationRoot(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur431")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	writeFixture := func(severity string) string {
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

	run := func(fixture string, extraArgs ...string) (int, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixture)
		var stdout, stderr bytes.Buffer
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
		return code, stdout.String()
	}

	errorFixture := writeFixture("error")
	infoFixture := writeFixture("info")

	// The CI-gate happy path: an error-severity finding at --fail-on high
	// makes the command exit 3, with the finding still reported.
	code1, out1 := run(errorFixture, "--fail-on", "high")
	if code1 != 3 {
		t.Fatalf("expected exit 3 for an error finding at --fail-on high, got %d\n%s", code1, out1)
	}
	if !bytes.Contains([]byte(out1), []byte("config/demo-tokens.txt")) {
		t.Fatalf("expected the finding on stdout, got:\n%s", out1)
	}

	// Determinism: same input, same output, same exit.
	code2, out2 := run(errorFixture, "--fail-on", "high")
	if code2 != code1 || out2 != out1 {
		t.Fatalf("gated review is not deterministic:\nfirst  exit=%d out=%q\nsecond exit=%d out=%q",
			code1, out1, code2, out2)
	}

	// Below the threshold the gate stays open: info-only findings at
	// --fail-on high exit 0.
	code3, out3 := run(infoFixture, "--fail-on", "high")
	if code3 != 0 {
		t.Fatalf("expected exit 0 for info-only findings at --fail-on high, got %d\n%s", code3, out3)
	}

	// The pre-existing contract (AUR-430) is untouched: without --fail-on
	// the same review exits 0 with byte-identical stdout.
	code4, out4 := run(errorFixture)
	if code4 != 0 {
		t.Fatalf("expected exit 0 without --fail-on, got %d\n%s", code4, out4)
	}
	if out4 != out1 {
		t.Fatalf("stdout must not change when --fail-on is added:\nwith=%q\nwithout=%q", out1, out4)
	}

	// An unknown level fails closed as a usage error, exit 2.
	code5, _ := run(errorFixture, "--fail-on", "catastrophic")
	if code5 != 2 {
		t.Fatalf("expected usage exit 2 for an unknown --fail-on level, got %d", code5)
	}
}
