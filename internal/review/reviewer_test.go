package review

import (
	"context"
	"os"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const knownProblemResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text.",
      "suggestion": "Remove the secret and rotate it; load it from the environment instead."
    }
  ],
  "summary": "One planted credential was found in the change."
}`

func newFixtureDiff(t *testing.T) *types.Diff {
	t.Helper()
	repo, err := analyzer.OpenRepo("../../tests/fixtures/repos/git-demo/repo.git")
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	diff, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	return diff
}

func TestGenerateReview_KnownProblem(t *testing.T) {
	diff := newFixtureDiff(t)

	orch := llm.NewOrchestrator(&FakeProvider{Response: knownProblemResponse}, nil, nil)
	reviewer := NewReviewer(orch, DefaultConfig())

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview failed: %v", err)
	}

	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %+v", len(result.Issues), result.Issues)
	}
	issue := result.Issues[0]
	if issue.File != "config/demo-tokens.txt" || issue.Line != 3 || issue.Severity != "error" {
		t.Errorf("unexpected issue: %+v", issue)
	}

	if result.Metadata["total_files"] == "" {
		t.Error("expected diff metrics to be attached to Metadata")
	}
}

func TestGenerateReview_Deterministic(t *testing.T) {
	diff := newFixtureDiff(t)

	run := func() *types.ReviewResult {
		orch := llm.NewOrchestrator(&FakeProvider{Response: knownProblemResponse}, nil, nil)
		reviewer := NewReviewer(orch, DefaultConfig())
		result, err := reviewer.GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview failed: %v", err)
		}
		return result
	}

	first, second := run(), run()
	if len(first.Issues) != len(second.Issues) {
		t.Fatalf("non-deterministic issue count: %d vs %d", len(first.Issues), len(second.Issues))
	}
	for i := range first.Issues {
		if first.Issues[i] != second.Issues[i] {
			t.Fatalf("non-deterministic issue at %d: %+v vs %+v", i, first.Issues[i], second.Issues[i])
		}
	}
}

// TestGenerateReview_MUT001Hook proves MUT-001 both ways in the same
// process: with the env hook set, a diff with a known problem must come
// back with zero issues (RED); with it unset (the only state a real build
// ever runs in), the same diff and the same fake response must come back
// with the issue restored byte-for-byte (GREEN). See reviewer.go's
// AURUM_A430_MUTATION hook and tests/acceptance/AUR-430.sh's
// mutation_case for the containerized equivalent of this same proof.
func TestGenerateReview_MUT001Hook(t *testing.T) {
	diff := newFixtureDiff(t)

	t.Setenv("AURUM_A430_MUTATION", "empty-issues")
	orch := llm.NewOrchestrator(&FakeProvider{Response: knownProblemResponse}, nil, nil)
	reviewer := NewReviewer(orch, DefaultConfig())
	red, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview (mutated) failed: %v", err)
	}
	if len(red.Issues) != 0 {
		t.Fatalf("MUT-001 RED: expected zero issues with the mutation active, got %d", len(red.Issues))
	}

	if err := os.Unsetenv("AURUM_A430_MUTATION"); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	orch2 := llm.NewOrchestrator(&FakeProvider{Response: knownProblemResponse}, nil, nil)
	reviewer2 := NewReviewer(orch2, DefaultConfig())
	green, err := reviewer2.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview (restored) failed: %v", err)
	}
	if len(green.Issues) != 1 || green.Issues[0].File != "config/demo-tokens.txt" {
		t.Fatalf("MUT-001 GREEN: expected the known-problem issue restored, got %+v", green.Issues)
	}
}
