package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/review/consolidate"
	"github.com/Mpaape/AurumCode/internal/review/rounds"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The same secret reported by the static analysis and by Gitleaks on one
// line is one item of the parecer, naming both sources, with a short
// Portuguese title; another problem on the same line stays its own item.
func TestOneSecretFromTwoSourcesIsOneItem(t *testing.T) {
	issues := []types.ReviewIssue{
		{File: "f.json", Line: 17, Severity: "error", RuleID: "analysis/hardcoded-secret", Origin: "analysis", Message: "Segredo no código casou a regra generic-api-key (Detected a Generic API Key.) (rule analysis/hardcoded-secret)"},
		{File: "f.json", Line: 17, Severity: "error", RuleID: "gitleaks:generic-api-key", Origin: "gitleaks", Message: "Segredo ou credencial escrito no código (regra `gitleaks:generic-api-key`, gitleaks v8.30.1, commit f60b78e4b714)"},
		{File: "f.json", Line: 17, Severity: "error", RuleID: "security/sql-injection", Origin: "security", Message: "Possível injeção de SQL [standards/security-review SCR-001] (rule security/sql-injection: SQL Injection)"},
	}
	rule := blocking.Ungated()
	shown := presentation{consolidate.Apply(issues, consolidate.Options{Blocking: rule.Blocks})}
	result := &types.ReviewResult{Issues: issues, Metadata: map[string]string{}, Limitations: []string{
		"policy gate: gitleaks:generic-api-key - Segredo ou credencial escrito no código (severidade error, limiar error, origem gitleaks, secao repo)",
		"policy gate: revisão inconclusiva — o provedor de modelo não respondeu [provider_failure]",
	}}
	body := formatReviewDocument(shown.publishedResult(result, "pt-BR", rule), nil, "pt-BR", rule)
	if !strings.Contains(body, "Bloqueado: 2 problemas") {
		t.Fatalf("the headline counts the secret twice:\n%s", body)
	}
	fix := body[strings.Index(body, "### Corrigir antes do merge"):strings.Index(body, "<details>")]
	if strings.Count(fix, "`f.json:17`") != 2 {
		t.Fatalf("want the secret once and the SQL injection once:\n%s", fix)
	}
	if !strings.Contains(fix, "Possível segredo no código — apontado por análise estática e Gitleaks") {
		t.Fatalf("the secret item lacks its short title or one of its sources:\n%s", fix)
	}
	if !strings.Contains(fix, "Possível injeção de SQL — apontado por revisão de segurança") {
		t.Fatalf("the other problem on the line was merged or lost its title:\n%s", fix)
	}
	for _, noise := range []string{"Detected a Generic", "Texto original", "(rule ", "[standards/", "[provider_failure]", "severidade error, limiar"} {
		if strings.Contains(body, noise) {
			t.Fatalf("the parecer carries %q:\n%s", noise, body)
		}
	}
	if !strings.Contains(body, "- revisão inconclusiva — o provedor de modelo não respondeu") {
		t.Fatalf("a gate line without a finding must stay, as a sentence:\n%s", body)
	}

	// Under a declared gate, each source is a gate key: the headline still
	// counts the problems, not the keys.
	var found []facts.AuditFinding
	for _, issue := range issues {
		found = append(found, facts.AuditFinding{RuleID: issue.RuleID, Path: issue.File, Line: issue.Line, Severity: issue.Severity})
	}
	gated := blocking.FromGate(true, gate.Result{Fail: true, BlockingFindings: found})
	shown = presentation{consolidate.Apply(issues, consolidate.Options{Blocking: gated.Blocks})}
	if body := formatReviewDocument(shown.publishedResult(result, "pt-BR", gated), nil, "pt-BR", gated); !strings.Contains(body, "Bloqueado: 2 problemas") {
		t.Fatalf("the gated headline counts a source twice:\n%s", body)
	}
}

// aur620Diff adds lines 1-12 of app.py.
func aur620Diff() *types.Diff {
	lines := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		lines = append(lines, "+x = 1")
	}
	return &types.Diff{Files: []types.DiffFile{{Path: "app.py", Hunks: []types.DiffHunk{{NewStart: 1, NewLines: 12, Lines: lines}}}}}
}

// A blocking finding with a concrete fix of its lines carries the fix as a
// GitHub suggestion that replaces exactly those lines; without a fix the
// comment has no block.
func TestBlockingFindingCommentCarriesItsFix(t *testing.T) {
	issue := types.ReviewIssue{File: "app.py", Line: 10, Severity: "error", RuleID: "security/hardcoded-secret", Message: "Possível segredo no código"}
	fixes := []types.ReviewSuggestion{{Kind: "code", File: "app.py", StartLine: 10, EndLine: 10, ProposedCode: `API_KEY = os.environ.get("ASSISTENTE_API_KEY", "")`}}
	c := findingComment(aur620Diff(), fixes, issue, "RIGHT", "**Possível segredo no código**")
	if c.Line != 10 || c.StartLine != 0 || c.Side != "RIGHT" || !strings.HasSuffix(c.Body, "```suggestion\nAPI_KEY = os.environ.get(\"ASSISTENTE_API_KEY\", \"\")\n```") {
		t.Fatalf("the fix is not an applicable suggestion on line 10: %+v", c)
	}
	multi := []types.ReviewSuggestion{{Kind: "code", File: "app.py", StartLine: 9, EndLine: 11, CurrentCode: "x = 1\nx = 1\nx = 1", ProposedCode: "a\nb"}}
	if c := findingComment(aur620Diff(), multi, issue, "RIGHT", "x"); c.StartLine != 9 || c.Line != 11 {
		t.Fatalf("a multi-line fix must replace exactly its range: %+v", c)
	}
	// A multi-line block must quote the lines it replaces, exactly as the
	// diff has them, or applying it could delete code the model never saw.
	unquoted := []types.ReviewSuggestion{{Kind: "code", File: "app.py", StartLine: 9, EndLine: 11, ProposedCode: "a\nb"}}
	if c := findingComment(aur620Diff(), unquoted, issue, "RIGHT", "x"); strings.Contains(c.Body, "```suggestion") {
		t.Fatalf("a multi-line fix without its current code became applicable: %+v", c)
	}
	misquoted := []types.ReviewSuggestion{{Kind: "code", File: "app.py", StartLine: 9, EndLine: 11, CurrentCode: "x = 1", ProposedCode: "a"}}
	if c := findingComment(aur620Diff(), misquoted, issue, "RIGHT", "x"); strings.Contains(c.Body, "```suggestion") {
		t.Fatalf("a fix whose current code is not the diff's lines became applicable: %+v", c)
	}
	masked := []types.ReviewSuggestion{{Kind: "code", File: "app.py", StartLine: 10, EndLine: 10, ProposedCode: "API_KEY = \"[REDACTED]\""}}
	if c := findingComment(aur620Diff(), masked, issue, "RIGHT", "x"); strings.Contains(c.Body, "```suggestion") {
		t.Fatalf("a fix carrying the redaction marker became applicable: %+v", c)
	}
	elsewhere := []types.ReviewSuggestion{{Kind: "code", File: "app.py", StartLine: 3, EndLine: 3, ProposedCode: "y"}}
	if c := findingComment(aur620Diff(), elsewhere, issue, "RIGHT", "x"); strings.Contains(c.Body, "```suggestion") || c.Line != 10 {
		t.Fatalf("a fix of other lines was attached: %+v", c)
	}
	if c := findingComment(aur620Diff(), nil, issue, "RIGHT", "x"); c.Body != "x" {
		t.Fatalf("no fix, no block: %+v", c)
	}
}

// A finding an earlier round commented on alone, now published inside
// another (the same secret from a second source), is still reported: the
// round never calls it resolved nor marks its comment.
func TestAbsorbedFindingIsNotResolvedNextRound(t *testing.T) {
	diff := aur620Diff()
	gitleaks := types.ReviewIssue{File: "app.py", Line: 10, Severity: "error", RuleID: "gitleaks:generic-api-key", Origin: "gitleaks", Message: "segredo"}
	analysis := types.ReviewIssue{File: "app.py", Line: 10, Severity: "error", RuleID: "analysis/hardcoded-secret", Origin: "analysis", Message: "segredo"}
	filter := redaction.NewFilter()
	line := 10
	earlier := githubclient.ReviewHistoryEntry{Kind: "review_comment", ID: 5, Author: "aurum-bot", Path: "app.py", Line: &line,
		Body: "**segredo**\n\n" + rounds.Marker(roundFingerprint(diff, gitleaks, filter), gitleaks.RuleID)}
	p := &prReview{historyEntries: []githubclient.ReviewHistoryEntry{earlier}}
	p.diff, p.filter = diff, filter
	p.deps.env = &reviewEnv{publisherLogin: "aurum-bot"}
	p.shown = presentation{consolidate.Apply([]types.ReviewIssue{analysis, gitleaks}, consolidate.Options{})}
	if len(p.shown.Absorbed) != 1 {
		t.Fatalf("the gitleaks occurrence was not absorbed: %+v", p.shown)
	}
	if plan := p.planRound(); len(plan.plan.Resolved) != 0 {
		t.Fatalf("an absorbed finding was called resolved: %+v", plan.plan.Resolved)
	}
}

// The aurumcode/review status counts problems in Portuguese: one secret two
// sources reported is one; another rule on the line is another problem.
func TestReviewStatusCountsProblems(t *testing.T) {
	issues := []types.ReviewIssue{
		{File: "a.py", Line: 3, Severity: "error", RuleID: "analysis/hardcoded-secret"},
		{File: "a.py", Line: 3, Severity: "error", RuleID: "gitleaks:generic-api-key"},
		{File: "a.py", Line: 3, Severity: "error", RuleID: "security/sql-injection"},
	}
	all := func(types.ReviewIssue) bool { return true }
	if got := distinctIssueProblems(issues, all); got != 2 {
		t.Fatalf("distinct problems = %d, want 2 (the secret once, the SQL injection once)", got)
	}
}
