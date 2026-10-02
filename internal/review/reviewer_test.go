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
      "impact": "The credential can be used by anyone who obtains the repository contents.",
      "evidence": "The added config line contains a credential-shaped assignment.",
      "suggestion": "Remove the secret and rotate it; load it from the environment instead."
      ,"verification": "Remove the value and run the focused review fixture again."
    }
  ],
  "summary": "One planted credential was found in the change."
}`

type optionsCaptureProvider struct {
	FakeProvider
	options llm.Options
}

func (p *optionsCaptureProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	p.options = opts
	return p.FakeProvider.Complete(prompt, opts)
}

func TestReviewRequestsJSONWithoutOutputCap(t *testing.T) {
	provider := &optionsCaptureProvider{FakeProvider: FakeProvider{Response: `{"issues":[],"summary":"No findings."}`}}
	reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil), DefaultConfig())
	if _, err := reviewer.GenerateReview(context.Background(), newFixtureDiff(t)); err != nil {
		t.Fatal(err)
	}
	if !provider.options.JSONMode || provider.options.MaxTokens != 0 {
		t.Fatalf("review options = %+v, want JSON mode without client-side output cap", provider.options)
	}
}

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

// TestAUR517DegradedParseSummaryPinnedToParser pins internal/review's own
// degraded-parse exception (GenerateReviewWithContext's
// prompt.IsDegradedParse check) to what internal/prompt's ResponseParser
// actually produces for a free-form (non-JSON) reply that still matches its
// degraded-recovery line pattern. AUR-519 unified both packages onto a
// single exported source of truth (prompt.DegradedParseSummary /
// prompt.IsDegradedParse), so this is no longer a two-literal drift guard --
// it instead guards that the exported predicate still fires on the parser's
// own real output, which is what this package's exception actually depends
// on.
func TestAUR517DegradedParseSummaryPinnedToParser(t *testing.T) {
	result, err := prompt.NewResponseParser().ParseReviewResponse("config/demo-tokens.txt:3: warning: looks suspicious")
	if err != nil {
		t.Fatalf("ParseReviewResponse: %v", err)
	}
	if !prompt.IsDegradedParse(result) {
		t.Fatalf("prompt.IsDegradedParse did not detect the parser's own degraded-recovery path (Summary=%q)", result.Summary)
	}
	if result.Summary != prompt.DegradedParseSummary {
		t.Fatalf("result.Summary = %q, want prompt.DegradedParseSummary", result.Summary)
	}
}

// TestAUR517DegradedParseNoticeSurvivesFilters covers AUR-517 B1: a
// non-JSON reply recovers one finding with no evidence (degradedExtract
// never sets it), so filterModelIssues always discards it -- but the
// parser's own "the reply was unusable" notice must still reach the
// caller instead of being wiped by the summary-withholding gate that
// discard would otherwise trigger. The diff is a small literal fixture
// (not the repo's git-demo fixture, which tests/acceptance/AUR-517.sh's
// sandboxed copy of go.mod/go.sum/cmd/internal/pkg does not include) so
// this test runs unmodified both in the full checkout and under that
// acceptance script.
func TestAUR517DegradedParseNoticeSurvivesFilters(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		{Path: "app.go", Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{
			"+package demo", "+", "+func ReadAll() []byte {", "+\tdata, _ := fetch()",
			"+\treturn data", "+}", "+", "+func fetch() ([]byte, error) { return nil, nil }",
		}}}},
	}}
	orch := llm.NewOrchestrator(&FakeProvider{Response: "app.go:4: warning: looks suspicious"}, nil, nil)
	reviewer := NewReviewer(orch, DefaultConfig())

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview failed: %v", err)
	}
	if result.Summary != prompt.DegradedParseSummary {
		t.Fatalf("degraded-parse notice was lost: got %q", result.Summary)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("expected the evidence-less recovered issue to be discarded, got %+v", result.Issues)
	}
}
