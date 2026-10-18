package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gittest"
)

// fetchPRRangeScript is scripts/ci/fetch-pr-range.sh; a missing script
// fails the test, only a module-only acceptance skips it.
func fetchPRRangeScript(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..")
	skip, err := manifestCheckSkipped(root, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Skip("AURUMCODE_MODULE_ONLY=1: the acceptance staged only the Go module")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	script, err := filepath.Abs(filepath.Join(root, "scripts", "ci", "fetch-pr-range.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("fetch-pr-range.sh: %v", err)
	}
	return script
}

func prGit(t *testing.T, dir string, stdin []byte, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gittest.HermeticEnv(t.TempDir())
	cmd.Stdin = bytes.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// longOrigin writes, with one git fast-import, a main of commits commits
// and a pull request branch of 3 commits forked behind main's tip by
// behind commits (an orphan branch when behind is 0).
func longOrigin(t *testing.T, commits, behind int) string {
	t.Helper()
	src := t.TempDir()
	prGit(t, src, nil, "init", "-q", "-b", "main")
	prGit(t, src, nil, "config", "uploadpack.allowReachableSHA1InWant", "true")
	var s strings.Builder
	commit := func(ref string, mark, when int, from string, text string) {
		fmt.Fprintf(&s, "commit %s\nmark :%d\ncommitter f <f@example.invalid> %d +0000\ndata %d\n%s\n", ref, mark, when, len(text), text)
		if from != "" {
			fmt.Fprintf(&s, "from %s\n", from)
		}
		fmt.Fprintf(&s, "M 644 inline %s.txt\ndata %d\n%s\n\n", strings.Fields(text)[0], len(text), text)
	}
	for i := 1; i <= commits; i++ {
		from := ""
		if i > 1 {
			from = ":" + strconv.Itoa(i-1)
		}
		commit("refs/heads/main", i, 1000000+i*60, from, fmt.Sprintf("main %d", i))
	}
	fork := ""
	if behind > 0 {
		fork = ":" + strconv.Itoa(commits-behind)
	}
	for i := 1; i <= 3; i++ {
		from := fork
		if i > 1 {
			from = ":" + strconv.Itoa(commits+i-1)
		}
		commit("refs/heads/pr", commits+i, 1000000+(commits+i)*60, from, fmt.Sprintf("pr %d", i))
		fork = ""
	}
	prGit(t, src, []byte(s.String()), "fast-import", "--quiet")
	return src
}

// runFetchPRRange clones the pull request with depth 1, as the workflow
// checkout does, and runs the script on it.
func runFetchPRRange(t *testing.T, src string) (string, string, string, string, error) {
	t.Helper()
	script := fetchPRRangeScript(t)
	dst := t.TempDir()
	prGit(t, dst, nil, "clone", "-q", "--no-local", "--depth=1", "--branch", "pr", "file://"+src, ".")
	base := prGit(t, src, nil, "rev-parse", "main")
	head := prGit(t, src, nil, "rev-parse", "pr")
	cmd := exec.Command("sh", script, dst, base, head)
	cmd.Env = append(gittest.HermeticEnv(t.TempDir()), "GH_TOKEN=fetch-pr-range-canary")
	out, err := cmd.CombinedOutput()
	return dst, base, head, string(out), err
}

// From a depth-1 checkout the script fetches only what the pull request
// needs: the diff is the full clone's, and the clone stays shallow.
func TestFetchPRRangeFetchesOnlyThePullRequestHistory(t *testing.T) {
	src := longOrigin(t, 3000, 120)
	dst, base, head, out, err := runFetchPRRange(t, src)
	if err != nil {
		t.Fatalf("fetch-pr-range.sh: %v\n%s", err, out)
	}
	t.Log(out)
	if got := prGit(t, dst, nil, "rev-parse", "--is-shallow-repository"); got != "true" {
		t.Fatalf("the clone is no longer shallow (%s):\n%s", got, out)
	}
	count, err := strconv.Atoi(prGit(t, dst, nil, "rev-list", "--count", "--all"))
	if err != nil || count > 400 {
		t.Fatalf("the clone holds %d of 3003 commits", count)
	}
	full := t.TempDir()
	prGit(t, full, nil, "clone", "-q", "--no-local", "file://"+src, ".")
	want := prGit(t, full, nil, "diff", base+"..."+head)
	if got := prGit(t, dst, nil, "diff", base+"..."+head); got != want || want == "" {
		t.Fatalf("shallow diff differs from the full clone's:\n%s\nwant:\n%s", got, want)
	}
	config, err := os.ReadFile(filepath.Join(dst, ".git", "config"))
	if err != nil || strings.Contains(string(config)+out, "fetch-pr-range-canary") {
		t.Fatalf("the token reached the config or the log: %v\n%s", err, out)
	}
}

// Without a common ancestor, even with the whole history, the script fails
// saying why.
func TestFetchPRRangeWithoutCommonAncestorFails(t *testing.T) {
	src := longOrigin(t, 50, 0)
	dst, _, _, out, err := runFetchPRRange(t, src)
	if err == nil || !strings.Contains(out, "::error title=") || !strings.Contains(out, "não têm ancestral comum") {
		t.Fatalf("unrelated histories: %v\n%s", err, out)
	}
	if got := prGit(t, dst, nil, "rev-parse", "--is-shallow-repository"); got != "false" {
		t.Fatalf("failed before reaching the whole history: %s", got)
	}
}
