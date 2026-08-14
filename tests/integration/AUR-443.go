// Package integration holds AUR-443's Integration-layer proof: the
// cleaned-up git-repository/ref-resolution error message is not just a
// hand-picked string match against the pure-Go backend -- it is the
// SAME text internal/analyzer's two independent backends (the pure-Go
// loose-object reader used when no `git` binary is on PATH, and the
// git-binary-shelling-out reader used when one is) produce for the exact
// same user mistake, proving cmd/aurumcode/main.go's cleanRefError
// normalizes both instead of only ever being exercised against one.
//
// This mirrors internal/analyzer's own package doc
// (gitrepo.go: "Both paths return the same information through the same
// Repo methods") one level up: this card's promise is that the TWO
// backends now also agree on what an unresolvable ref is reported as.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur443IntegrationRoot(t *testing.T) string {
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

func aur443IntegrationBuild(t *testing.T, root string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur443-integration")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return binPath
}

// aur443IntegrationRun runs the built binary with an explicit PATH, so the
// caller controls whether a `git` binary is visible to
// internal/analyzer.OpenRepo's own PATH lookup (exec.LookPath("git")) --
// the one thing that selects which of the two backends answers.
func aur443IntegrationRun(t *testing.T, bin, dir, path string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+path)
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

// aur443NoGitPath builds a PATH entry that exposes go/bash (whatever the
// test process itself needs to keep functioning, e.g. for os/exec's own
// bookkeeping) but never `git`, so exec.LookPath("git") inside
// internal/analyzer.OpenRepo fails and the pure-Go backend is selected --
// exactly the shape of this card's own sealed acceptance profile
// (bootstrap-readonly-v1: Go and bash, no git, documented in
// internal/analyzer/gitrepo.go's package doc).
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

// aur443UnreadableRefFixture copies srcRepoDir into a writable temp
// directory and revokes read permission on its refs/heads/main file,
// reproducing the exact EACCES the reviewer found on AUR-443's first cut:
// a ref that exists but this process cannot read.
func aur443UnreadableRefFixture(t *testing.T, srcRepoDir string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("cp", "-R", srcRepoDir, dst).CombinedOutput(); err != nil {
		t.Fatalf("staging a writable fixture copy: %v\n%s", err, out)
	}
	refFile := filepath.Join(dst, "refs", "heads", "main")
	if err := os.Chmod(refFile, 0o000); err != nil {
		t.Fatalf("chmod 000 %s: %v", refFile, err)
	}
	t.Cleanup(func() { os.Chmod(refFile, 0o644) })
	return dst
}

// aur443HierarchicalRefFixture copies srcRepoDir into a writable temp
// directory and adds refs/heads/feature/sub (with main's own content, so
// it is a coherent, resolvable ref), leaving refs/heads/main in place.
// This reproduces review's vector 3: "--base feature" against a
// repository whose only branch under that name is the hierarchical
// "feature/sub" resolves refs/heads/feature to a DIRECTORY, not a ref
// file.
func aur443HierarchicalRefFixture(t *testing.T, srcRepoDir string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("cp", "-R", srcRepoDir, dst).CombinedOutput(); err != nil {
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

// IntegrationAUR443 proves, through the real binary, that
// internal/analyzer's two ref-resolution backends now report the identical
// clean message for the same unresolvable ref, and that a real git binary
// (when present on PATH) is not required to reach that clean message --
// only internal/analyzer's pure-Go path, the one bootstrap-readonly-v1
// exercises, has to.
func IntegrationAUR443(t *testing.T) {
	root := aur443IntegrationRoot(t)
	bin := aur443IntegrationBuild(t, root)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	realGitOnPath, hasRealGit := "", false
	if p, err := exec.LookPath("git"); err == nil {
		realGitOnPath = filepath.Dir(p)
		hasRealGit = true
	}
	noGitPath := aur443NoGitPath(t)

	const badRef = "does-not-exist-aur443"
	wantSubstr := `ref "` + badRef + `" not found in this repository`

	t.Run("pure-Go backend (no git on PATH): clean ref-not-found message", func(t *testing.T) {
		code, _, stderr := aur443IntegrationRun(t, bin, repoDir, noGitPath, "review", "--base", badRef)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, wantSubstr) {
			t.Fatalf("expected %q, got:\n%s", wantSubstr, stderr)
		}
		if strings.Contains(stderr, "no such file or directory") || strings.Contains(stderr, "refs/heads/") {
			t.Fatalf("pure-Go backend leaked internal detail:\n%s", stderr)
		}
	})

	// This is the reviewer's blocker on AUR-443's first cut
	// (cmd/aurumcode/main.go's cleanRefError): matching ANY *fs.PathError
	// as "ref not found" also matched EACCES, so a ref that exists but
	// cannot be read produced a confidently wrong "not found" diagnosis.
	// Root is excluded because chmod 000 does not deny root read access.
	t.Run("pure-Go backend: a ref that exists but is unreadable is a permission message, not ref-not-found", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: chmod 000 does not deny root read access, so this case cannot be reproduced here")
		}
		unreadable := aur443UnreadableRefFixture(t, repoDir)
		code, _, stderr := aur443IntegrationRun(t, bin, unreadable, noGitPath, "review", "--base", "main")
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

	// This is the second review-found blocker (cmd/aurumcode/main.go's
	// cleanRefError): once the "resolving base/head ref" prefix matched,
	// the prior fix still fell through to a raw %w wrap for any cause it
	// did not recognize -- and a ref that resolves to a DIRECTORY (a
	// hierarchical branch namespace) is exactly such a cause: EISDIR is
	// neither fs.ErrNotExist nor fs.ErrPermission.
	t.Run("pure-Go backend: a ref that resolves to a directory is a clean message, never the raw leaked path", func(t *testing.T) {
		hier := aur443HierarchicalRefFixture(t, repoDir)
		code, _, stderr := aur443IntegrationRun(t, bin, hier, noGitPath, "review", "--base", "feature")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, `ref "feature" is not a branch`) {
			t.Fatalf("expected a clean \"is not a branch\" diagnosis, got:\n%s", stderr)
		}
		for _, leak := range []string{"no such file or directory", "refs/heads/", "is a directory"} {
			if strings.Contains(stderr, leak) {
				t.Fatalf("expected no leaked internal detail (%q), got:\n%s", leak, stderr)
			}
		}
	})

	if !hasRealGit {
		t.Skip("no git binary on PATH in this environment: skipping the git-binary-backend comparison (the pure-Go backend above is what bootstrap-readonly-v1 exercises and is not skipped)")
	}

	t.Run("git-binary backend: the same directory ref is also a clean message, identical to the pure-Go backend", func(t *testing.T) {
		hier := aur443HierarchicalRefFixture(t, repoDir)
		_, _, pureGoStderr := aur443IntegrationRun(t, bin, hier, noGitPath, "review", "--base", "feature")

		code, _, stderr := aur443IntegrationRun(t, bin, hier, realGitOnPath, "review", "--base", "feature")
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

	// git's own `rev-parse --verify` stderr ("fatal: Needed a single
	// revision") is byte-identical for ENOENT and EACCES -- it never
	// exposes the underlying errno -- so string-matching git's output
	// alone could never tell a missing ref from an unreadable one. This is
	// exactly why cleanRefError's permission probe is checked on both
	// backends instead of only patching the pure-Go one.
	t.Run("git-binary backend: the same unreadable ref is also a permission message, not ref-not-found", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: chmod 000 does not deny root read access, so this case cannot be reproduced here")
		}
		unreadable := aur443UnreadableRefFixture(t, repoDir)
		code, _, stderr := aur443IntegrationRun(t, bin, unreadable, realGitOnPath, "review", "--base", "main")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "permission denied") {
			t.Fatalf("expected a permission-denied diagnosis, got:\n%s", stderr)
		}
		if strings.Contains(stderr, "not found") {
			t.Fatalf("got \"not found\" for a permission problem -- git's own stderr for EACCES and ENOENT is identical, so this is exactly the case the probe exists for:\n%s", stderr)
		}
	})

	t.Run("git-binary backend and pure-Go backend report the identical message", func(t *testing.T) {
		_, _, pureGoStderr := aur443IntegrationRun(t, bin, repoDir, noGitPath, "review", "--base", badRef)

		code, _, gitBinStderr := aur443IntegrationRun(t, bin, repoDir, realGitOnPath, "review", "--base", badRef)
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, gitBinStderr)
		}
		if !strings.Contains(gitBinStderr, wantSubstr) {
			t.Fatalf("git-binary backend: expected %q, got:\n%s", wantSubstr, gitBinStderr)
		}
		if strings.Contains(gitBinStderr, "fatal:") {
			t.Fatalf("git-binary backend leaked git's own wording:\n%s", gitBinStderr)
		}
		if gitBinStderr != pureGoStderr {
			t.Fatalf("the two backends disagree on the same user mistake:\ngit-binary: %q\npure-Go:    %q", gitBinStderr, pureGoStderr)
		}
	})

	t.Run("a valid ref still resolves and reviews normally on both backends", func(t *testing.T) {
		fixture := filepath.Join(t.TempDir(), "response-clean.json")
		if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"ok"}`), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		for name, path := range map[string]string{"pure-Go": noGitPath, "git-binary": realGitOnPath} {
			cmd := exec.Command(bin, "review", "--base", "HEAD~1")
			cmd.Dir = repoDir
			cmd.Env = append(os.Environ(), "PATH="+path, "AURUMCODE_LLM_FIXTURE="+fixture)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s backend: valid ref review failed: %v\n%s", name, err, out)
			}
			if strings.TrimSpace(string(out)) != "No issues found." {
				t.Fatalf("%s backend: unexpected output for a valid ref: %q", name, out)
			}
		}
	})
}
