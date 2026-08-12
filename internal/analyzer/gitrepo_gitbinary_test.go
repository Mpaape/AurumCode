package analyzer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These tests exercise the git-binary path in gitrepo.go: they build a real
// repository with `git` itself and prove OpenRepo/Diff work against the two
// shapes the pure-Go loose-object reader cannot handle -- a linked worktree
// (".git" is a file, not a directory) and a repository whose objects have
// been packed by `git gc`. They skip, rather than silently pass, when no
// `git` binary is on PATH (this card's own sealed acceptance sandbox has
// none): AUR-430's required citations (TestAUR430, IntegrationAUR430) never
// skip and always run the pure-Go path for real, so the card's own gate is
// unaffected either way.

func requireGitBinary(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH; this test only proves gitrepo.go's git-binary path")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=AurumCode Test", "GIT_AUTHOR_EMAIL=test@aurumcode.invalid",
		"GIT_COMMITTER_NAME=AurumCode Test", "GIT_COMMITTER_EMAIL=test@aurumcode.invalid",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFileOrFail(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// twoCommitRepo creates a repository at dir with two commits that change
// a.txt from "one\n" to "two\n" -- the minimal fixture both tests below
// need to exercise a real, non-empty diff.
func twoCommitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "a.txt"), "one\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-q", "-m", "one")
	writeFileOrFail(t, filepath.Join(dir, "a.txt"), "two\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-q", "-m", "two")
}

func TestOpenRepo_GitBinary_LinkedWorktree(t *testing.T) {
	requireGitBinary(t)

	main := t.TempDir()
	twoCommitRepo(t, main)

	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "-q", "--detach", worktree, "HEAD")

	if info, err := os.Lstat(filepath.Join(worktree, ".git")); err != nil || info.IsDir() {
		t.Fatalf("test setup: expected %s/.git to be a file (linked worktree), got err=%v isDir=%v", worktree, err, err == nil && info.IsDir())
	}

	repo, err := OpenRepo(worktree)
	if err != nil {
		t.Fatalf("OpenRepo(linked worktree): %v", err)
	}
	diff, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff over a linked worktree: %v", err)
	}
	if len(diff.Files) != 1 || diff.Files[0].Path != "a.txt" {
		t.Fatalf("expected exactly a.txt to have changed, got %+v", diff.Files)
	}
}

func TestOpenRepo_GitBinary_PackedObjects(t *testing.T) {
	requireGitBinary(t)

	dir := t.TempDir()
	twoCommitRepo(t, dir)

	// Force every object into a packfile so no loose objects remain; the
	// pure-Go reader would fail this, the git-binary reader must not.
	runGit(t, dir, "gc", "-q", "--aggressive")
	looseDirs, err := filepath.Glob(filepath.Join(dir, ".git", "objects", "??"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(looseDirs) != 0 {
		t.Fatalf("test setup: expected no loose object directories after gc, found %v", looseDirs)
	}

	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	diff, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff over packed objects: %v", err)
	}
	if len(diff.Files) != 1 || diff.Files[0].Path != "a.txt" {
		t.Fatalf("expected exactly a.txt to have changed, got %+v", diff.Files)
	}
}

func TestResolveRef_GitBinary(t *testing.T) {
	requireGitBinary(t)

	dir := t.TempDir()
	twoCommitRepo(t, dir)

	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	head, err := repo.ResolveRef("HEAD")
	if err != nil {
		t.Fatalf("ResolveRef(HEAD): %v", err)
	}
	if !hexSHA.MatchString(head) {
		t.Fatalf("ResolveRef(HEAD) = %q, not a 40-hex SHA", head)
	}

	parent, err := repo.ResolveRef("HEAD~1")
	if err != nil {
		t.Fatalf("ResolveRef(HEAD~1): %v", err)
	}
	if parent == head {
		t.Fatal("HEAD~1 resolved to the same commit as HEAD")
	}

	if _, err := repo.ResolveRef("does-not-exist"); err == nil {
		t.Fatal("expected an error resolving a nonexistent ref")
	}
}
