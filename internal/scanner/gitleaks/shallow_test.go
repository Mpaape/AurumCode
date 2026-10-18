package gitleaks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gittest"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

// gitIn runs git in dir with a hermetic environment and returns stdout.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gittest.HermeticEnv(t.TempDir())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// shallowClone builds a 20-commit main with a 3-commit pull request forked
// at main~5, and clones the pull request with depth commits of it.
func shallowClone(t *testing.T, depth int) (string, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	src := t.TempDir()
	gitIn(t, src, "init", "-q", "-b", "main")
	commit := func(file, text string) {
		if err := os.WriteFile(filepath.Join(src, file), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		gitIn(t, src, "add", file)
		gitIn(t, src, "commit", "-q", "-m", text)
	}
	for i := 0; i < 20; i++ {
		if i == 15 {
			gitIn(t, src, "branch", "pr")
		}
		commit("a.txt", fmt.Sprintf("main %d", i))
	}
	gitIn(t, src, "checkout", "-q", "pr")
	for i := 0; i < 3; i++ {
		commit("b.txt", fmt.Sprintf("pr %d", i))
	}
	dst := t.TempDir()
	gitIn(t, dst, "clone", "-q", "--no-local", fmt.Sprintf("--depth=%d", depth), "--branch", "pr", "file://"+src, ".")
	gitIn(t, dst, "fetch", "-q", "--depth=1", "origin", "main:refs/remotes/origin/main")
	return dst, gitIn(t, src, "rev-parse", "main~5"), gitIn(t, src, "rev-parse", "pr")
}

// A real shallow clone: holding the range from the fork point it is
// scannable; cut inside the range it is not, and saying why; a full
// history scan always needs the complete repository.
func TestVerifyRangeOfARealShallowClone(t *testing.T) {
	ctx := context.Background()
	whole, fork, head := shallowClone(t, 4)
	if err := verifyRange(ctx, scanner.ExecCommand, whole, scanner.Range{Base: fork, Head: head}); err != nil {
		t.Fatalf("shallow clone holding the range: %v", err)
	}
	if err := verifyRange(ctx, scanner.ExecCommand, whole, scanner.Range{Head: head, Full: true}); !errors.Is(err, ErrBadRange) {
		t.Fatalf("full scan of a shallow clone: %v, want ErrBadRange", err)
	}
	cut, fork, head := shallowClone(t, 2)
	gitIn(t, cut, "fetch", "-q", "--depth=1", "origin", fork)
	err := verifyRange(ctx, scanner.ExecCommand, cut, scanner.Range{Base: fork, Head: head})
	if !errors.Is(err, ErrBadRange) || !strings.Contains(err.Error(), "cuts the pull request range") {
		t.Fatalf("shallow clone cutting the range: %v", err)
	}
}
