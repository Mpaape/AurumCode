package main

import (
	"github.com/Mpaape/AurumCode/internal/memory"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAUR489PRMemoryDirNotEmpty pins the caller-side half of AC-001: the PR
// path always has owner/repo (--repo is required by runPRReview), so
// memoryDirFor -- the function pr.go's memory.New call now feeds -- must
// never return "" for that case, and two different repositories must
// resolve to two different directories. Before this card, pr.go:229 passed
// memory.New(mode, "") unconditionally; TestAUR489MemoryScope
// (internal/memory) is the store-side half of the same proof.
func TestAUR489PRMemoryDirNotEmpty(t *testing.T) {
	// The PR path opens memory only through newRepoMemory. A note saved while
	// reviewing team/project-a must be invisible to team/project-b: before
	// AUR-489 both resolved to one process-wide file.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	storeA, err := newRepoMemory("local", "team", "project-a")
	if err != nil {
		t.Fatalf("newRepoMemory(A): %v", err)
	}
	if err := storeA.Save([]memory.Note{{Body: "only-project-a-knows-this"}}); err != nil {
		t.Fatalf("Save(A): %v", err)
	}
	storeB, err := newRepoMemory("local", "team", "project-b")
	if err != nil {
		t.Fatalf("newRepoMemory(B): %v", err)
	}
	notesB, _ := storeB.Load()
	for _, n := range notesB {
		if strings.Contains(n.Body, "only-project-a-knows-this") {
			t.Fatalf("AUR-489/AC-001: project-b loaded a note project-a saved: %+v", notesB)
		}
	}
	againA, err := newRepoMemory("local", "team", "project-a")
	if err != nil {
		t.Fatalf("newRepoMemory(A again): %v", err)
	}
	notesA, _ := againA.Load()
	if len(notesA) != 1 {
		t.Fatalf("AUR-489/AC-001: project-a should read back its own note, got %d", len(notesA))
	}
}

// TestAUR489MemoryDirSanitizesTraversal proves a hostile owner/repo value
// (a ".." segment, or a literal "/") can never escape the cache root: every
// resolved directory must stay inside os.UserCacheDir()/aurumcode/memory.
func TestAUR489MemoryDirSanitizesTraversal(t *testing.T) {
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache dir on this platform: %v", err)
	}
	root := filepath.Join(base, "aurumcode", "memory")

	for _, tc := range []struct{ owner, repo string }{
		{"..", "escape"},
		{"team", "../../etc"},
		{"a/b", "c"},
	} {
		dir, err := memoryDirFor(tc.owner, tc.repo)
		if err != nil {
			t.Fatalf("memoryDirFor(%q, %q): %v", tc.owner, tc.repo, err)
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			t.Fatalf("filepath.Rel: %v", err)
		}
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			t.Fatalf("memoryDirFor(%q, %q) escaped the cache root: %q (rel=%q)", tc.owner, tc.repo, dir, rel)
		}
	}
}

// TestAUR489MemoryDirFallsBackWithoutOwnerRepo proves the two non-PR
// fallbacks (remote hash, then absolute-path hash) still produce a
// non-empty, deterministic directory, exercised from a plain temp
// directory that is never a git checkout.
func TestAUR489MemoryDirFallsBackWithoutOwnerRepo(t *testing.T) {
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()

	got, err := memoryDirFor("", "")
	if err != nil {
		t.Fatalf("memoryDirFor(\"\", \"\"): %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("memoryDirFor(\"\", \"\") returned an empty dir")
	}
	again, err := memoryDirFor("", "")
	if err != nil {
		t.Fatalf("memoryDirFor (again): %v", err)
	}
	if again != got {
		t.Fatalf("path-fallback memoryDirFor is not deterministic: %q vs %q", got, again)
	}
}

// chdir switches the process working directory to dir for the duration of
// the test and returns a func that restores it. No t.Parallel may run in
// this package's tests while a chdir is in effect (none do today).
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q): %v", dir, err)
	}
	return func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore os.Chdir(%q): %v", prev, err)
		}
	}
}
