package types

import (
	"encoding/json"
	"testing"
)

func TestDiffJSONDeterminism(t *testing.T) {
	diff := &Diff{
		Files: []DiffFile{
			{
				Path: "example.go",
				Lang: "go",
				Hunks: []DiffHunk{
					{
						OldStart: 1,
						OldLines: 2,
						NewStart: 1,
						NewLines: 3,
						Lines:    []string{"+line1", "-line2", "+line3"},
					},
				},
			},
		},
	}

	// Marshal twice and compare
	json1, err1 := json.Marshal(diff)
	if err1 != nil {
		t.Fatalf("First marshal failed: %v", err1)
	}

	json2, err2 := json.Marshal(diff)
	if err2 != nil {
		t.Fatalf("Second marshal failed: %v", err2)
	}

	if string(json1) != string(json2) {
		t.Errorf("Non-deterministic JSON output:\n%s\nvs\n%s", string(json1), string(json2))
	}
}

func TestReviewResultJSONDeterminism(t *testing.T) {
	result := &ReviewResult{
		Issues: []ReviewIssue{
			{
				ID:       "issue-1",
				File:     "example.go",
				Line:     10,
				Severity: "warning",
				RuleID:   "security-001",
				Message:  "Potential security issue",
			},
		},
		ISOScores: &ISOScores{
			Functionality:   80,
			Reliability:     75,
			Usability:       70,
			Efficiency:      85,
			Maintainability: 90,
			Portability:     85,
			Security:        60,
			Compatibility:   80,
		},
		Summary: "Code review completed",
	}

	// Marshal twice and compare
	json1, err1 := json.Marshal(result)
	if err1 != nil {
		t.Fatalf("First marshal failed: %v", err1)
	}

	json2, err2 := json.Marshal(result)
	if err2 != nil {
		t.Fatalf("Second marshal failed: %v", err2)
	}

	if string(json1) != string(json2) {
		t.Errorf("Non-deterministic JSON output:\n%s\nvs\n%s", string(json1), string(json2))
	}
}

func TestReviewIssueZeroValues(t *testing.T) {
	var issue ReviewIssue

	if issue.ID != "" {
		t.Errorf("Expected empty ID, got %s", issue.ID)
	}

	if issue.Severity != "" {
		t.Errorf("Expected empty severity, got %s", issue.Severity)
	}

	if issue.Line != 0 {
		t.Errorf("Expected line 0, got %d", issue.Line)
	}
}

func TestISOScoresZeroValues(t *testing.T) {
	var scores ISOScores

	if scores.Functionality != 0 {
		t.Errorf("Expected 0 for Functionality, got %d", scores.Functionality)
	}

	if scores.Security != 0 {
		t.Errorf("Expected 0 for Security, got %d", scores.Security)
	}
}
