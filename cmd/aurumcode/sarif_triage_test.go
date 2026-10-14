package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A deterministic finding the model disputed and the gate's triage stopped
// counting is left out of the SARIF: code scanning does not dismiss an alert
// from a SARIF suppression, so a refuted finding must disappear for the
// alert to close. A finding nobody disputed stays an open result.
func TestSARIFLeavesOutModelDisputedFindings(t *testing.T) {
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
	if why, present := got["gitleaks:generic-api-key"]; present {
		t.Fatalf("the disputed finding must be left out of the SARIF, got %q", why)
	}
	if got["security/sql-injection"] != "open" {
		t.Fatalf("an undisputed finding must stay open, got %q", got["security/sql-injection"])
	}
}
