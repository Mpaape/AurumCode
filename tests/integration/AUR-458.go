// AUR-458 integration selector. DISTINCT ASSERTION: the COMPOSITION of
// the two passes when the model half fails -- that the deterministic
// security pass still delivers its findings on stdout, and that the run
// never prints the clean-review sentence it has not earned. It asserts
// stdout content, which tests/unit/AUR-458.go deliberately does not, and
// it does not touch the --fail-on gate, which tests/e2e/AUR-458.sh owns.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur458Root(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func aur458Bin(t *testing.T, root string) string {
	t.Helper()
	if b := os.Getenv("AURUMCODE_BIN"); b != "" {
		return b
	}
	bin := filepath.Join(t.TempDir(), "aurumcode")
	cmd := exec.Command("go", "build", "-mod=mod", "-o", bin, "./cmd/aurumcode")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("infrastructure: cannot build aurumcode: %v\n%s", err, out)
	}
	return bin
}

const (
	aur458SecHeader = "Security findings (standards/security-review):"
	aur458Secret    = "Hardcoded credentials or API keys detected"
	aur458Clean     = "No issues found."
	aur458HalfNote  = "reviewed HALF of what was asked"
)

// IntegrationAUR458 proves the card's requisito explícito: a provider that
// FAILS must not take the deterministic security pass down with it. Before
// this card the security section vanished entirely whenever the model half
// broke -- the user lost real, already-computed security findings because
// an unrelated network call failed.
func IntegrationAUR458(t *testing.T) {
	root := aur458Root(t)
	bin := aur458Bin(t, root)
	demo := filepath.Join(root, "tests", "fixtures", "repos", "git-demo", "repo.git")
	if _, err := os.Stat(demo); err != nil {
		t.Skipf("infrastructure: demo fixture missing: %v", err)
	}

	dir := t.TempDir()
	badJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badJSON, []byte("not json at all {{{"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	base := []string{}
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "AURUMCODE_LLM_FIXTURE="),
			strings.HasPrefix(kv, "LLM_API_KEY="),
			strings.HasPrefix(kv, "LLM_BASE_URL="),
			strings.HasPrefix(kv, "LLM_MODEL="):
		default:
			base = append(base, kv)
		}
	}

	cases := []struct {
		name string
		env  []string
	}{
		{
			name: "the provider call fails",
			env:  append(append([]string{}, base...), "LLM_API_KEY=k", "LLM_BASE_URL=http://127.0.0.1:9/v1"),
		},
		{
			name: "the response does not parse",
			env:  append(append([]string{}, base...), "AURUMCODE_LLM_FIXTURE="+badJSON),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(bin, "review", "--base", "HEAD~1", "--seguranca")
			cmd.Dir = demo
			cmd.Env = tc.env
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			rc := 0
			if ee, ok := err.(*exec.ExitError); ok {
				rc = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("running the command: %v", err)
			}
			out := stdout.String()

			// The security pass survived the model's failure.
			if !strings.Contains(out, aur458SecHeader) {
				t.Fatalf("the security section must survive the quality failure; stdout was:\n%s", out)
			}
			if !strings.Contains(out, aur458Secret) {
				t.Fatalf("the security pass must still report its findings; stdout was:\n%s", out)
			}
			// ...and the exit code still says the quality half did not run.
			if rc != 1 {
				t.Fatalf("exit code %d, want 1: the security findings printed but the quality review never happened", rc)
			}
			// ...and nothing claims the code was reviewed and clean.
			if strings.Contains(out, aur458Clean) {
				t.Fatalf("stdout must never claim %q when the model never answered:\n%s", aur458Clean, out)
			}
			// ...and the user is told, in words, that this was half a review.
			if !strings.Contains(stderr.String(), aur458HalfNote) {
				t.Fatalf("stderr must say the run reviewed only half; stderr was:\n%s", stderr.String())
			}
		})
	}
}
