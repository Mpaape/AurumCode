package review

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
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
	diff, _, err := repo.Diff("HEAD~1", "HEAD")
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

func TestDefaultConfigKeepsMixedCodeDiffInPrompt(t *testing.T) {
	cfg := DefaultConfig()
	builder := prompt.NewPromptBuilder()
	diff := &types.Diff{Files: []types.DiffFile{
		{Path: ".github/workflows/review.yml", Hunks: []types.DiffHunk{{Lines: []string{"+permissions:"}}}},
		{Path: "lib/pricing.mjs", Hunks: []types.DiffHunk{{Lines: []string{"+'gpt-5.6-terra': { input: 2.0, output: 12.0 }"}}}},
		{Path: "test/unit.mjs", Hunks: []types.DiffHunk{{Lines: []string{"+ok('pricing', true)"}}}},
	}}
	metrics := &analyzer.DiffMetrics{TotalFiles: 3, LanguageBreakdown: map[string]int{"javascript": 2, "yaml": 1}}

	parts, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: cfg.MaxTokens, SchemaKind: "review", Role: "reviewer", ReserveReply: cfg.ReserveReply,
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	for _, marker := range []string{"gpt-5.6-terra", "ok('pricing', true)"} {
		if !strings.Contains(parts.User, marker) {
			t.Fatalf("mixed code diff lost %q from prompt:\n%s", marker, parts.User)
		}
	}
}
