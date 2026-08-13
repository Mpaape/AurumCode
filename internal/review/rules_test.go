package review

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestRulesLoaderEmbeddedCatalog proves the embedded catalog loads with
// rules in it and that lookups fail closed for missing or unknown ids.
func TestRulesLoaderEmbeddedCatalog(t *testing.T) {
	loader := NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() of the embedded catalog must succeed, got: %v", err)
	}
	if len(loader.GetAll()) == 0 {
		t.Fatal("embedded catalog must not be empty")
	}
	rule, ok := loader.Get("security/hardcoded-secret")
	if !ok || rule.Title != "Hardcoded Secrets" || rule.Severity != "error" {
		t.Fatalf("expected security/hardcoded-secret with full metadata, got %+v (ok=%v)", rule, ok)
	}
	if _, ok := loader.Get(""); ok {
		t.Fatal("an empty rule id must never resolve")
	}
	if _, ok := loader.Get("no/such-rule"); ok {
		t.Fatal("an unknown rule id must never resolve")
	}
	if len(loader.GetByCategory("security")) == 0 {
		t.Fatal("expected security rules in the embedded catalog")
	}
}

// TestRulesLoaderFailsLoud proves the failure modes the card names: a
// catalog with no rule files, a catalog whose files declare zero rules, a
// rule without an id, and a duplicated id are all loud errors, never a
// silent zero-rule load.
func TestRulesLoaderFailsLoud(t *testing.T) {
	cases := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{
			name: "no rule files at all",
			fsys: fstest.MapFS{},
			want: "no embedded review rules files",
		},
		{
			name: "files with zero rules",
			fsys: fstest.MapFS{
				"rules/empty.yml": {Data: []byte("rules: []\n")},
			},
			want: "catalog is empty",
		},
		{
			name: "rule without id",
			fsys: fstest.MapFS{
				"rules/bad.yml": {Data: []byte("rules:\n  - title: No Id\n    severity: error\n")},
			},
			want: "empty id",
		},
		{
			name: "duplicate rule id",
			fsys: fstest.MapFS{
				"rules/a.yml": {Data: []byte("rules:\n  - id: x/dup\n    title: A\n")},
				"rules/b.yml": {Data: []byte("rules:\n  - id: x/dup\n    title: B\n")},
			},
			want: "duplicate rule id",
		},
		{
			name: "malformed yaml",
			fsys: fstest.MapFS{
				"rules/broken.yml": {Data: []byte("rules: [unclosed\n")},
			},
			want: "failed to load",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewRulesLoader().loadFrom(tc.fsys)
			if err == nil {
				t.Fatalf("expected a loud error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error mentioning %q, got: %v", tc.want, err)
			}
		})
	}
}

// TestEnforceRuleCitations proves the gate itself: rejection of issues
// without a resolvable rule (the NEW logic this card adds over the
// restored c12d7ab code) plus the restored mapRulesToIssues enrichment
// (empty Message/Severity filled from the rule, file path cleaned) and the
// appended citation.
func TestEnforceRuleCitations(t *testing.T) {
	loader := NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}

	result := &types.ReviewResult{
		Issues: []types.ReviewIssue{
			{File: "a/b/../c.go", Line: 1, RuleID: "security/hardcoded-secret", Severity: "error", Message: "planted secret"},
			{File: "d.go", Line: 2, Severity: "error", Message: "no rule cited"},
			{File: "e.go", Line: 3, RuleID: "no/such-rule", Severity: "warning", Message: "unknown rule cited"},
			{File: "f.go", Line: 4, RuleID: "security/sql-injection"},
		},
	}

	rejected := enforceRuleCitations(loader, result)
	if rejected != 2 {
		t.Fatalf("expected 2 rejected issues, got %d: %+v", rejected, result.Issues)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 surviving issues, got %+v", result.Issues)
	}

	first := result.Issues[0]
	if first.File != "a/c.go" {
		t.Errorf("expected cleaned file path a/c.go, got %q", first.File)
	}
	if first.Message != "planted secret (rule security/hardcoded-secret: Hardcoded Secrets)" {
		t.Errorf("expected citation appended to the message, got %q", first.Message)
	}
	if first.Severity != "error" {
		t.Errorf("a populated severity must not be overwritten, got %q", first.Severity)
	}

	second := result.Issues[1]
	if second.RuleID != "security/sql-injection" {
		t.Fatalf("unexpected surviving issue: %+v", second)
	}
	if second.Severity != "error" {
		t.Errorf("expected empty severity filled from the rule, got %q", second.Severity)
	}
	if second.Message != "Potential SQL injection vulnerability detected (rule security/sql-injection: SQL Injection Vulnerability)" {
		t.Errorf("expected empty message filled from the rule before the citation, got %q", second.Message)
	}
}
