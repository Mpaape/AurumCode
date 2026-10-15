package main

import (
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// The same finding reported by two analysts is one problem: the review
// status counts it once and never reads "nenhum achado grave" while it
// fails (seen on the demo: state failure, description "nenhum achado grave").
func TestReviewStatusNeverSaysNothingWhileItFails(t *testing.T) {
	finding := types.ReviewIssue{File: "plugins/vvah/perfil.py", Line: 57, RuleID: "time#qa-001-comportamento-novo-tem-teste", Severity: "error"}
	issues := []types.ReviewIssue{finding, finding}
	counts := func(types.ReviewIssue) bool { return true }
	if got := distinctIssueProblems(issues, counts); got != 1 {
		t.Fatalf("distinctProblems = %d, want 1", got)
	}
	if got := reviewCheckDescription("pt-BR", 14, 1, false, false); got == "nenhum achado grave" {
		t.Fatalf("a failing review status said %q", got)
	}
}
