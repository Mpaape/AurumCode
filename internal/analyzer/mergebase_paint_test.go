package analyzer

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// memGraph is an in-memory commit graph that counts every commit read.
type memGraph struct {
	nodes map[string]commitNode
	reads int
}

func (g *memGraph) add(sha string, when int64, parents ...string) {
	g.nodes[sha] = commitNode{parents: parents, when: when}
}

func (g *memGraph) read(sha string) (commitNode, error) {
	g.reads++
	n, ok := g.nodes[sha]
	if !ok {
		return commitNode{}, fmt.Errorf("object %s not found", sha)
	}
	return n, nil
}

// longHistory is a 3000-commit trunk with a pull request of 3 commits
// forked at trunk commit 2990, while the base moved 10 commits on.
func longHistory() (*memGraph, string, string) {
	g := &memGraph{nodes: map[string]commitNode{}}
	g.add("t0", 1)
	for i := 1; i < 3000; i++ {
		g.add(fmt.Sprintf("t%d", i), int64(i+1), fmt.Sprintf("t%d", i-1))
	}
	g.add("p1", 2992, "t2990")
	g.add("p2", 2995, "p1")
	g.add("p3", 3001, "p2")
	return g, "t2999", "p3"
}

func TestMergeBaseReadsThePullRequestNotTheHistory(t *testing.T) {
	g, base, head := longHistory()
	got, err := newPaintWalk(g.read).mergeBase(base, head)
	if err != nil || got != "t2990" {
		t.Fatalf("merge base = %q, %v; want t2990", got, err)
	}
	// 3 PR commits, 10 base commits, the fork point and its parent.
	if g.reads > 20 {
		t.Fatalf("read %d commits of a 3000-commit history for a 3-commit pull request", g.reads)
	}
}

func TestMergeBaseOfCrissCrossHistoryIsAmbiguous(t *testing.T) {
	g := &memGraph{nodes: map[string]commitNode{}}
	g.add("r", 1)
	g.add("x", 2, "r")
	g.add("y", 3, "r")
	g.add("a", 4, "x", "y")
	g.add("b", 5, "y", "x")
	if _, err := newPaintWalk(g.read).mergeBase("a", "b"); !errors.Is(err, errAmbiguousMergeBase) {
		t.Fatalf("criss-cross merge base: %v, want errAmbiguousMergeBase", err)
	}
}

func TestMergeBaseWithoutCommonAncestorIsAnError(t *testing.T) {
	g := &memGraph{nodes: map[string]commitNode{}}
	g.add("a", 1)
	g.add("b", 2)
	if got, err := newPaintWalk(g.read).mergeBase("a", "b"); err == nil {
		t.Fatalf("unrelated histories gave merge base %q", got)
	}
}

func TestMergeBaseOfACommitWithItsAncestor(t *testing.T) {
	g, _, head := longHistory()
	if got, err := newPaintWalk(g.read).mergeBase("t2990", head); err != nil || got != "t2990" {
		t.Fatalf("merge base = %q, %v; want t2990", got, err)
	}
}

// A clock-skewed ancestor must not survive as a second best candidate.
func TestRemoveRedundantDropsAnAncestorCandidate(t *testing.T) {
	g := &memGraph{nodes: map[string]commitNode{}}
	g.add("r", 50)
	g.add("c", 2, "r")
	got, err := newPaintWalk(g.read).removeRedundant([]string{"r", "c"})
	if err != nil || len(got) != 1 || got[0] != "c" {
		t.Fatalf("kept %v, %v; want [c]", got, err)
	}
}

// A commit the walk needs but cannot read is an error, never a guess.
func TestMergeBaseMissingCommitIsAnError(t *testing.T) {
	g, base, head := longHistory()
	delete(g.nodes, "t2990")
	if _, err := newPaintWalk(g.read).mergeBase(base, head); err == nil || !strings.Contains(err.Error(), "t2990") {
		t.Fatalf("missing fork point: %v", err)
	}
}

// shallowPullRequest builds a 40-commit main, a 3-commit pull request
// forked at main~5, and a clone holding only the pull request and the base
// down to the fork point: shallow, with the common ancestor present.
func shallowPullRequest(t *testing.T, baseDepth int) (string, string, string, string) {
	t.Helper()
	requireGitBinary(t)
	src := t.TempDir()
	runGit(t, src, "init", "-q", "-b", "main")
	for i := 0; i < 40; i++ {
		if i == 35 {
			runGit(t, src, "branch", "pr")
		}
		writeFileOrFail(t, filepath.Join(src, "a.txt"), fmt.Sprintf("main %d\n", i))
		runGit(t, src, "add", "a.txt")
		runGit(t, src, "commit", "-q", "-m", fmt.Sprintf("main %d", i))
	}
	runGit(t, src, "checkout", "-q", "pr")
	for i := 0; i < 3; i++ {
		writeFileOrFail(t, filepath.Join(src, "b.txt"), fmt.Sprintf("pr %d\n", i))
		runGit(t, src, "add", "b.txt")
		runGit(t, src, "commit", "-q", "-m", fmt.Sprintf("pr %d", i))
	}
	dst := t.TempDir()
	runGit(t, dst, "clone", "-q", "--no-local", "--depth=4", "--branch", "pr", "file://"+src, ".")
	runGit(t, dst, "fetch", "-q", fmt.Sprintf("--depth=%d", baseDepth), "origin", "main:refs/remotes/origin/main")
	return dst, revParse(t, dst, "origin/main"), revParse(t, dst, "HEAD"), revParse(t, src, "main~5")
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "-c", "safe.directory="+dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

func TestMergeBaseInAShallowCloneWithTheForkPoint(t *testing.T) {
	dst, base, head, fork := shallowPullRequest(t, 6)
	if revParse(t, dst, "--is-shallow-repository") != "true" {
		t.Fatal("the clone is not shallow")
	}
	repo, err := OpenRepo(dst)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.mergeBaseFromObjects(base, head)
	if err != nil || got != fork {
		t.Fatalf("shallow merge base = %q, %v; want %s", got, err, fork)
	}
	diff, err := repo.RangeDiffFromObjects(base, head)
	if err != nil || len(diff.Files) != 1 || diff.Files[0].Path != "b.txt" {
		t.Fatalf("shallow range diff = %+v, %v", diff, err)
	}
}

func TestMergeBaseInAShallowCloneCutBeforeTheForkPoint(t *testing.T) {
	dst, base, head, _ := shallowPullRequest(t, 2)
	repo, err := OpenRepo(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := repo.mergeBaseFromObjects(base, head); err == nil {
		t.Fatalf("a cut clone gave merge base %q", got)
	}
}
