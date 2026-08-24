package review

import (
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestSuppressWorkflowReferenceFindingsKeepsLiteralSecrets(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: ".github/workflows/review.yml",
		Hunks: []types.DiffHunk{{
			NewStart: 1,
			Lines: []string{
				"+types: [opened, synchronize, reopened]",
				"+contents: read",
				"+LLM_API_KEY: ${{ secrets.LLM_API_KEY }}",
				"+API_KEY: AURUM-FAKE-KEY-9000",
			},
		}},
	}}}
	issues := []types.ReviewIssue{
		{File: ".github/workflows/review.yml", Line: 1, RuleID: "security/hardcoded-secret"},
		{File: ".github/workflows/review.yml", Line: 2, RuleID: "security/hardcoded-secret"},
		{File: ".github/workflows/review.yml", Line: 3, RuleID: "security/hardcoded-secret"},
		{File: ".github/workflows/review.yml", Line: 4, RuleID: "security/hardcoded-secret"},
	}

	got := suppressWorkflowReferenceFindings(diff, issues)
	if len(got) != 1 || got[0].Line != 4 {
		t.Fatalf("safe workflow references should be suppressed while the literal remains: %+v", got)
	}
}

func TestSuppressWorkflowReferenceFindingsDoesNotBroadenToOtherFiles(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  "config/app.yml",
		Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{"+contents: read"}}},
	}}}
	issues := []types.ReviewIssue{{File: "config/app.yml", Line: 1, RuleID: "security/hardcoded-secret"}}

	got := suppressWorkflowReferenceFindings(diff, issues)
	if len(got) != 1 {
		t.Fatalf("non-workflow findings must remain visible: %+v", got)
	}
}

func TestSuppressWorkflowReferenceFindingsDropsBlankWorkflowLine(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  ".github/workflows/review.yml",
		Hunks: []types.DiffHunk{{NewStart: 5, Lines: []string{"+types: [opened]", "+", "+permissions:"}}},
	}}}
	issues := []types.ReviewIssue{{File: ".github/workflows/review.yml", Line: 6, RuleID: "security/hardcoded-secret"}}

	if got := suppressWorkflowReferenceFindings(diff, issues); len(got) != 0 {
		t.Fatalf("a model finding on a blank workflow line must be suppressed: %+v", got)
	}
}

func TestSuppressWorkflowReferenceFindingsDropsWorkflowLineOutsideDiff(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  ".github/workflows/review.yml",
		Hunks: []types.DiffHunk{{NewStart: 7, Lines: []string{" permissions:", "+  statuses: write"}}},
	}}}
	issues := []types.ReviewIssue{{File: ".github/workflows/review.yml", Line: 5, RuleID: "security/hardcoded-secret"}}

	if got := suppressWorkflowReferenceFindings(diff, issues); len(got) != 0 {
		t.Fatalf("a workflow secret finding outside the reviewed hunk must not block this diff: %+v", got)
	}
}
