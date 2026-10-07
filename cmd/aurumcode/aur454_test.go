package main

import (
	"strings"
	"testing"

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
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "analysis, model") {
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
