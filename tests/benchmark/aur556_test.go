package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// AC-004: in the versioned report, a defect the embedded analysis detects no
// longer slips through approved, and detection did not drop.
func TestAUR556VersionedReportApprovesNoDetectedDefect(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(mlReportDir, mlReportJS))
	if err != nil {
		t.Fatal(err)
	}
	var rep MLReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	for _, row := range rep.Languages {
		if row.Language == "java" || row.Language == "python" {
			if row.ApprovedWithDefect != 0 {
				t.Errorf("%s: approved with defect = %d (%v), want 0", row.Language, row.ApprovedWithDefect, row.ApprovedWithDefectID)
			}
		}
		if row.Detected < row.Defects-1 || (row.Language != "php" && row.Detected != row.Defects) {
			t.Errorf("%s: recall regressed (detected %d of %d)", row.Language, row.Detected, row.Defects)
		}
	}
	// Every approved-with-defect case left is an undetected one.
	if rep.Total.ApprovedWithDefect > rep.Total.Defects-rep.Total.Detected {
		t.Errorf("approved with defect (%d) exceeds undetected defects (%d)", rep.Total.ApprovedWithDefect, rep.Total.Defects-rep.Total.Detected)
	}
}

// Approved requires exit 0, whatever the gate decision says.
func TestAUR556ApprovedRequiresExitZero(t *testing.T) {
	if (Outcome{ExitCode: 3, GateDecision: "pass"}).Approved() {
		t.Fatal("exit 3 with gate pass must not be approved")
	}
	if !(Outcome{ExitCode: 0, GateDecision: "pass"}).Approved() {
		t.Fatal("exit 0 with gate pass is approved")
	}
	if (Outcome{ExitCode: 0, GateDecision: "fail"}).Approved() {
		t.Fatal("a failed gate is never approval")
	}
}

// A case added without regenerating the manifest is refused.
func TestAUR556CaseWithoutManifestRefused(t *testing.T) {
	corpus, err := EnumerateMLCorpus(mlRoot)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	copyTree(t, mlRoot, tmp)
	victim := corpus.Cases[0]
	copyTree(t, filepath.Join(mlRoot, "cases", victim.ID), filepath.Join(tmp, "cases", "added-case"))
	raw, err := os.ReadFile(filepath.Join(tmp, "cases", "added-case", multilangCaseFile))
	if err != nil {
		t.Fatal(err)
	}
	var mc map[string]any
	if err := json.Unmarshal(raw, &mc); err != nil {
		t.Fatal(err)
	}
	mc["id"] = "added-case"
	raw, _ = json.Marshal(mc)
	if err := os.WriteFile(filepath.Join(tmp, "cases", "added-case", multilangCaseFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := EnumerateMLCorpus(tmp)
	if err != nil {
		t.Fatalf("the added case itself must be well-formed: %v", err)
	}
	if err := VerifyMLCorpus(added); err == nil {
		t.Fatal("a case without a manifest entry must be refused")
	}
}

// Editing one byte of a copy of the policy changes policy_sha256 in the header.
func TestAUR556PolicyByteChangesHeaderDigest(t *testing.T) {
	corpus, err := EnumerateMLCorpus(mlRoot)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	copyTree(t, mlRoot, tmp)
	p := filepath.Join(tmp, "policy", ".aurumcode", "config.yml")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-2] ^= 0x01
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	edited, err := EnumerateMLCorpus(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if edited.PolicySHA256 == corpus.PolicySHA256 {
		t.Fatal("one edited policy byte must change the policy digest")
	}
	a, _ := BuildMLReport(corpus, nil).JSON()
	b, _ := BuildMLReport(edited, nil).JSON()
	if string(a) == string(b) {
		t.Fatal("the report header must carry the new policy_sha256")
	}
	var rep MLReport
	_ = json.Unmarshal(b, &rep)
	if rep.PolicySHA256 != edited.PolicySHA256 {
		t.Fatalf("header policy_sha256=%s, want %s", rep.PolicySHA256, edited.PolicySHA256)
	}
}
