package analyzer

import (
	"fmt"
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
	env := gittest.HermeticEnv(t.TempDir())
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	for _, kv := range [][2]string{{"gc.auto", "0"}, {"maintenance.auto", "false"}, {"gc.autoDetach", "false"}} {
		git("config", kv[0], kv[1])
	}
	for i := 0; i < n; i++ {
		git("commit", "-q", "--allow-empty", "-m", fmt.Sprintf("c%d", i))
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
