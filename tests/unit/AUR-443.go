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

// aur443EnvWithPath is aur443BaseEnv but with PATH replaced instead of
// duplicated: os/exec does not dedupe a repeated key, so which PATH entry
// ultimately wins would be implementation-defined if it were merely
// appended.
func aur443EnvWithPath(path string, extra ...string) []string {
	drop := map[string]bool{
		"AURUM_SECRET_CANARY":   true,
		"AURUMCODE_LLM_FIXTURE": true,
		"LLM_API_KEY":           true,
		"LLM_BASE_URL":          true,
		"PATH":                  true,
	}
	env := make([]string, 0, len(os.Environ())+len(extra)+1)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !drop[name] {
			env = append(env, kv)
		}
	}
	env = append(env, "PATH="+path)
	return append(env, extra...)
}

// aur443NoGitPath builds a PATH entry that exposes go/bash/sh (whatever
// os/exec itself needs to keep functioning) but never `git`, so
// exec.LookPath("git") inside internal/analyzer.OpenRepo fails and the
// pure-Go backend is selected -- the shape of this card's own sealed
// acceptance profile (bootstrap-readonly-v1: Go and bash, no git,
// documented in internal/analyzer/gitrepo.go's package doc).
func aur443NoGitPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"go", "bash", "sh"} {
		src, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := os.Symlink(src, filepath.Join(dir, name)); err != nil {
			t.Fatalf("linking %s into the git-less PATH: %v", name, err)
		}
	}
	return dir
}

// aur443UnreadableRefFixture copies tests/fixtures/repos/git-demo/repo.git
// into a writable temp directory and revokes read permission on its
// refs/heads/main file, reproducing the exact EACCES the reviewer found on
// AUR-443's first cut: a ref that exists but this process cannot read.
func aur443UnreadableRefFixture(t *testing.T, root string) string {
	t.Helper()
	src := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	dst := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("cp", "-R", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("staging a writable fixture copy: %v\n%s", err, out)
	}
	refFile := filepath.Join(dst, "refs", "heads", "main")
	if err := os.Chmod(refFile, 0o000); err != nil {
		t.Fatalf("chmod 000 %s: %v", refFile, err)
	}
	// Restore read permission before the temp directory's own removal:
	// removing a directory entry only depends on the parent directory's
	// permissions, not the file's own mode, so this is not required for
	// cleanup to succeed -- it just leaves the fixture inspectable if a
	// failure leaves it behind.
	t.Cleanup(func() { os.Chmod(refFile, 0o644) })
	return dst
}

// aur443HierarchicalRefFixture copies tests/fixtures/repos/git-demo/repo.git
// into a writable temp directory and adds refs/heads/feature/sub (with
// main's own content, so it is a coherent, resolvable ref), while leaving
// refs/heads/main in place. This reproduces the exact shape review's
// vector 3 found: "--base feature" against a repository whose only branch
// under that name is the hierarchical "feature/sub" resolves
// refs/heads/feature to a DIRECTORY, not a ref file.
func aur443HierarchicalRefFixture(t *testing.T, root string) string {
	t.Helper()
	src := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	dst := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("cp", "-R", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("staging a writable fixture copy: %v\n%s", err, out)
	}
	mainRef := filepath.Join(dst, "refs", "heads", "main")
	content, err := os.ReadFile(mainRef)
	if err != nil {
		t.Fatalf("reading %s: %v", mainRef, err)
	}
	hierDir := filepath.Join(dst, "refs", "heads", "feature")
	if err := os.MkdirAll(hierDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", hierDir, err)
	}
	if err := os.WriteFile(filepath.Join(hierDir, "sub"), content, 0o644); err != nil {
		t.Fatalf("writing refs/heads/feature/sub: %v", err)
	}
	return dst
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
		if strings.Contains(stderr, "permission denied") {
			t.Fatalf("a genuinely missing ref must not be misreported as a permission problem, got:\n%s", stderr)
		}
	})

	// This is the reviewer's blocker (cmd/aurumcode/main.go's cleanRefError):
	// the pre-fix code matched ANY *fs.PathError as "ref not found",
	// including EACCES, so a ref that exists but cannot be read produced a
	// confidently wrong "not found" diagnosis instead of naming the real
	// cause. Reproduced with root excluded (chmod 000 does not block root)
	// and covering both analyzer backends, since a real `git rev-parse`
	// prints the byte-identical "fatal: Needed a single revision" for
	// ENOENT and EACCES -- string-matching git's own output could never
	// tell them apart either.
	t.Run("a ref that exists but is unreadable: permission message, never ref-not-found", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: chmod 000 does not deny root read access, so this case cannot be reproduced here")
		}
		unreadable := aur443UnreadableRefFixture(t, root)

		t.Run("pure-Go backend (no git on PATH)", func(t *testing.T) {
			env := aur443EnvWithPath(aur443NoGitPath(t))
			code, _, stderr := aur443Run(t, bin, unreadable, env, "review", "--base", "main")
			if code != 1 {
				t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, "permission denied") {
				t.Fatalf("expected a permission-denied diagnosis, got:\n%s", stderr)
			}
			if strings.Contains(stderr, "not found") {
				t.Fatalf("a permission problem must not be reported as \"not found\", got:\n%s", stderr)
			}
		})

		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("no git binary on PATH in this environment: skipping the git-binary-backend half (the pure-Go backend above is what bootstrap-readonly-v1 exercises)")
		}
		t.Run("git-binary backend", func(t *testing.T) {
			code, _, stderr := aur443Run(t, bin, unreadable, aur443BaseEnv(), "review", "--base", "main")
			if code != 1 {
				t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, "permission denied") {
				t.Fatalf("expected a permission-denied diagnosis, got:\n%s", stderr)
			}
			if strings.Contains(stderr, "not found") {
				t.Fatalf("a permission problem must not be reported as \"not found\" (git's own stderr for EACCES and ENOENT is identical, so this is exactly the case the probe exists for), got:\n%s", stderr)
			}
		})
	})

	// This is the second review-found blocker (cmd/aurumcode/main.go's
	// cleanRefError): once the "resolving base/head ref" prefix matched,
	// the prior fix still fell through to a raw %w wrap for any cause it
	// did not recognize -- and a ref that resolves to a DIRECTORY (a
	// hierarchical branch namespace, e.g. only "feature/sub" exists, never
	// a literal "feature") is exactly such a cause: EISDIR is neither
	// fs.ErrNotExist nor fs.ErrPermission. The leaked message contained
	// the literal "refs/heads/" path this card's own leak checks elsewhere
	// already treat as a defect.
	t.Run("a ref that resolves to a directory: clean message, never the raw leaked path", func(t *testing.T) {
		hier := aur443HierarchicalRefFixture(t, root)

		t.Run("pure-Go backend (no git on PATH)", func(t *testing.T) {
			env := aur443EnvWithPath(aur443NoGitPath(t))
			code, _, stderr := aur443Run(t, bin, hier, env, "review", "--base", "feature")
			if code != 1 {
				t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, `ref "feature" is not a branch`) {
				t.Fatalf("expected a clean \"is not a branch\" diagnosis, got:\n%s", stderr)
			}
			for _, leak := range []string{"no such file or directory", "refs/heads/", ".git/", "is a directory"} {
				if strings.Contains(stderr, leak) {
					t.Fatalf("expected no leaked internal detail (%q), got:\n%s", leak, stderr)
				}
			}
		})

		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("no git binary on PATH in this environment: skipping the git-binary-backend half (the pure-Go backend above is what bootstrap-readonly-v1 exercises)")
		}
		t.Run("git-binary backend produces the identical message", func(t *testing.T) {
			_, _, pureGoStderr := aur443Run(t, bin, hier, aur443EnvWithPath(aur443NoGitPath(t)), "review", "--base", "feature")

			code, _, stderr := aur443Run(t, bin, hier, aur443BaseEnv(), "review", "--base", "feature")
			if code != 1 {
				t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, `ref "feature" is not a branch`) {
				t.Fatalf("expected a clean \"is not a branch\" diagnosis, got:\n%s", stderr)
			}
			if strings.Contains(stderr, "fatal:") {
				t.Fatalf("git-binary backend leaked git's own wording:\n%s", stderr)
			}
			if stderr != pureGoStderr {
				t.Fatalf("the two backends disagree on the same user mistake:\ngit-binary: %q\npure-Go:    %q", stderr, pureGoStderr)
			}
		})
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
