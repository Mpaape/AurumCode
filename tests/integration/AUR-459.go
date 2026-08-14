// Package integration holds AUR-459's Integration-layer proof, at the seam
// the unit program cannot see: internal/prompt's parser feeding
// internal/review's rule gate through the real GenerateReview path, with a
// deterministic offline provider (review.FakeProvider) standing in for the
// model.
//
// The unit program (tests/unit/AUR-459.go) asserts on the parser alone: a
// line_comments-only response becomes an issue with its fields mapped.
// This program asserts what only the composition can show -- that a
// converted finding survives the AUR-434 rule gate and comes back carrying
// the catalog citation, that one without a rule_id is discarded WITH the
// AUR-448 warning populated instead of silence, and that the parser's own
// discard warning is not clobbered by the gate's, which writes to the same
// metadata map right after. None of these three facts is observable from
// the parser in isolation.
package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur459Diff is the minimal reviewable input: the response is canned, so
// the diff only has to be non-degenerate.
func aur459Diff() *types.Diff {
	return &types.Diff{
		Files: []types.DiffFile{
			{
				Path:     "config/demo-tokens.txt",
				Lang:     "text",
				Hunks:    []types.DiffHunk{{Lines: []string{"+DEMO_API_TOKEN=redacted-in-this-fixture"}}},
			},
		},
	}
}

func aur459Review(t *testing.T, response string) *types.ReviewResult {
	t.Helper()
	orchestrator := llm.NewOrchestrator(&review.FakeProvider{Response: response}, nil, nil)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())
	result, err := reviewer.GenerateReview(context.Background(), aur459Diff())
	if err != nil {
		t.Fatalf("GenerateReview failed: %v", err)
	}
	return result
}

// IntegrationAUR459 is this card's integration selector.
func IntegrationAUR459(t *testing.T) {
	t.Run("ConvertedFindingSurvivesTheRuleGate", aur459SurvivesGate)
	t.Run("UncitedConvertedFindingIsAnnouncedNotSilent", aur459UncitedAnnounced)
	t.Run("ParserDiscardWarningSurvivesTheGate", aur459ParserWarningSurvives)
}

// aur459SurvivesGate: a finding the model reported only under
// "line_comments", citing a rule of the embedded catalog, comes back from
// the full pipeline enriched with that rule's citation -- proof the
// conversion happens early enough (inside ParseReviewResponse) for
// redaction and the rule gate to treat it exactly like a finding reported
// under "issues".
func aur459SurvivesGate(t *testing.T) {
	result := aur459Review(t, `{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","rule_id":"security/hardcoded-secret","body":"credential-shaped value committed"}],"summary":"one finding"}`)

	if len(result.Issues) != 1 {
		t.Fatalf("expected the converted finding to survive the rule gate, got %d issues: %+v", len(result.Issues), result.Issues)
	}
	issue := result.Issues[0]
	if issue.File != "config/demo-tokens.txt" || issue.Line != 4 {
		t.Errorf("location lost through the pipeline: %+v", issue)
	}
	if !strings.Contains(issue.Message, "credential-shaped value committed") {
		t.Errorf("the model's text was lost: %q", issue.Message)
	}
	if !strings.Contains(issue.Message, "(rule security/hardcoded-secret:") {
		t.Errorf("a surviving finding must carry the catalog citation, got %q", issue.Message)
	}
	if warning := result.Metadata["discard_warning"]; warning != "" {
		t.Errorf("nothing was discarded, so the rule-gate warning must stay empty, got %q", warning)
	}
}

// aur459UncitedAnnounced: the same finding without a rule_id is discarded
// by the gate -- and the discard is named. This is the half of AUR-459's
// chosen answer to the trap that the parser alone cannot demonstrate.
func aur459UncitedAnnounced(t *testing.T) {
	result := aur459Review(t, `{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"body":"credential-shaped value committed"}],"summary":"one finding"}`)

	if len(result.Issues) != 0 {
		t.Fatalf("a finding citing no rule must not reach the user, got %+v", result.Issues)
	}
	warning := result.Metadata["discard_warning"]
	if warning == "" {
		t.Fatal("the model reported a finding and the user is told nothing: this is the silence AUR-459 exists to remove")
	}
	if !strings.Contains(warning, "1 finding(s) discarded") || !strings.Contains(warning, "no rule_id") {
		t.Errorf("the warning must say how many and why, got %q", warning)
	}
	if result.Metadata["issues_rejected_without_rule"] != "1" {
		t.Errorf("the gate must count the rejection, got %q", result.Metadata["issues_rejected_without_rule"])
	}
}

// aur459ParserWarningSurvives: GenerateReview writes its own keys into the
// same Metadata map the parser filled, immediately after parsing. The
// parser's discard warning must still be there for cmd/aurumcode to print
// -- a key silently overwritten here would restore, one layer up, exactly
// the silence this card removed.
func aur459ParserWarningSurvives(t *testing.T) {
	result := aur459Review(t, `{
	  "issues":[{"file":"config/demo-tokens.txt","line":4,"severity":"error","rule_id":"security/hardcoded-secret","message":"credential committed"}],
	  "line_comments":[
	    {"path":"config/demo-tokens.txt","line":4,"body":"Credential   Committed"},
	    {"path":"","line":9,"body":"nowhere to point"}
	  ]
	}`)

	if len(result.Issues) != 1 {
		t.Fatalf("expected the single reported finding, got %+v", result.Issues)
	}
	warning := result.Metadata["parse_discard_warning"]
	if warning == "" {
		t.Fatal("the parser's own discards were lost between the parse and the caller")
	}
	if !strings.Contains(warning, "already reported as an issue") || !strings.Contains(warning, "no path") {
		t.Errorf("the parser warning must name both reasons, got %q", warning)
	}
	if result.Metadata["line_comments_discarded"] != "2" {
		t.Errorf("both non-adopted comments must be counted, got %q", result.Metadata["line_comments_discarded"])
	}
}
