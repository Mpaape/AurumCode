package review

import (
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAddedLinesForTestingTracksCurrentSideOnly(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "internal/app.go",
		Hunks: []types.DiffHunk{{
			NewStart: 10,
			Lines:    []string{"-old", " context", "+new", "+another"},
		}},
	}}}

	want := map[string][]int{"internal/app.go": {11, 12}}
	if got := addedLinesForTesting(diff); !reflect.DeepEqual(got, want) {
		t.Fatalf("added lines = %#v, want %#v", got, want)
	}
}

func TestFilterModelIssuesRejectsUnscopedAndUnprovedFindings(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  "internal/app.go",
		Hunks: []types.DiffHunk{{NewStart: 4, Lines: []string{" context", "+changed"}}},
	}}}
	proof := types.ReviewIssue{
		File: "internal/app.go", Line: 5, Severity: "warning", RuleID: "quality/dead-code",
		Message:      "the changed branch is unreachable",
		Evidence:     "the new branch returns before its condition can be evaluated",
		Impact:       "the intended behavior is never reached",
		Verification: "run the focused branch test and observe the branch counter",
	}
	issues := []types.ReviewIssue{
		proof,
		{File: "internal/app.go", Line: 4, Severity: "error", RuleID: proof.RuleID, Message: "context line"},
		{File: "other.go", Line: 9, Severity: "warning", RuleID: proof.RuleID, Message: "outside file"},
		{File: "internal/app.go", Line: 5, Severity: "warning", RuleID: proof.RuleID, Message: "no proof", Evidence: "maybe"},
	}

	got, discarded := filterModelIssues(diff, issues)
	if len(got) != 1 || got[0].Message != proof.Message {
		t.Fatalf("kept issues = %#v, want only the proved added-line finding", got)
	}
	if discarded.OutsideAddedLines != 2 || discarded.MissingImpact != 1 || discarded.MissingVerification != 0 || discarded.MissingEvidence != 0 {
		t.Fatalf("discard summary = %#v, want outside=2 impact=1", discarded)
	}
	if discarded.warning() == "" {
		t.Fatal("discarding model findings must produce an operator-visible warning")
	}
}
