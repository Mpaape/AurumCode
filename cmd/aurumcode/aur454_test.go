package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/review/consolidate"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR454PublishedBodyExplainsMergeAndCollapse: the review body names
// the consolidated finding's sources and lists every condensed finding.
func TestAUR454PublishedBodyExplainsMergeAndCollapse(t *testing.T) {
	in := []types.ReviewIssue{
		{File: "a.go", Line: 4, RuleID: "security/sql-injection", Severity: "error", Message: "consulta montada", Evidence: "do modelo"},
		{File: "a.go", Line: 4, RuleID: "security/sql-injection", Severity: "error", Message: "consulta montada", Evidence: "da analise", Origin: "analysis"},
		{File: "a.go", Line: 9, RuleID: "quality/poor-naming", Severity: "info", Message: "nome"},
	}
	shown := presentation{consolidate.Apply(in, consolidate.Options{Collapse: map[string]bool{"info": true}, Blocking: func(i types.ReviewIssue) bool { return i.Severity == "error" }})}
	for _, language := range []string{"pt-BR", "en-US"} {
		issues := shown.withSources(language)
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "analysis, "+i18n.Text(language, "review.presentation_model_source")) {
			t.Fatalf("(%s) consolidated finding not traceable: %+v", language, issues)
		}
		body := appendPresentationNotice("corpo", shown, language)
		if !strings.Contains(body, "`quality/poor-naming` a.go:9") || !strings.Contains(body, "review.presentation.collapse") {
			t.Fatalf("(%s) condensed finding not explained:\n%s", language, body)
		}
	}
	if body := appendPresentationNotice("corpo", presentation{consolidate.Apply(in[2:], consolidate.Options{})}, "en-US"); body != "corpo" {
		t.Fatalf("nothing merged or condensed, body changed:\n%s", body)
	}
}

// TestAUR454InconclusiveGateCondensesNothing: a declared gate that fails
// without naming a blocking finding, or any inconclusive run, condenses
// nothing, so collapse: [error] cannot group a finding that may block.
func TestAUR454InconclusiveGateCondensesNothing(t *testing.T) {
	failingNoKeys := blocking.FromGate(true, gate.Result{Active: true, Fail: true, Inconclusive: true})
	if collapseAllowed(failingNoKeys, false) {
		t.Fatal("a failing gate without blocking findings allowed condensing")
	}
	if collapseAllowed(blocking.Ungated(), true) {
		t.Fatal("an inconclusive run allowed condensing")
	}
	breach := blocking.FromGate(true, gate.Result{Active: true, Fail: true, BlockingFindings: []facts.AuditFinding{{RuleID: "r", Path: "a.go", Line: 1}}})
	if !collapseAllowed(breach, false) || !collapseAllowed(blocking.Ungated(), false) {
		t.Fatal("a conclusive run with a known blocking rule refused the preference")
	}
}
