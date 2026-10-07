package consolidate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func blocksErrors(issue types.ReviewIssue) bool { return issue.Severity == "error" }

// TestAC001TwoPassesSameDefectPublishedOnce: the model and a deterministic
// pass report the same rule at the same line: one finding, the
// deterministic one kept, both evidences, the higher severity, traceable.
func TestAC001TwoPassesSameDefectPublishedOnce(t *testing.T) {
	model := types.ReviewIssue{File: "a.go", Line: 4, RuleID: "security/sql-injection", Severity: "warning", Message: "do modelo",
		Evidence: "evidencia do modelo", Impact: "impacto do modelo"}
	engine := types.ReviewIssue{File: "a.go", Line: 4, RuleID: "security/sql-injection", Severity: "error", Message: "da analise",
		Evidence: "evidencia da analise", Origin: "analysis"}
	res := Apply([]types.ReviewIssue{model, engine}, Options{})
	if len(res.Issues) != 1 || res.Merged != 1 {
		t.Fatalf("AUR-454 duplicate kept: %d finding(s), merged %d", len(res.Issues), res.Merged)
	}
	got := res.Issues[0]
	if got.Origin != "analysis" || got.Severity != "error" || got.Impact != "impacto do modelo" {
		t.Fatalf("consolidated finding = %+v", got)
	}
	if !strings.Contains(got.Evidence, "evidencia do modelo") || !strings.Contains(got.Evidence, "evidencia da analise") {
		t.Fatalf("evidence of one pass lost: %q", got.Evidence)
	}
	if len(res.AlsoFrom[0]) != 1 || res.AlsoFrom[0][0] != "" {
		t.Fatalf("the merged model occurrence is not traceable: %+v", res.AlsoFrom)
	}
}

// TestAC002DistinctProblemsStayDistinctNoCountCut: another rule on the same
// line is another finding, and many distinct findings are all kept.
func TestAC002DistinctProblemsStayDistinctNoCountCut(t *testing.T) {
	in := []types.ReviewIssue{
		{File: "a.go", Line: 4, RuleID: "security/sql-injection", Severity: "error"},
		{File: "a.go", Line: 4, RuleID: "quality/magic-numbers", Severity: "info"},
		{File: "a.go", Line: 4, Side: "LEFT", RuleID: "security/sql-injection", Severity: "error"},
	}
	for i := 0; i < 200; i++ {
		in = append(in, types.ReviewIssue{File: "b.go", Line: i + 1, RuleID: "quality/poor-naming", Severity: "info"})
	}
	res := Apply(in, Options{})
	if len(res.Issues) != len(in) || res.Merged != 0 {
		t.Fatalf("AUR-454 distinct problems merged: %d of %d kept", len(res.Issues), len(in))
	}
}

// TestAC003CollapseIsDeterministicExplainedAndNeverHidesBlocking: the
// preference condenses non-blocking findings of the named severity, in
// order; a blocking one of that severity is never condensed.
func TestAC003CollapseIsDeterministicExplainedAndNeverHidesBlocking(t *testing.T) {
	in := []types.ReviewIssue{
		{File: "a.go", Line: 1, RuleID: "quality/poor-naming", Severity: "info"},
		{File: "a.go", Line: 2, RuleID: "security/xss", Severity: "error"},
		{File: "a.go", Line: 3, RuleID: "quality/magic-numbers", Severity: "info"},
	}
	opts := Options{Collapse: map[string]bool{"info": true, "error": true}, Blocking: blocksErrors}
	first, second := Apply(in, opts), Apply(in, opts)
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatal("collapse is not deterministic")
	}
	if len(first.Issues) != 1 || first.Issues[0].RuleID != "security/xss" {
		t.Fatalf("a blocking finding was condensed or a non-blocking one kept: %+v", first.Issues)
	}
	if len(first.Collapsed) != 2 || first.Collapsed[0].Line != 1 || first.Collapsed[1].Line != 3 {
		t.Fatalf("condensed findings not listed in order: %+v", first.Collapsed)
	}
	if none := Apply(in, Options{Blocking: blocksErrors}); len(none.Issues) != 3 || len(none.Collapsed) != 0 {
		t.Fatalf("without a preference something was condensed: %+v", none)
	}
}
