package analyzer

import (
	"strings"
	"testing"
)

// fixtureRepo is tests/fixtures/repos/git-demo/repo.git, a deterministic
// bare repository with three commits (see history.spec). Package tests run
// with the package directory as the working directory, so the path climbs
// two levels back to the repository root.
const fixtureRepo = "../../tests/fixtures/repos/git-demo/repo.git"

// Commit ids from tests/fixtures/repos/git-demo/manifest.json, spelled out
// here so a change to the fixture's history is a loud, obvious test
// failure rather than a silently different SHA being trusted.
const (
	fixtureCommit1 = "443b29afea9469f73e609f95a3e7438763bb12f0" // seed
	fixtureCommit2 = "c7a2e5a32bb7b0af16b97e5b70fec5a38ff9d5d7" // add greeter
	fixtureCommit3 = "eedb8f9dbd4e725cb8b9cc065fe7f1090f2a9024" // plant tokens, drop notes (HEAD)
)

func TestOpenRepo_Bare(t *testing.T) {
	if _, err := OpenRepo(fixtureRepo); err != nil {
		t.Fatalf("OpenRepo(%s) failed: %v", fixtureRepo, err)
	}
}

func TestOpenRepo_NotAGitDir(t *testing.T) {
	if _, err := OpenRepo("."); err == nil {
		t.Fatal("expected an error opening a plain directory as a git repo")
	}
}

func TestResolveRef(t *testing.T) {
	repo, err := OpenRepo(fixtureRepo)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	tests := []struct {
		ref  string
		want string
	}{
		{"HEAD", fixtureCommit3},
		{"HEAD~1", fixtureCommit2},
		{"HEAD~2", fixtureCommit1},
		{"main", fixtureCommit3},
		{fixtureCommit1, fixtureCommit1},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := repo.ResolveRef(tt.ref)
			if err != nil {
				t.Fatalf("ResolveRef(%q) failed: %v", tt.ref, err)
			}
			if got != tt.want {
				t.Errorf("ResolveRef(%q) = %s, want %s", tt.ref, got, tt.want)
			}
		})
	}
}

func TestResolveRef_TooManyParents(t *testing.T) {
	repo, err := OpenRepo(fixtureRepo)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if _, err := repo.ResolveRef("HEAD~3"); err == nil {
		t.Fatal("expected an error walking past the root commit")
	}
}

func TestDiff_HeadAgainstParent(t *testing.T) {
	repo, err := OpenRepo(fixtureRepo)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	diff, _, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	byPath := map[string]bool{}
	for _, f := range diff.Files {
		byPath[f.Path] = true
	}

	if !byPath["config/demo-tokens.txt"] {
		t.Errorf("expected config/demo-tokens.txt to appear as added, files=%v", byPath)
	}
	if !byPath["NOTES.txt"] {
		t.Errorf("expected NOTES.txt to appear as deleted, files=%v", byPath)
	}
	// config/app.yaml and README.md did not change between commit 2 and
	// commit 3, so they must not appear at all.
	if byPath["config/app.yaml"] {
		t.Errorf("config/app.yaml is unchanged between HEAD~1 and HEAD and must be omitted")
	}
	if byPath["README.md"] {
		t.Errorf("README.md is unchanged between HEAD~1 and HEAD and must be omitted")
	}

	var tokensFile *string
	for _, f := range diff.Files {
		if f.Path == "config/demo-tokens.txt" {
			joined := strings.Join(f.Hunks[0].Lines, "\n")
			tokensFile = &joined
		}
	}
	if tokensFile == nil {
		t.Fatal("config/demo-tokens.txt hunk not found")
	}
	if !strings.Contains(*tokensFile, "+DEMO_API_TOKEN=AURUM-FAKE-TOKEN-0000-0001") {
		t.Errorf("expected the planted token line as an addition, got:\n%s", *tokensFile)
	}
}

func TestDiff_Deterministic(t *testing.T) {
	repo, err := OpenRepo(fixtureRepo)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	first, _, err := repo.Diff("HEAD~2", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	second, _, err := repo.Diff("HEAD~2", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(first.Files) != len(second.Files) {
		t.Fatalf("non-deterministic file count: %d vs %d", len(first.Files), len(second.Files))
	}
	for i := range first.Files {
		if first.Files[i].Path != second.Files[i].Path {
			t.Fatalf("non-deterministic file order at %d: %s vs %s", i, first.Files[i].Path, second.Files[i].Path)
		}
	}
}

func TestDiff_NoChanges(t *testing.T) {
	repo, err := OpenRepo(fixtureRepo)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	diff, _, err := repo.Diff("HEAD", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diff.Files) != 0 {
		t.Fatalf("expected no changed files comparing HEAD to itself, got %d", len(diff.Files))
	}
}
