package review

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/scanner/gitleaks"
	"github.com/Mpaape/AurumCode/internal/scanner/govet"
	"github.com/Mpaape/AurumCode/internal/scanner/semgrep"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// redactedSecretResponse reproduces the measured false positive: the model
// read `APIKey: [REDACTED]` (the redactor's own mask) and reported it as a
// committed secret it never saw. Since AUR-609 an identifier is no longer
// masked, so the fixture diff assigns a quoted literal, which still is. A
// second finding carries the marker only in its evidence; the third is an
// ordinary, provable finding.
const redactedSecretResponse = `{
  "issues": [
    {"file": "profiles/resolve.go", "line": 2, "severity": "error",
     "rule_id": "security/hardcoded-secret",
     "message": "Hardcoded API key: APIKey: [REDACTED]",
     "impact": "Anyone with the repository can use the key.",
     "evidence": "The added line assigns a credential literal.",
     "suggestion": "Load the key from the environment.",
     "verification": "Search the file for the literal after the fix."},
    {"file": "profiles/resolve.go", "line": 2, "severity": "error",
     "rule_id": "security/hardcoded-secret",
     "message": "Credential committed in plain text.",
     "impact": "The credential leaks with the source.",
     "evidence": "APIKey: [REDACTED] is assigned on the added line.",
     "suggestion": "Use a secret manager.",
     "verification": "Rerun the secret scanner."},
    {"file": "profiles/resolve.go", "line": 3, "severity": "warning",
     "rule_id": "quality/dead-code",
     "message": "The returned profile ignores the resolved timeout.",
     "impact": "Callers always get the default timeout.",
     "evidence": "The added return builds the profile without the timeout field.",
     "suggestion": "Pass the resolved timeout into the profile.",
     "verification": "Resolve a profile with a custom timeout and compare."}
  ],
  "summary": "Two findings about the key and one about the timeout."
}`

func redactedSecretDiff() *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: "profiles/resolve.go",
		Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{
			" func resolve(key string, timeout int) Profile {",
			"+\tp := Profile{APIKey: \"not-a-real-key\"}",
			"+\treturn p",
			" }",
		}}},
	}}}
}

// AC-001: a model finding that cites the redaction marker in its message or
// evidence is discarded, counted under its own reason and named in the
// warning; no blocking finding about the masked value survives.
func TestRedactionMarkerAC001ModelFindingDiscarded(t *testing.T) {
	orch := llm.NewOrchestrator(&FakeProvider{Response: redactedSecretResponse}, nil, nil)
	result, err := NewReviewer(orch, DefaultConfig()).GenerateReview(context.Background(), redactedSecretDiff())
	if err != nil {
		t.Fatalf("GenerateReview: %v", err)
	}
	if len(result.Issues) != 1 || result.Issues[0].RuleID != "quality/dead-code" {
		t.Fatalf("kept issues = %+v, want only the timeout finding", result.Issues)
	}
	for _, issue := range result.Issues {
		if issue.Severity == "error" || strings.Contains(issue.Message+issue.Evidence, redaction.Marker) {
			t.Fatalf("a finding about the masked value survived: %+v", issue)
		}
	}
	if got := result.Metadata[RedactionMarkerDiscardKey]; got != "2" {
		t.Fatalf("%s = %q, want 2", RedactionMarkerDiscardKey, got)
	}
	if got := result.Metadata["issues_rejected_by_scope"]; got != "2" {
		t.Fatalf("issues_rejected_by_scope = %q, want 2", got)
	}
	if warning := result.Metadata["scope_discard_warning"]; !strings.Contains(warning, "2 "+redactedMarkerDiscardReason) {
		t.Fatalf("scope_discard_warning = %q, want the redaction-marker reason with count 2", warning)
	}
}

// AC-002: deterministic findings keep their authority even when their text
// carries the marker; only the model's finding is removed.
func TestRedactionMarkerAC002ScannerFindingKept(t *testing.T) {
	text := "APIKey: " + redaction.Marker
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "error", RuleID: "gitleaks/generic-api-key", Message: text, Origin: gitleaks.Name},
		{File: "a.go", Line: 1, Severity: "error", RuleID: "semgrep/secret", Evidence: text, Origin: semgrep.Name},
		{File: "a.go", Line: 1, Severity: "error", RuleID: "govet/printf", Impact: text, Origin: govet.Name},
		{File: "a.go", Line: 1, Severity: "error", RuleID: "security/hardcoded-secret", Suggestion: text, Origin: "security"},
		{File: "a.go", Line: 1, Severity: "error", RuleID: "security/hardcoded-secret", Message: text},
	}
	kept, discarded := discardRedactedModelFindings(issues)
	if discarded != 1 || len(kept) != 4 {
		t.Fatalf("discarded=%d kept=%+v, want only the model finding removed", discarded, kept)
	}
	for _, issue := range kept {
		if issue.Origin == "" {
			t.Fatalf("the model finding citing the marker survived: %+v", issue)
		}
	}
}
