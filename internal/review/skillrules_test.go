package review

import (
	"testing"

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
