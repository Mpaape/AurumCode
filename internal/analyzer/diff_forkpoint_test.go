package analyzer

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// forkRepo builds main (seed, then c.txt) and feature (seed, then b.txt),
// plus an orphan commit with an unrelated history, with the git binary.
func forkRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=F", "GIT_AUTHOR_EMAIL=f@example.invalid", "GIT_COMMITTER_NAME=F", "GIT_COMMITTER_EMAIL=f@example.invalid", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("a.txt", "seed\n")
	git("add", ".")
	git("commit", "-q", "-m", "seed")
	git("switch", "-q", "-c", "feature")
	write("b.txt", "feature\n")
	git("add", ".")
	git("commit", "-q", "-m", "feature")
	git("switch", "-q", "main")
	write("c.txt", "main moved on\n")
	git("add", ".")
	git("commit", "-q", "-m", "main")
	git("switch", "-q", "--orphan", "unrelated")
	write("z.txt", "other history\n")
	git("add", ".")
	git("commit", "-q", "-m", "unrelated")
	git("switch", "-q", "feature")
	return dir
}

func diffPaths(t *testing.T, repo *Repo, base, head string) map[string]bool {
	t.Helper()
	diff, _, err := repo.Diff(base, head)
	if err != nil {
		t.Fatalf("Diff(%s, %s): %v", base, head, err)
	}
	paths := map[string]bool{}
	for _, f := range diff.Files {
		paths[f.Path] = true
	}
	return paths
}

// A branch behind its base shows only its own change, never the base's
// newer files as removed (the diff starts at the merge base, as base...head).
func TestDiffStartsAtTheMergeBase(t *testing.T) {
	repo, err := OpenRepo(forkRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	got := diffPaths(t, repo, "main", "feature")
	if !got["b.txt"] || got["c.txt"] || len(got) != 1 {
		t.Fatalf("Diff(main, feature) = %v, want only b.txt", got)
	}
}

// Unrelated histories keep comparing the base's own tree.
func TestDiffWithoutACommonAncestorComparesTheTrees(t *testing.T) {
	repo, err := OpenRepo(forkRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	got := diffPaths(t, repo, "unrelated", "feature")
	if !got["z.txt"] || !got["a.txt"] || !got["b.txt"] {
		t.Fatalf("Diff(unrelated, feature) = %v, want the tree comparison (z.txt removed, a.txt and b.txt added)", got)
	}
}

// A shallow clone cut before the fork point is an error, never a silent
// tree comparison that would show the base's newer files as removed.
func TestDiffInAShallowCloneCutBeforeTheForkPointFails(t *testing.T) {
	dst, base, head, _ := shallowPullRequest(t, 2)
	repo, err := OpenRepo(dst)
	if err != nil {
		t.Fatal(err)
	}
	diff, _, err := repo.Diff(base, head)
	if err == nil {
		t.Fatalf("a cut shallow clone gave a diff of %d files instead of an error", len(diff.Files))
	}
	if !errors.Is(err, errNoCommonAncestor) || !strings.Contains(err.Error(), "shallow clone") || !strings.Contains(err.Error(), "git fetch") {
		t.Fatalf("error does not say the shallow clone cuts the range and how to fetch more: %v", err)
	}
}

// A history that cannot be read (a commit object missing from a complete
// clone) is an error, never a silent tree comparison.
func TestDiffWithAnUnreadableHistoryFails(t *testing.T) {
	dir := forkRepo(t)
	out, err := exec.Command("git", "-C", dir, "rev-parse", "main~1").Output()
	if err != nil {
		t.Fatal(err)
	}
	seed := strings.TrimSpace(string(out))
	object := filepath.Join(dir, ".git", "objects", seed[:2], seed[2:])
	if err := os.Remove(object); err != nil {
		t.Fatalf("removing the seed commit object: %v", err)
	}
	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if diff, _, err := repo.Diff("main", "feature"); err == nil {
		t.Fatalf("a history missing a commit gave a diff of %d files instead of an error", len(diff.Files))
	}
}

// A criss-cross history (two best common ancestors) starts from one of
// them, as git does: the base's own later gain is never shown as removed.
func TestDiffInACrissCrossHistoryStartsAtAMergeBase(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=F", "GIT_AUTHOR_EMAIL=f@example.invalid", "GIT_COMMITTER_NAME=F", "GIT_COMMITTER_EMAIL=f@example.invalid", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(name string) { write(name); git("add", "."); git("commit", "-q", "-m", name) }
	git("init", "-q", "-b", "main")
	commit("seed.txt")
	git("switch", "-q", "-c", "other")
	commit("y1.txt")
	git("switch", "-q", "main")
	commit("x1.txt")
	git("branch", "-q", "x1")
	git("merge", "-q", "--no-edit", "other") // main = merge(x1, y1)
	git("switch", "-q", "other")
	git("merge", "-q", "--no-edit", "x1") // other = merge(y1, x1)
	commit("feature.txt")
	git("switch", "-q", "main")
	commit("onlybase.txt")
	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := diffPaths(t, repo, "main", "other")
	if got["onlybase.txt"] || !got["feature.txt"] {
		t.Fatalf("Diff(main, other) = %v, want feature.txt and never the base's onlybase.txt", got)
	}
}
