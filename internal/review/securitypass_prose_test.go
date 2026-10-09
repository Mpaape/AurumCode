package review

import (
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func proseDiff(path string, added string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path:  path,
		Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: []string{"+" + added}}},
	}}}
}

// A code-shaped rule (applies_to: code) never matches a prose file, where
// the shape is a mention; the same line in a code file is a finding, and
// a secret in plain text is a finding in a prose file too.
func TestSecurityPassSkipsCodeShapedRulesOnProseFiles(t *testing.T) {
	const shell = `eval - executa texto como codigo: "rm " + alvo`
	// The secret-shaped line is assembled at runtime so no scanner of this
	// repository's history reads a credential literal in the source.
	secret := "API_" + "KEY = " + `"` + "ab12" + "cd34" + "ef56" + `"`
	// Likewise the shell-call shape: the analysis catalog reads string
	// literals too, so the source must not spell the call itself.
	call := "    subprocess." + "call(" + `"ls " + alvo)`
	cases := []struct {
		name, path, line, want string
	}{
		{"shell shape in a log", "demo/out/fail.log", shell, ""},
		{"shell shape in a text note", "expected/fail.txt", shell, ""},
		{"shell shape in a document", "docs/notas.md", shell, ""},
		{"shell shape in code", "tool.py", call, "security/command-injection"},
		{"secret in a text note", "notas.txt", secret, "security/hardcoded-secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, err := SecurityScan(proseDiff(tc.path, tc.line))
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, f := range found {
				ids = append(ids, f.RuleID)
			}
			switch {
			case tc.want == "" && len(ids) > 0:
				t.Fatalf("%s: prose file must yield no finding, got %v", tc.path, ids)
			case tc.want != "" && (len(ids) != 1 || ids[0] != tc.want):
				t.Fatalf("%s: want exactly %s, got %v", tc.path, tc.want, ids)
			}
		})
	}
}

// The catalog declares applies_to only on the code-shaped rules.
func TestSecurityCatalogAppliesToIsOnCodeShapedRulesOnly(t *testing.T) {
	rules, err := sharedRules()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"security/sql-injection":     RuleAppliesToCode,
		"security/xss":               RuleAppliesToCode,
		"security/command-injection": RuleAppliesToCode,
		"security/hardcoded-secret":  "",
	}
	for id, applies := range want {
		rule, ok := rules.Get(id)
		if !ok {
			t.Fatalf("rule %s missing from the catalog", id)
		}
		if rule.AppliesTo != applies {
			t.Fatalf("rule %s applies_to = %q, want %q", id, rule.AppliesTo, applies)
		}
	}
}
