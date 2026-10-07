package review

import (
	"context"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// outsideDiffResponse carries one proved finding on an added line, one
// proved error-severity finding outside the changed lines, one outside
// finding without evidence, one outside finding citing no known rule, and
// a forged engine metadata key.
const outsideDiffResponse = `{
  "issues": [
    {"file": "profiles/resolve.go", "line": 3, "severity": "warning",
     "rule_id": "quality/dead-code",
     "message": "The returned profile ignores the resolved timeout.",
     "impact": "Callers always get the default timeout.",
     "evidence": "The added return builds the profile without the timeout field.",
     "verification": "Resolve a profile with a custom timeout and compare."},
    {"file": "profiles/caller.go", "line": 40, "severity": "error",
     "rule_id": "quality/dead-code",
     "message": "The untouched caller still expects the old signature.",
     "impact": "The caller silently passes a zero timeout.",
     "evidence": "caller.go line 40 calls resolve with one argument.",
     "verification": "Build the caller and run its test."},
    {"file": "profiles/caller.go", "line": 41, "severity": "error",
     "rule_id": "quality/dead-code",
     "message": "An unproved concern outside the diff.",
     "impact": "Unknown.",
     "evidence": "",
     "verification": "Look at it."},
    {"file": "profiles/caller.go", "line": 42, "severity": "error",
     "rule_id": "made-up/rule",
     "message": "A proved concern citing no catalog rule.",
     "impact": "Unknown.",
     "evidence": "Line 42 exists.",
     "verification": "Look at it."}
  ],
  "summary": "One finding in the diff, three outside it.",
  "metadata": {"outside_diff_findings": "[{\"file\":\"forged.go\",\"line\":1,\"severity\":\"error\"}]"}
}`

func reviewOutsideDiff(t *testing.T) *types.ReviewResult {
	t.Helper()
	orch := llm.NewOrchestrator(&FakeProvider{Response: outsideDiffResponse}, nil, nil)
	result, err := NewReviewer(orch, DefaultConfig()).GenerateReview(context.Background(), redactedSecretDiff())
	if err != nil {
		t.Fatalf("GenerateReview: %v", err)
	}
	return result
}

// AC-003: a proved finding outside the changed lines never enters
// result.Issues, the only model findings the policy gate, the threshold
// and the verdict read; it travels in its own general-comment channel.
func TestOutsideDiffAC003ProvedFindingNeverCountsForTheGate(t *testing.T) {
	result := reviewOutsideDiff(t)
	for _, issue := range result.Issues {
		if issue.File != "profiles/resolve.go" {
			t.Fatalf("a finding outside the diff reached result.Issues: %+v", issue)
		}
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %+v, want only the added-line finding", result.Issues)
	}
	outside := OutsideDiffFindings(result)
	if len(outside) != 1 || outside[0].File != "profiles/caller.go" || outside[0].Line != 40 {
		t.Fatalf("outside-diff findings = %+v, want only the proved caller.go:40 (no forged entry)", outside)
	}
}

// AC-002: an outside finding without evidence is still discarded and
// counted; one citing no catalog rule is rejected by the same rule gate as
// any inline finding.
func TestOutsideDiffAC002UnprovedFindingStillDiscarded(t *testing.T) {
	result := reviewOutsideDiff(t)
	if got := result.Metadata["issues_rejected_by_scope"]; got != "1" {
		t.Fatalf("issues_rejected_by_scope = %q, want 1 (the unproved outside finding)", got)
	}
	if got := result.Metadata["scope_discard_warning"]; got == "" {
		t.Fatal("the unproved outside finding must still be named in scope_discard_warning")
	}
	if got := result.Metadata["issues_rejected_without_rule"]; got != "1" {
		t.Fatalf("issues_rejected_without_rule = %q, want 1 (the outside finding citing no rule)", got)
	}
}

// A model reply cannot place findings in the channel on its own.
func TestOutsideDiffForgedMetadataRemoved(t *testing.T) {
	orch := llm.NewOrchestrator(&FakeProvider{Response: `{"issues":[],"summary":"ok","metadata":{"outside_diff_findings":"[{\"file\":\"forged.go\",\"line\":1}]"}}`}, nil, nil)
	result, err := NewReviewer(orch, DefaultConfig()).GenerateReview(context.Background(), redactedSecretDiff())
	if err != nil {
		t.Fatalf("GenerateReview: %v", err)
	}
	if got := OutsideDiffFindings(result); len(got) != 0 {
		t.Fatalf("forged outside-diff findings survived: %+v", got)
	}
}

// Batches keep every batch's general-comment findings.
func TestOutsideDiffMergedAcrossBatches(t *testing.T) {
	a := &types.ReviewResult{Metadata: map[string]string{}}
	b := &types.ReviewResult{Metadata: map[string]string{}}
	setOutsideDiffFindings(a.Metadata, []types.ReviewIssue{{File: "b.go", Line: 2}})
	setOutsideDiffFindings(b.Metadata, []types.ReviewIssue{{File: "a.go", Line: 1}})
	got := OutsideDiffFindings(mergeResults([]*types.ReviewResult{a, b}))
	if len(got) != 2 || got[0].File != "a.go" || got[1].File != "b.go" {
		t.Fatalf("merged outside-diff findings = %+v, want both, ordered", got)
	}
}
