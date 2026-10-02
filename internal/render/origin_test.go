package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// AuditFinding.Origin is serialized as "origin" and omitted when empty.
func TestAuditFindingOriginSerialization(t *testing.T) {
	with, err := json.Marshal(AuditFinding{RuleID: "r", Path: "a.go", Line: 1, Severity: "error", Origin: "analysis"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(with), `"origin":"analysis"`) {
		t.Fatalf("origin missing: %s", with)
	}
	without, _ := json.Marshal(AuditFinding{RuleID: "r", Path: "a.go", Line: 1, Severity: "error"})
	if strings.Contains(string(without), "origin") {
		t.Fatalf("empty origin must be omitted: %s", without)
	}
}

// The audit origin passes through redaction like every other string field.
func TestAuditFindingOriginRedacted(t *testing.T) {
	secret := "ORIGIN-SECRET-123"
	rec := BuildAuditRecord("", "", "o/r", "sha", "m", "v", AuditGate{Decision: "fail"},
		[]AuditFinding{{RuleID: "r", Path: "a.go", Line: 1, Severity: "error", Origin: "analysis" + secret}}, nil, true, nil)
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := WriteAuditRecord(path, rec, redaction.NewFilter(secret)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), secret) || !strings.Contains(string(data), `"origin"`) {
		t.Fatalf("origin must be present and redacted:\n%s", data)
	}
}

// properties.origin is written when set, omitted when empty, and redacted.
func TestSARIFPropertiesOrigin(t *testing.T) {
	secret := "SARIF-ORIGIN-SECRET"
	findings := []SARIFFinding{
		{RuleID: "r1", Path: "a.go", Line: 1, Severity: "error", Message: "m", Context: "c", Origin: "analysis"},
		{RuleID: "r2", Path: "b.go", Line: 2, Severity: "error", Message: "m", Context: "c"},
		{RuleID: "r3", Path: "c.go", Line: 3, Severity: "error", Message: "m", Context: "c", Origin: "sast" + secret},
	}
	path := filepath.Join(t.TempDir(), "out.sarif")
	if err := WriteSARIF(path, "v", findings, true, "", redaction.NewFilter(secret)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), secret) {
		t.Fatalf("origin leaked past redaction:\n%s", data)
	}
	var doc struct {
		Runs []struct {
			Results []struct {
				RuleID     string `json:"ruleId"`
				Properties *struct {
					Origin string `json:"origin"`
				} `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	has := map[string]bool{}
	for _, r := range doc.Runs[0].Results {
		if r.Properties != nil {
			got[r.RuleID], has[r.RuleID] = r.Properties.Origin, true
		}
	}
	if got["r1"] != "analysis" {
		t.Errorf("r1 origin=%q", got["r1"])
	}
	if has["r2"] {
		t.Error("a finding without origin must carry no properties")
	}
	if !has["r3"] {
		t.Error("r3 must keep its (redacted) origin property")
	}
}
