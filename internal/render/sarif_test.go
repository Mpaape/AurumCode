package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// sarifDoc is a loosely-typed view used only by these tests to check the
// SARIF 2.1.0 fields AC-002 requires, without depending on this package's
// own (unexported) structs.
type sarifDoc struct {
	Version string `json:"version"`
	Schema  string `json:"$schema"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				Rules   []struct {
					ID               string `json:"id"`
					ShortDescription struct {
						Text string `json:"text"`
					} `json:"shortDescription"`
				} `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Invocations []struct {
			ExecutionSuccessful        bool `json:"executionSuccessful"`
			ToolExecutionNotifications []struct {
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"toolExecutionNotifications"`
		} `json:"invocations"`
		Results []struct {
			RuleID  string `json:"ruleId"`
			Level   string `json:"level"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
			PartialFingerprints map[string]string `json:"partialFingerprints"`
			Suppressions        []struct {
				Kind          string `json:"kind"`
				Justification string `json:"justification"`
			} `json:"suppressions"`
		} `json:"results"`
	} `json:"runs"`
}

// TestAUR521SARIFRequiredFields covers AC-002: the document carries the
// 2.1.0 version, one run with tool.driver name/version/rules, and a result
// with ruleId/level/message/location(uri+line)/a non-empty fingerprint.
func TestAUR521SARIFRequiredFields(t *testing.T) {
	log := BuildSARIFLog("1.2.3", []SARIFFinding{
		{
			RuleID:    "security#no-hardcoded-secrets",
			RuleTitle: "No Hardcoded Secrets",
			Path:      "app.go",
			Line:      3,
			Severity:  "error",
			Message:   "Hardcoded secret",
			Context:   `dbPassword := "hunter2"`,
		},
	}, true, "")

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	var doc sarifDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != "2.1.0" {
		t.Fatalf("version=%q, want 2.1.0", doc.Version)
	}
	if len(doc.Runs) != 1 {
		t.Fatalf("runs=%d, want 1", len(doc.Runs))
	}
	run := doc.Runs[0]
	if run.Tool.Driver.Name == "" || run.Tool.Driver.Version != "1.2.3" {
		t.Fatalf("tool.driver=%+v, want name set and version 1.2.3", run.Tool.Driver)
	}
	if len(run.Tool.Driver.Rules) != 1 || run.Tool.Driver.Rules[0].ID != "security#no-hardcoded-secrets" {
		t.Fatalf("rules=%+v", run.Tool.Driver.Rules)
	}
	if run.Tool.Driver.Rules[0].ShortDescription.Text != "No Hardcoded Secrets" {
		t.Fatalf("rule title=%q, want the skill section title", run.Tool.Driver.Rules[0].ShortDescription.Text)
	}
	if len(run.Results) != 1 {
		t.Fatalf("results=%d, want 1", len(run.Results))
	}
	res := run.Results[0]
	if res.RuleID != "security#no-hardcoded-secrets" || res.Level != "error" {
		t.Fatalf("result ruleId/level=%q/%q", res.RuleID, res.Level)
	}
	if len(res.Locations) != 1 || res.Locations[0].PhysicalLocation.ArtifactLocation.URI != "app.go" || res.Locations[0].PhysicalLocation.Region.StartLine != 3 {
		t.Fatalf("result location=%+v", res.Locations)
	}
	fp := res.PartialFingerprints[FindingFingerprintKey]
	if fp == "" {
		t.Fatal("result must carry a non-empty partial fingerprint")
	}
	if run.Invocations[0].ExecutionSuccessful != true {
		t.Fatal("a conclusive run must mark executionSuccessful true")
	}
}

// TestAUR521SARIFFingerprintStableAcrossWrites covers AC-002/MUT-001 at the
// SARIF-writer level: writing the same finding twice must produce the exact
// same partialFingerprints value both times.
func TestAUR521SARIFFingerprintStableAcrossWrites(t *testing.T) {
	finding := SARIFFinding{RuleID: "r1", Path: "app.go", Line: 10, Severity: "warning", Message: "m", Context: "x := 1"}
	first := BuildSARIFLog("v", []SARIFFinding{finding}, true, "")
	second := BuildSARIFLog("v", []SARIFFinding{finding}, true, "")
	fp1 := first.Runs[0].Results[0].PartialFingerprints[FindingFingerprintKey]
	fp2 := second.Runs[0].Results[0].PartialFingerprints[FindingFingerprintKey]
	if fp1 == "" || fp1 != fp2 {
		t.Fatalf("fingerprint not stable across two SARIF builds of the same finding: %q vs %q", fp1, fp2)
	}
}

// TestAUR521SARIFSuppressionForExceptedFinding covers AC-003: a finding
// marked as excepted (synthetic until AUR-520 lands and starts populating
// this from a real exception) renders into SARIF's own suppressions shape
// with its justification.
func TestAUR521SARIFSuppressionForExceptedFinding(t *testing.T) {
	log := BuildSARIFLog("v", []SARIFFinding{
		{RuleID: "r1", Path: "app.go", Line: 1, Severity: "error", Message: "m", Context: "x",
			Suppressed: true, Justification: "accepted risk: tracked in TICKET-1"},
	}, true, "")
	res := log.Runs[0].Results[0]
	if len(res.Suppressions) != 1 {
		t.Fatalf("suppressions=%v, want exactly one", res.Suppressions)
	}
	if res.Suppressions[0].Justification != "accepted risk: tracked in TICKET-1" {
		t.Fatalf("suppression justification=%q", res.Suppressions[0].Justification)
	}
	if res.Suppressions[0].Kind != "external" {
		t.Fatalf("suppression kind=%q, want external", res.Suppressions[0].Kind)
	}
}

// TestAUR521SARIFInconclusiveRun covers AC-004: an inconclusive run still
// produces a structurally valid SARIF document, with
// invocations[0].executionSuccessful=false and a notification naming why.
func TestAUR521SARIFInconclusiveRun(t *testing.T) {
	log := BuildSARIFLog("v", nil, false, "provider_failure")
	inv := log.Runs[0].Invocations[0]
	if inv.ExecutionSuccessful {
		t.Fatal("inconclusive run must mark executionSuccessful false")
	}
	if len(inv.ToolExecutionNotifications) != 1 || inv.ToolExecutionNotifications[0].Message.Text != "provider_failure" {
		t.Fatalf("notifications=%+v, want one naming provider_failure", inv.ToolExecutionNotifications)
	}
}

// TestAUR521SARIFRedactsSecretCanary covers AC-005 for the SARIF sink: a
// canary present in a finding's message/context must not survive into the
// written SARIF file.
func TestAUR521SARIFRedactsSecretCanary(t *testing.T) {
	const canary = "AURUM-CANARY-sarif-f00d"
	t.Setenv(redaction.CanaryEnv, canary)
	filter := redaction.FromEnv()

	findings := []SARIFFinding{{
		RuleID: "r1", Path: "app.go", Line: 1, Severity: "error",
		Message: "leaked: " + canary, Context: "token = \"" + canary + "\"",
	}}
	path := filepath.Join(t.TempDir(), "out.sarif")
	if err := WriteSARIF(path, "v", findings, true, "", filter); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), canary) {
		t.Fatalf("canary leaked into SARIF file: %s", data)
	}
	var doc sarifDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("SARIF file is not valid JSON after redaction: %v\n%s", err, data)
	}
}
