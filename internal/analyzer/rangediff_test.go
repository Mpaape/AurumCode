package analyzer

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

// gitOut runs git in dir and returns its trimmed output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	runGit(t, dir, args...)
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// lines makes a file of n numbered lines, with line k replaced by repl.
func lines(n int, repl map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if r, ok := repl[i]; ok {
			b.WriteString(r + "\n")
			continue
		}
		fmt.Fprintf(&b, "linha %d\n", i)
	}
	return b.String()
}

// The range read from the object database (no git binary) has the same
// files and hunks as git's own base...head diff, from the merge base even
// when the base branch moved on after the branch point.
func TestRangeDiffFromObjectsMatchesGit(t *testing.T) {
	requireGitBinary(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "a.txt"), lines(40, nil))
	writeFileOrFail(t, filepath.Join(dir, "gone.txt"), "adeus\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	runGit(t, dir, "checkout", "-q", "-b", "feature")
	writeFileOrFail(t, filepath.Join(dir, "a.txt"), lines(40, map[int]string{2: "dois", 20: "vinte", 21: "vinte e um", 38: "trinta e oito"}))
	writeFileOrFail(t, filepath.Join(dir, "novo.txt"), "um\ndois\n")
	runGit(t, dir, "rm", "-q", "gone.txt")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "feature")
	runGit(t, dir, "checkout", "-q", "main")
	writeFileOrFail(t, filepath.Join(dir, "main-only.txt"), "so na main\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "main moves on")
	base, head := gitOut(t, dir, "rev-parse", "main"), gitOut(t, dir, "rev-parse", "feature")

	withGit, err := OpenRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	text, err := withGit.RangeDiff(base, head)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := githubclient.ParseUnifiedDiff(text)
	if err != nil {
		t.Fatal(err)
	}
	objects := &Repo{gitDir: filepath.Join(dir, ".git")}
	got, err := objects.RangeDiffFromObjects(base, head)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != len(parsed.Files) {
		t.Fatalf("files: objects %d, git %d\n%s", len(got.Files), len(parsed.Files), text)
	}
	for i, f := range parsed.Files {
		g := got.Files[i]
		if g.Path != f.Path || len(g.Hunks) != len(f.Hunks) {
			t.Fatalf("file %d: objects %s (%d hunks), git %s (%d hunks)\n%s", i, g.Path, len(g.Hunks), f.Path, len(f.Hunks), text)
		}
		for j, h := range f.Hunks {
			gh := g.Hunks[j]
			want := []int{h.OldStart, h.OldLines, h.NewStart, h.NewLines}
			have := []int{gh.OldStart, gh.OldLines, gh.NewStart, gh.NewLines}
			if !reflect.DeepEqual(want, have) || !reflect.DeepEqual(h.Lines, gh.Lines) {
				t.Fatalf("%s hunk %d: objects %v %q, git %v %q", f.Path, j, have, gh.Lines, want, h.Lines)
			}
		}
	}
	for _, f := range got.Files {
		if f.Path == "main-only.txt" {
			t.Fatal("a change made only on the base branch entered the pull request's range")
		}
	}
}
