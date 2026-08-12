package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur430IntegrationRoot(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

// IntegrationAUR430 builds the real cmd/aurumcode binary and runs it, as a
// user would, against the tests/fixtures/repos/git-demo bare repository --
// no cloning, no working-tree checkout, no `git` binary: OpenRepo detects a
// bare repository directly (see internal/analyzer/gitrepo.go), so the
// fixture's repo.git directory is used as the reviewed repository's cwd
// as-is. The LLM call is pinned to a deterministic offline fixture via
// AURUMCODE_LLM_FIXTURE (see review.FakeProvider and
// docs/specs/AUR-430.md).
func IntegrationAUR430(t *testing.T) {
	root := aur430IntegrationRoot(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")

	for _, p := range []string{repoDir, fixture} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("required input missing: %s: %v", p, err)
		}
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur430")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	runOnce := func() string {
		cmd := exec.Command(binPath, "review", "--base", "HEAD~1")
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixture)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("aurumcode review --base HEAD~1 failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
		}
		return stdout.String()
	}

	first := runOnce()
	if !strings.Contains(first, "config/demo-tokens.txt") {
		t.Fatalf("expected a finding naming config/demo-tokens.txt, got:\n%s", first)
	}
	if !strings.Contains(first, "[error]") {
		t.Fatalf("expected a finding with an [error] severity marker, got:\n%s", first)
	}

	second := runOnce()
	if first != second {
		t.Fatalf("aurumcode review is not deterministic across runs:\nfirst=%q\nsecond=%q", first, second)
	}

	// No --base at all must fail closed (usage error), not silently
	// review nothing.
	badCmd := exec.Command(binPath, "review")
	badCmd.Dir = repoDir
	if err := badCmd.Run(); err == nil {
		t.Fatal("expected `aurumcode review` without --base to fail")
	}
}
