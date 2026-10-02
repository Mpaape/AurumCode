package review

import (
	"context"
	"fmt"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR519ParseSkillSections proves AC-007: every "## " heading of a
// skill's Markdown content becomes one citable Rule, with no code change
// needed to pick up a newly added section on the next run.
func TestAUR519ParseSkillSections(t *testing.T) {
	content := "# Security skill\n\nIntro text, not a section.\n\n" +
		"## No Hardcoded Secrets\n\nseverity: error\nNever commit a literal credential.\n\n" +
		"## Prefer Parameterized Queries\n\nBuild SQL with bound parameters.\n"
	rules := ParseSkillSections("security.md", content, "policy")
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2: %+v", len(rules), rules)
	}
	if got, want := rules[0].ID, "security#no-hardcoded-secrets"; got != want {
		t.Errorf("rule[0].ID = %q, want %q", got, want)
	}
	if rules[0].Severity != "error" {
		t.Errorf("rule[0].Severity = %q, want error (declared)", rules[0].Severity)
	}
	if rules[0].Origin != "policy" {
		t.Errorf("rule[0].Origin = %q, want policy", rules[0].Origin)
	}
	if got, want := rules[1].ID, "security#prefer-parameterized-queries"; got != want {
		t.Errorf("rule[1].ID = %q, want %q", got, want)
	}
	if rules[1].Severity != "warning" {
		t.Errorf("rule[1].Severity = %q, want warning (default)", rules[1].Severity)
	}

	// Adding a third section (simulating the next run, after an edit to the
	// skill file with no code change) makes it citable immediately.
	content += "\n## A Brand New Section\n\nSomething new.\n"
	rules = ParseSkillSections("security.md", content, "policy")
	if len(rules) != 3 {
		t.Fatalf("after adding a section, got %d rules, want 3", len(rules))
	}
	if got, want := rules[2].ID, "security#a-brand-new-section"; got != want {
		t.Errorf("rule[2].ID = %q, want %q", got, want)
	}
}

// TestAUR519EnforceRuleCitationsDynamic proves AC-002: a finding that cites
// a skill/section id not present in the dynamic set is discarded and
// counted as unlinked (the existing "unknown rule_id" discard path), while
// a finding citing a real dynamic rule survives and is enriched exactly
// like a built-in rule.
func TestAUR519EnforceRuleCitationsDynamic(t *testing.T) {
	loader := NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}
	extra := map[string]Rule{
		"security#no-hardcoded-secrets": {
			ID: "security#no-hardcoded-secrets", Title: "No Hardcoded Secrets",
			Description: "Never commit a literal credential.", Severity: "error", Origin: "policy",
		},
	}
	result := &types.ReviewResult{Issues: []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "error", Message: "leak", RuleID: "security#no-hardcoded-secrets"},
		{File: "b.go", Line: 2, Severity: "error", Message: "ghost", RuleID: "security#does-not-exist"},
	}}
	rejected, discarded := enforceRuleCitations(loader, extra, result)
	if rejected != 1 || discarded.Unknown != 1 {
		t.Fatalf("rejected=%d discarded=%+v, want 1 rejected/1 unknown", rejected, discarded)
	}
	if len(result.Issues) != 1 || result.Issues[0].RuleID != "security#no-hardcoded-secrets" {
		t.Fatalf("surviving issues = %+v, want exactly the dynamic-rule citation kept", result.Issues)
	}
}

// TestAUR519ResolveRuleBuiltinWinsOverDynamic proves resolveRule's own
// collision rule: the embedded catalog always wins over a dynamic
// skill-section rule of the same id. A skill can never shadow a built-in
// rule id to relax or hide what it means.
func TestAUR519ResolveRuleBuiltinWinsOverDynamic(t *testing.T) {
	loader := NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}
	builtin, ok := loader.Get("security/sql-injection")
	if !ok {
		t.Fatal("embedded catalog does not carry security/sql-injection; test fixture assumption broken")
	}
	extra := map[string]Rule{
		"security/sql-injection": {ID: "security/sql-injection", Title: "Shadow attempt", Origin: "policy"},
	}
	rule, ok := resolveRule(loader, extra, "security/sql-injection")
	if !ok || rule.Title != builtin.Title || rule.Origin != "" {
		t.Fatalf("resolveRule() = %+v, want the embedded catalog's own rule (Origin \"\"), not the dynamic shadow", rule)
	}
}

// TestAUR519ReviewerCopiesCoverageMetadata is B-C's regression: the
// prompt builder's own per-file coverage counts (code_files_total/
// complete/partial/omitted, computed from the real token budget) used to
// stop at PromptParts.Meta and never reach result.Metadata at all, so a
// file the budget genuinely left out or partially sent was invisible to
// any caller reading result.Metadata (cmd/aurumcode's AUR-476 coverage
// pass and AUR-519's own gate included): it always read as "complete".
// A tiny MaxTokens here forces the prompt builder to omit at least one of
// several changed files; result.Metadata must say so.
func TestAUR519ReviewerCopiesCoverageMetadata(t *testing.T) {
	bigFile := func(name string) types.DiffFile {
		lines := make([]string, 0, 60)
		for i := 0; i < 60; i++ {
			lines = append(lines, fmt.Sprintf("+func Line%d() int { return %d }", i, i))
		}
		return types.DiffFile{Path: name, Hunks: []types.DiffHunk{{NewStart: 1, Lines: lines}}}
	}
	diff := &types.Diff{Files: []types.DiffFile{
		bigFile("a.go"), bigFile("b.go"), bigFile("c.go"),
	}}
	orch := llm.NewOrchestrator(&FakeProvider{Response: `{"summary":"ok","issues":[]}`}, nil, nil)
	reviewer := NewReviewer(orch, Config{MaxTokens: 4200, ReserveReply: 100})

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview() error = %v", err)
	}
	total := result.Metadata["code_files_total"]
	omitted := result.Metadata["code_files_omitted"]
	partial := result.Metadata["code_files_partial"]
	if total == "" {
		t.Fatalf("result.Metadata[code_files_total] is absent -- the fix did not copy promptParts.Meta's coverage counts at all")
	}
	if omitted == "0" && partial == "0" {
		t.Fatalf("expected the tiny budget to omit or partially cover at least one of 3 large files, got total=%s omitted=%s partial=%s", total, omitted, partial)
	}
}
