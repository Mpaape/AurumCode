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

// A model finding at the place an engine of the same category already
// reported is the same problem, whatever its rule id: published once,
// under the engine's rule, traceable to the model. Another category at
// the same line stays its own finding.
func TestModelFindingEchoingAnEngineIsMergedByCategory(t *testing.T) {
	model := types.ReviewIssue{File: "r.go", Line: 186, RuleID: "security/weak-crypto", Severity: "warning", Message: "SHA-1 e fraco", Evidence: "sha1.Sum"}
	engine := types.ReviewIssue{File: "r.go", Line: 186, RuleID: "semgrep:go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1", Severity: "warning", Message: "Detected SHA1", Origin: "semgrep"}
	quality := types.ReviewIssue{File: "r.go", Line: 186, RuleID: "quality/long-function", Severity: "info", Message: "funcao longa"}
	res := Apply([]types.ReviewIssue{model, quality, engine}, Options{})
	if len(res.Issues) != 2 || res.Merged != 1 {
		t.Fatalf("want the echo merged and the other category kept: %+v merged=%d", res.Issues, res.Merged)
	}
	var kept types.ReviewIssue
	for i, issue := range res.Issues {
		if issue.Origin == "semgrep" {
			kept = issue
			if len(res.AlsoFrom[i]) != 1 || res.AlsoFrom[i][0] != "" {
				t.Fatalf("the model occurrence is not traceable: %+v", res.AlsoFrom)
			}
		}
	}
	if kept.RuleID != engine.RuleID || !strings.Contains(kept.Evidence, "sha1.Sum") {
		t.Fatalf("merged finding = %+v", kept)
	}
	// Two engines at one line with different rules stay two findings.
	other := engine
	other.RuleID, other.Origin = "gitleaks:generic", "gitleaks"
	if res := Apply([]types.ReviewIssue{engine, other}, Options{}); len(res.Issues) != 2 {
		t.Fatalf("two engine findings merged: %+v", res.Issues)
	}
}

// A model finding of another problem of the same category, and a model
// finding that blocks beside an engine finding that does not, are never
// hidden under the engine's rule: the gate's key survives consolidation.
func TestEchoKeepsTheBlockingRuleAndAnotherProblemApart(t *testing.T) {
	scan := types.ReviewIssue{File: "a.go", Line: 10, RuleID: "semgrep:go.lang.security.audit.weak-crypto", Severity: "info", Message: "weak crypto", Origin: "semgrep"}
	traversal := types.ReviewIssue{File: "a.go", Line: 10, RuleID: "security/path-traversal", Severity: "error", Message: "caminho vindo do usuario"}
	res := Apply([]types.ReviewIssue{traversal, scan}, Options{})
	if len(res.Issues) != 2 {
		t.Fatalf("another problem of the same category was merged: %+v", res.Issues)
	}
	// The same problem, where only the model's occurrence is what the gate
	// keyed its decision on: the model's rule and message are kept.
	crypto := types.ReviewIssue{File: "a.go", Line: 10, RuleID: "security/weak-crypto", Severity: "error", Message: "SHA-1 nao serve para assinatura", Evidence: "sha1.Sum"}
	blocksCrypto := func(i types.ReviewIssue) bool { return i.RuleID == "security/weak-crypto" }
	res = Apply([]types.ReviewIssue{crypto, scan}, Options{Blocking: blocksCrypto})
	if len(res.Issues) != 1 || res.Merged != 1 {
		t.Fatalf("the echo was not merged: %+v", res.Issues)
	}
	got := res.Issues[0]
	if !blocksCrypto(got) || got.Message != crypto.Message || got.Severity != "error" || !strings.Contains(got.Evidence, "sha1.Sum") {
		t.Fatalf("the blocking occurrence lost its rule or message: %+v", got)
	}
	if len(res.AlsoFrom[0]) != 1 || res.AlsoFrom[0][0] != "semgrep" {
		t.Fatalf("the merged engine occurrence is not traceable: %+v", res.AlsoFrom)
	}
}
