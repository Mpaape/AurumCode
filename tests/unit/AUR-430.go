package unit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
)

// aur430Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-430.sh); running `go test ./tests/unit -run
// ^TestAUR430$` directly from a full checkout works too, climbing two
// directories from tests/unit back to the repository root.
func aur430Root() string {
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	return filepath.Join("..", "..")
}

// TestAUR430 exercises the restored given -> analyze -> prompt -> llm ->
// parse -> findings pipeline end to end against the deterministic
// tests/fixtures/repos/git-demo fixture and a fake, offline LLM provider,
// then separately proves the parser's degraded-mode fallback recovers
// findings from a response that is not valid JSON at all.
func TestAUR430(t *testing.T) {
	root := aur430Root()

	repoPath := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	repo, err := analyzer.OpenRepo(repoPath)
	if err != nil {
		t.Fatalf("OpenRepo(%s): %v", repoPath, err)
	}

	diff, _, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff(HEAD~1, HEAD): %v", err)
	}
	if len(diff.Files) == 0 {
		t.Fatal("expected the known-problem diff (commit 2 -> commit 3) to contain changed files")
	}

	fixturePath := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
	response, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reading %s: %v", fixturePath, err)
	}

	orch := llm.NewOrchestrator(&review.FakeProvider{Response: string(response)}, nil, nil)
	reviewer := review.NewReviewer(orch, review.DefaultConfig())

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview: %v", err)
	}
	if len(result.Issues) == 0 {
		t.Fatal("AUR-430: expected at least one issue for a diff with a known planted problem, got none")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.File == "config/demo-tokens.txt" && issue.Line > 0 && issue.Severity != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an issue naming config/demo-tokens.txt with file, line and severity populated, got %+v", result.Issues)
	}

	// Degraded-mode fallback: a response with no valid JSON at all must
	// still recover findings via internal/prompt.ResponseParser's
	// documented "file:line: severity: message" convention rather than
	// dying with no output. See internal/prompt/parser.go and
	// docs/specs/AUR-430.md ("Parser fallback").
	parser := prompt.NewResponseParser()
	degraded, err := parser.ParseReviewResponse("- config/demo-tokens.txt:4: error: plaintext credential committed\n")
	if err != nil {
		t.Fatalf("expected the degraded fallback to succeed, got error: %v", err)
	}
	if len(degraded.Issues) != 1 || degraded.Issues[0].File != "config/demo-tokens.txt" {
		t.Fatalf("expected 1 degraded-mode issue for config/demo-tokens.txt, got %+v", degraded.Issues)
	}

	// And the typed-error half of the same contract: a response with
	// nothing recoverable must fail clearly, not silently succeed empty.
	if _, err := parser.ParseReviewResponse("this is not json and matches no finding pattern"); err == nil {
		t.Fatal("expected an error for a response with nothing parseable or recoverable")
	}
}
