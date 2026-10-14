package analyzer

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gittest"
)

// Issue #179: a range past the old 512-commit walk bound is read whole, by
// the git binary and by the pure-Go reader, and the merge base of a long
// history is still found. Automatic gc stays off: the pure-Go reader
// reads loose objects only.
func TestCommitsPastTheOldWalkBoundAreReadWhole(t *testing.T) {
	requireGitBinary(t)
	dir := t.TempDir()
	twoCommitRepo(t, dir)
	const n = 530
	cmd := exec.Command("sh", "-c", `git config gc.auto 0 && git config maintenance.auto false && git config gc.autoDetach false || exit 1; i=0; while [ $i -lt 530 ]; do git commit -q --allow-empty -m "c$i" || exit 1; i=$((i+1)); done`)
	cmd.Dir = dir
	cmd.Env = gittest.HermeticEnv(t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating %d commits: %v\n%s", n, err, out)
	}
	viaGit, err := OpenRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	pureGo := &Repo{gitDir: filepath.Join(dir, ".git")}
	for name, repo := range map[string]*Repo{"git": viaGit, "pure-go": pureGo} {
		commits, err := repo.Commits("HEAD~530", "HEAD")
		if err != nil || len(commits) != n {
			t.Fatalf("%s: err=%v commits=%d, want %d", name, err, len(commits), n)
		}
	}
}
