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

func TestFilterModelIssuesRoutesProvedOutsideFindingsAndRejectsUnproved(t *testing.T) {
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
	provedOutside := proof
	provedOutside.File, provedOutside.Line, provedOutside.Message = "./other.go", 9, "caller outside the diff now breaks"
	escaping := provedOutside
	escaping.File = "../outside-repo.go"
	issues := []types.ReviewIssue{
		proof,
		{File: "internal/app.go", Line: 4, Severity: "error", RuleID: proof.RuleID, Message: "context line"},
		{File: "other.go", Line: 9, Severity: "warning", RuleID: proof.RuleID, Message: "outside file", Evidence: "maybe"},
		{File: "internal/app.go", Line: 5, Severity: "warning", RuleID: proof.RuleID, Message: "no proof", Evidence: "maybe"},
		provedOutside,
		escaping,
	}

	got, outside, discarded := filterModelIssues(diff, issues)
	if len(got) != 1 || got[0].Message != proof.Message {
		t.Fatalf("kept issues = %#v, want only the proved added-line finding", got)
	}
	if len(outside) != 1 || outside[0].Message != provedOutside.Message || outside[0].File != "other.go" {
		t.Fatalf("outside = %#v, want only the proved finding on other.go (normalized path)", outside)
	}
	if discarded.OutsideAddedLines != 3 || discarded.MissingImpact != 1 || discarded.MissingVerification != 0 || discarded.MissingEvidence != 0 {
		t.Fatalf("discard summary = %#v, want outside=3 impact=1", discarded)
	}
	if discarded.warning() == "" {
		t.Fatal("discarding model findings must produce an operator-visible warning")
	}
}

func TestFilterModelIssuesNeverRoutesOutsideFindingWithoutFullProof(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{Path: "a.go", Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{"+x"}}}}}}
	full := types.ReviewIssue{File: "a.go", Line: 40, Severity: "error", RuleID: "quality/dead-code", Message: "m",
		Evidence: "e", Impact: "i", Verification: "v"}
	for name, mutate := range map[string]func(*types.ReviewIssue){
		"no evidence":     func(i *types.ReviewIssue) { i.Evidence = " " },
		"no impact":       func(i *types.ReviewIssue) { i.Impact = "" },
		"no verification": func(i *types.ReviewIssue) { i.Verification = "" },
		"absolute path":   func(i *types.ReviewIssue) { i.File = "/etc/a.go" },
		"empty path":      func(i *types.ReviewIssue) { i.File = "" },
	} {
		t.Run(name, func(t *testing.T) {
			issue := full
			mutate(&issue)
			kept, outside, discarded := filterModelIssues(diff, []types.ReviewIssue{issue})
			if len(kept) != 0 || len(outside) != 0 || discarded.OutsideAddedLines != 1 {
				t.Fatalf("kept=%v outside=%v discarded=%#v, want a counted discard", kept, outside, discarded)
			}
		})
	}
}

func TestDeletionRegressionUsesOldSideWithoutAdmittingUntouchedCode(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{Path: "handler.go", Hunks: []types.DiffHunk{{
		OldStart: 10, NewStart: 10,
		Lines: []string{" if user == nil {", "-  return errMissingUser", " }", "+persist(user.ID)"},
	}}}}}
	base := types.ReviewIssue{File: "handler.go", Line: 11, Side: "LEFT", Severity: "error",
		RuleID: "quality/dead-code", Message: "Removing the return allows a nil dereference",
		Evidence: "When user == nil, execution now reaches user.ID", Impact: "The handler panics",
		Verification: "Call the handler without a user and check that it returns an error"}
	// Every case carries full proof: a location outside the changed lines
	// is never admitted inline, it is routed to the general comment.
	for _, tc := range []struct {
		name, side, file string
		line             int
		keep             bool
	}{
		{"removed guard", "LEFT", "handler.go", 11, true},
		{"addition", "RIGHT", "handler.go", 12, true},
		{"legacy addition", "", "handler.go", 12, true},
		{"wrong coordinate space", "RIGHT", "handler.go", 11, false},
		{"old context", "LEFT", "handler.go", 10, false},
		{"new context", "RIGHT", "handler.go", 10, false},
		{"unknown side", "BOTH", "handler.go", 11, false},
		{"unrelated file", "LEFT", "other.go", 11, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue := base
			issue.Side, issue.File, issue.Line = tc.side, tc.file, tc.line
			got, outside, _ := filterModelIssues(diff, []types.ReviewIssue{issue})
			if (len(got) == 1) != tc.keep {
				t.Fatalf("kept=%v want=%v", got, tc.keep)
			}
			if (len(outside) == 1) == tc.keep {
				t.Fatalf("outside=%v, want the finding routed to the general comment exactly when it is not kept", outside)
			}
		})
	}
}
