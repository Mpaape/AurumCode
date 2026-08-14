// AUR-458 unit selector. DISTINCT ASSERTION: the exit CODE alone, for
// every situation this card classifies as "did not review", plus the two
// situations it deliberately classifies as "reviewed" (exit 0). It makes
// no claim about stdout composition (that is tests/integration/AUR-458.go)
// and none about the CI gate's precedence (tests/e2e/AUR-458.sh).
package unit

import (
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

// aur458Bin builds the real binary once per run.
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

// TestAUR458 is the exit-code table. Every row is a situation a user can
// reach, and the only thing asserted is the number the shell sees --
// because that number is the entire defect: a CI job reads the exit code,
// not stderr.
func TestAUR458(t *testing.T) {
	root := aur458Root(t)
	bin := aur458Bin(t, root)
	demo := filepath.Join(root, "tests", "fixtures", "repos", "git-demo", "repo.git")
	if _, err := os.Stat(demo); err != nil {
		t.Skipf("infrastructure: demo fixture missing: %v", err)
	}
	goodFixture := filepath.Join(root, "tests", "fixtures", "review", "known-problem-response.json")

	dir := t.TempDir()
	badJSON := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badJSON, []byte("not json at all {{{"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// noProvider is the environment with every provider-selecting variable
	// removed, so "nothing configured" is never an accident of the harness.
	noProvider := []string{}
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "AURUMCODE_LLM_FIXTURE="),
			strings.HasPrefix(kv, "LLM_API_KEY="),
			strings.HasPrefix(kv, "LLM_BASE_URL="),
			strings.HasPrefix(kv, "LLM_MODEL="):
		default:
			noProvider = append(noProvider, kv)
		}
	}

	cases := []struct {
		name string
		env  []string
		args []string
		want int
		why  string
	}{
		{
			name: "provider absent, quality review demanded",
			env:  noProvider,
			args: []string{"review", "--base", "HEAD~1"},
			want: 1,
			why:  "did not review: nothing configured and no deterministic pass asked for",
		},
		{
			name: "provider absent, --seguranca only (THE DEMO PATH)",
			env:  noProvider,
			args: []string{"review", "--base", "HEAD~1", "--seguranca"},
			want: 0,
			why:  "reviewed exactly what was asked: the deterministic pass, which needs no model",
		},
		{
			name: "provider absent, --seguranca, quality required",
			env:  noProvider,
			args: []string{"review", "--base", "HEAD~1", "--seguranca", "--exigir-qualidade"},
			want: 1,
			why:  "did not review: the caller opted in to demanding the quality half",
		},
		{
			name: "provider configured but the call fails",
			env:  append(append([]string{}, noProvider...), "LLM_API_KEY=k", "LLM_BASE_URL=http://127.0.0.1:9/v1"),
			args: []string{"review", "--base", "HEAD~1", "--seguranca"},
			want: 1,
			why:  "did not review: the provider was reachable-in-intent and failed",
		},
		{
			name: "response does not parse",
			env:  append(append([]string{}, noProvider...), "AURUMCODE_LLM_FIXTURE="+badJSON),
			args: []string{"review", "--base", "HEAD~1", "--seguranca"},
			want: 1,
			why:  "did not review: the answer was unintelligible",
		},
		{
			name: "--limite refuses before the call",
			env:  append(append([]string{}, noProvider...), "AURUMCODE_LLM_FIXTURE="+goodFixture),
			args: []string{"review", "--base", "HEAD~1", "--seguranca", "--limite", "0.0000001"},
			want: 1,
			why:  "did not review: the budget gate refused, so no model was ever called",
		},
		{
			name: "reviewed and clean",
			env:  append(append([]string{}, noProvider...), "AURUMCODE_LLM_FIXTURE="+goodFixture),
			args: []string{"review", "--base", "HEAD~1"},
			want: 0,
			why:  "reviewed: the model answered and the answer was understood",
		},
		{
			name: "--exigir-qualidade is refused on the --pr path",
			env:  noProvider,
			args: []string{"review", "--pr", "1", "--repo", "a/b", "--publicar", "--na-linha", "--exigir-qualidade"},
			want: 2,
			why:  "usage error: the flag guards the --base path only and is never silently dropped",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Dir = demo
			cmd.Env = tc.env
			err := cmd.Run()
			got := 0
			if ee, ok := err.(*exec.ExitError); ok {
				got = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("running the command: %v", err)
			}
			if got != tc.want {
				t.Fatalf("exit code %d, want %d (%s)", got, tc.want, tc.why)
			}
		})
	}
}
