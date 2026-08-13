// Package unit holds AUR-443's Unit-layer proof: a user who has never read
// the source discovers what `aurumcode` does and can run a first review,
// through the real binary's --help/--version surfaces, its unified --help
// convention, its provider-missing message, and its cleaned-up
// git-repository/ref-resolution errors -- while every pre-existing
// published surface this card must not disturb (review --base's findings
// output, docs's output, the AUR-433 --limite exit code) stays exactly as
// it was.
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office (see tests/unit/AUR-426.go's own note):
// tests/acceptance/AUR-443.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR443, so the
// assertions below run inside the sandboxed acceptance instead of being
// swept into an unrelated top-level `go test ./...`.
package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur443Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-443.sh); running the test directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur443Root(t *testing.T) string {
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

// aur443Build compiles cmd/aurumcode once and returns the binary's path.
func aur443Build(t *testing.T, root string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur443")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return binPath
}

// aur443Run runs the built binary and returns its exit code, stdout and
// stderr, never failing the test on a non-zero exit (that is the behavior
// under test).
func aur443Run(t *testing.T, bin, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
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

// aur443BaseEnv returns the process environment with AURUM_SECRET_CANARY
// and every LLM provider variable stripped, so each subtest starts from a
// known, provider-less, canary-less baseline and opts back in explicitly.
func aur443BaseEnv(extra ...string) []string {
	drop := map[string]bool{
		"AURUM_SECRET_CANARY":   true,
		"AURUMCODE_LLM_FIXTURE": true,
		"LLM_API_KEY":           true,
		"LLM_BASE_URL":          true,
	}
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !drop[name] {
			env = append(env, kv)
		}
	}
	return append(env, extra...)
}

// TestAUR443 proves, through the real binary, that a first-time user
// discovers what aurumcode does and can run a first review without reading
// source. See docs/specs/AUR-443.md for the full rationale behind each
// assertion.
func TestAUR443(t *testing.T) {
	root := aur443Root(t)
	bin := aur443Build(t, root)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}
	knownProblemFixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	if _, err := os.Stat(knownProblemFixture); err != nil {
		t.Fatalf("required input missing: %s: %v", knownProblemFixture, err)
	}

	// --- 1. Top-level --help lists the subcommands with a runnable example. ---

	t.Run("top-level --help lists subcommands on stdout, exit 0", func(t *testing.T) {
		code, stdout, stderr := aur443Run(t, bin, root, aur443BaseEnv(), "--help")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr for --help, got:\n%s", stderr)
		}
		for _, want := range []string{"review", "docs", "aurumcode review --base HEAD~1"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("expected %q in top-level --help, got:\n%s", want, stdout)
			}
		}
	})

	t.Run("-h and the bare word help behave the same as --help", func(t *testing.T) {
		_, wantStdout, _ := aur443Run(t, bin, root, aur443BaseEnv(), "--help")
		for _, arg := range []string{"-h", "help"} {
			code, stdout, _ := aur443Run(t, bin, root, aur443BaseEnv(), arg)
			if code != 0 {
				t.Fatalf("%q: expected exit 0, got %d", arg, code)
			}
			if stdout != wantStdout {
				t.Fatalf("%q: expected identical output to --help\ngot:  %q\nwant: %q", arg, stdout, wantStdout)
			}
		}
	})

	t.Run("with no top-level help this would regress to unknown command (MUT-001 rationale)", func(t *testing.T) {
		// Not a mutation itself -- tests/acceptance/AUR-443.sh's MUT-001
		// performs the actual source mutation and rebuild. This subtest
		// just documents, against the unmutated binary, that --help is
		// NOT one of the tokens "unknown command" already covers by
		// accident: an unrelated bogus token still produces exactly that
		// message, so the --help branch is doing real, exercised work.
		code, _, stderr := aur443Run(t, bin, root, aur443BaseEnv(), "--frobnicate")
		if code != 2 {
			t.Fatalf("expected exit 2 for an unrelated unknown token, got %d", code)
		}
		if !strings.Contains(stderr, `unknown command "--frobnicate"`) {
			t.Fatalf("expected the unknown-command message, got:\n%s", stderr)
		}
	})

	// --- 2. --version / version. ---

	t.Run("--version and version print a version string on stdout, exit 0", func(t *testing.T) {
		for _, arg := range []string{"--version", "version"} {
			code, stdout, stderr := aur443Run(t, bin, root, aur443BaseEnv(), arg)
			if code != 0 {
				t.Fatalf("%q: expected exit 0, got %d\nstderr=%s", arg, code, stderr)
			}
			if !strings.HasPrefix(stdout, "aurumcode ") {
				t.Fatalf("%q: expected a version string prefixed \"aurumcode \", got:\n%s", arg, stdout)
			}
			if strings.TrimSpace(stdout) == "aurumcode" {
				t.Fatalf("%q: expected a non-empty version, got:\n%s", arg, stdout)
			}
		}
	})

	// --- 3. One --help convention: review and docs agree, and with each other. ---

	t.Run("review --help is on stdout, exit 0, no stderr (same convention as docs --help)", func(t *testing.T) {
		code, stdout, stderr := aur443Run(t, bin, root, aur443BaseEnv(), "review", "--help")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr for review --help, got:\n%s", stderr)
		}
		for _, want := range []string{"-base", "-fail-on", "-modelo", "-seguranca", "-pr", "-limite", "-check"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("expected %q documented in review --help, got:\n%s", want, stdout)
			}
		}
	})

	t.Run("review -h behaves the same as review --help", func(t *testing.T) {
		_, wantStdout, _ := aur443Run(t, bin, root, aur443BaseEnv(), "review", "--help")
		code, stdout, stderr := aur443Run(t, bin, root, aur443BaseEnv(), "review", "-h")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got:\n%s", stderr)
		}
		if stdout != wantStdout {
			t.Fatalf("review -h output differs from review --help\ngot:  %q\nwant: %q", stdout, wantStdout)
		}
	})

	t.Run("docs --help is still on stdout, exit 0 (regression check)", func(t *testing.T) {
		code, stdout, stderr := aur443Run(t, bin, root, aur443BaseEnv(), "docs", "--help")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr, got:\n%s", stderr)
		}
		if !strings.Contains(stdout, "usage: aurumcode docs") {
			t.Fatalf("expected usage on stdout, got:\n%s", stdout)
		}
	})

	t.Run("a genuine usage error (not --help) is still stderr, exit 2, for both subcommands", func(t *testing.T) {
		for _, sub := range []string{"review", "docs"} {
			code, stdout, stderr := aur443Run(t, bin, root, aur443BaseEnv(), sub, "--bogus-flag-xyz")
			if code != 2 {
				t.Fatalf("%s --bogus-flag-xyz: expected exit 2, got %d\nstdout=%s\nstderr=%s", sub, code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("%s --bogus-flag-xyz: expected empty stdout, got:\n%s", sub, stdout)
			}
			if stderr == "" {
				t.Fatalf("%s --bogus-flag-xyz: expected a usage error on stderr", sub)
			}
		}
	})

	// --- 4. Provider-missing message shows the fixture's shape. ---

	t.Run("no provider configured: the message shows the fixture shape and points at a real example", func(t *testing.T) {
		code, stdout, stderr := aur443Run(t, bin, repoDir, aur443BaseEnv(), "review", "--base", "HEAD~1")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "no LLM provider configured") {
			t.Fatalf("expected the existing diagnosis preserved (tests/unit/AUR-436.go relies on this substring), got:\n%s", stderr)
		}
		for _, want := range []string{`"issues"`, `"severity"`, `"message"`, "tests/fixtures/review/known-problem-response.json"} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("expected %q in the provider-missing message, got:\n%s", want, stderr)
			}
		}
	})

	t.Run("the pointed-at fixture actually works for an offline first run", func(t *testing.T) {
		env := aur443BaseEnv("AURUMCODE_LLM_FIXTURE=" + knownProblemFixture)
		code, stdout, stderr := aur443Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the fixture's finding on stdout, got:\n%s", stdout)
		}
	})

	t.Run("--modelo unavailable: the message also points at the fixture example", func(t *testing.T) {
		code, _, stderr := aur443Run(t, bin, repoDir, aur443BaseEnv(), "review", "--base", "HEAD~1", "--modelo", "local")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(stderr, "tests/fixtures/review/known-problem-response.json") {
			t.Fatalf("expected the fixture pointer in the --modelo unavailable message, got:\n%s", stderr)
		}
	})

	// --- 5. --limite's refusal exit code: unchanged (investigated, not fixed; see docs/specs/AUR-443.md). ---

	t.Run("--limite refusal is still exit 1 (AUR-433's published contract, unchanged)", func(t *testing.T) {
		env := aur443BaseEnv("AURUMCODE_LLM_FIXTURE=" + knownProblemFixture)
		code, stdout, stderr := aur443Run(t, bin, repoDir, env, "review", "--base", "HEAD~1", "--limite", "0.0001")
		if code != 1 {
			t.Fatalf("expected exit 1 (AUR-433's documented, test-pinned contract), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "refusing to call the model") {
			t.Fatalf("expected the refusal message, got:\n%s", stderr)
		}
	})

	// --- 6. Git-repository and ref errors: one clean message, no duplication or leak. ---

	t.Run("not a git repository: one clean sentence, no duplicated phrase", func(t *testing.T) {
		outside := t.TempDir()
		code, _, stderr := aur443Run(t, bin, outside, aur443BaseEnv(), "review", "--base", "HEAD~1")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(stderr, "is not a git repository") {
			t.Fatalf("expected the not-a-git-repository diagnosis, got:\n%s", stderr)
		}
		if strings.Count(stderr, "not a git repository") != 1 {
			t.Fatalf("expected the phrase \"not a git repository\" exactly once (it was duplicated before this card), got:\n%s", stderr)
		}
	})

	t.Run("a ref that does not resolve: one clean sentence, no leaked filesystem path", func(t *testing.T) {
		code, _, stderr := aur443Run(t, bin, repoDir, aur443BaseEnv(), "review", "--base", "does-not-exist")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if !strings.Contains(stderr, `ref "does-not-exist" not found`) {
			t.Fatalf("expected a clean \"ref not found\" message, got:\n%s", stderr)
		}
		for _, leak := range []string{"no such file or directory", "refs/heads/", ".git/", "fatal:"} {
			if strings.Contains(stderr, leak) {
				t.Fatalf("expected no leaked internal detail (%q), got:\n%s", leak, stderr)
			}
		}
	})

	// --- 7. review --base's published findings output is byte-for-byte unchanged. ---

	t.Run("review --base's published contract is untouched: a non-empty summary is still never printed", func(t *testing.T) {
		fixture := filepath.Join(t.TempDir(), "response-clean.json")
		if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"Nothing to report."}`), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		env := aur443BaseEnv("AURUMCODE_LLM_FIXTURE=" + fixture)
		code, stdout, stderr := aur443Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stdout != "No issues found.\n" {
			t.Fatalf("aurumcode review's published byte-for-byte contract changed: stdout=%q", stdout)
		}
		if strings.Contains(stdout, "Nothing to report") {
			t.Fatalf("the summary field leaked onto stdout: %q", stdout)
		}
	})

	t.Run("review --base with a real finding still prints exactly the AUR-430 line shape", func(t *testing.T) {
		env := aur443BaseEnv("AURUMCODE_LLM_FIXTURE=" + knownProblemFixture)
		code, stdout, _ := aur443Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt:4: [error]") {
			t.Fatalf("expected the unchanged finding line shape, got:\n%s", stdout)
		}
	})

	// --- Determinism, one more time at the top level. ---

	t.Run("repeating --help produces the same output", func(t *testing.T) {
		_, first, _ := aur443Run(t, bin, root, aur443BaseEnv(), "--help")
		_, second, _ := aur443Run(t, bin, root, aur443BaseEnv(), "--help")
		if first != second {
			t.Fatalf("--help is not deterministic:\nfirst:  %q\nsecond: %q", first, second)
		}
	})
}
