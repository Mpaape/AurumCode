package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A deterministic finding the model disputed and the gate's triage stopped
// counting goes to the SARIF suppressed, with the model's reason: code
// scanning must not show as an error what the gate decided against. A
// finding nobody disputed stays an open result.
func TestSARIFCarriesModelDisputedFindingsSuppressed(t *testing.T) {
	disputed := types.ReviewIssue{
		RuleID: "gitleaks:generic-api-key", File: "demo/fixture.json", Line: 17, Severity: "error",
		Message:    "segredo no codigo",
		Assessment: &types.EvidenceAssessment{Status: types.AssessmentDisputed, Justification: "canario sintetico do teste"},
	}
	kept := types.ReviewIssue{RuleID: "security/sql-injection", File: "app.go", Line: 3, Severity: "error", Message: "injecao"}
	path := filepath.Join(t.TempDir(), "out.sarif")
	in := complianceArtifactInputs{
		sarifPath: path,
		issues:    []types.ReviewIssue{disputed, kept},
		demoted:   []types.ReviewIssue{disputed},
	}
	if err := writeSARIFFile(in, redaction.NewFilter()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runs []struct {
			Results []struct {
				RuleID       string `json:"ruleId"`
				Suppressions []struct {
					Justification string `json:"justification"`
				} `json:"suppressions"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid SARIF: %v", err)
	}
	got := map[string]string{}
	for _, r := range doc.Runs[0].Results {
		why := "open"
		if len(r.Suppressions) > 0 {
			why = r.Suppressions[0].Justification
		}
		got[r.RuleID] = why
	}
	if !strings.Contains(got["gitleaks:generic-api-key"], "contestado pelo modelo") || !strings.Contains(got["gitleaks:generic-api-key"], "canario sintetico") {
		t.Fatalf("the disputed finding must be suppressed with the model's reason, got %q", got["gitleaks:generic-api-key"])
	}
	if got["security/sql-injection"] != "open" {
		t.Fatalf("an undisputed finding must stay open, got %q", got["security/sql-injection"])
	}
}
