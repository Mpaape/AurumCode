package render

import "testing"

// #177: three analysts reporting the same rule on the same line are one
// SARIF result, with the highest severity.
func TestMergeSameFindingsKeepsOnePerRuleAndLine(t *testing.T) {
	in := []SARIFFinding{
		{RuleID: "time#qa-001", Path: "assistente.py", Line: 17, Severity: "warning", Message: "a"},
		{RuleID: "time#qa-001", Path: "assistente.py", Line: 17, Severity: "error", Message: "b"},
		{RuleID: "time#qa-001", Path: "assistente.py", Line: 17, Severity: "info", Message: "c"},
		{RuleID: "time#qa-001", Path: "assistente.py", Line: 18, Severity: "info"},
		{RuleID: "ia#ia-001", Path: "assistente.py", Line: 17, Severity: "warning"},
	}
	out := MergeSameFindings(in)
	if len(out) != 3 {
		t.Fatalf("want 3 results, got %d: %+v", len(out), out)
	}
	if out[0].Severity != "error" || out[0].Message != "a" {
		t.Fatalf("merged result: %+v", out[0])
	}
	sup := MergeSameFindings([]SARIFFinding{
		{RuleID: "r", Path: "p", Line: 1, Suppressed: true, Justification: "j"},
		{RuleID: "r", Path: "p", Line: 1},
	})
	if len(sup) != 1 || sup[0].Suppressed {
		t.Fatalf("a copy that is not suppressed keeps the result open: %+v", sup)
	}
}
