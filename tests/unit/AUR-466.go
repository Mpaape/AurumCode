// TestAUR466 proves the AUR-466 outcome at the package boundary: the
// 2026-08-14 measurement on a real, 55-commit Node repository found 15
// security/`error` findings and every sampled one was false --
// security/hardcoded-secret matched a documentation placeholder
// (`export GEMINI_API_KEY=sua-chave`), a help-text string that teaches
// the same export, and two test-fixture assignments whose values are
// synthetic labels (`ttm-smoke-key`, `sk-body-ULTRA-SECRET`); and
// security/sql-injection matched a query built entirely from string
// CONSTANTS, whose real values are bound later through `?` placeholders.
// The shared cause: both rules matched the FORM of the text, never the
// VALUE or the CONTEXT.
//
// This file proves the fix (internal/review/rules/security.yml): none of
// the five measured false-positive shapes are matched (AC-001); a real
// digit-bearing secret, a SQL query concatenating a VARIABLE, and a shell
// command concatenating a VARIABLE are all three still matched (AC-002);
// and the pre-existing Python SQL-injection line and hardcoded-secret
// values this catalog already matched keep matching, unchanged (AC-003).
//
// Selector naming note, the same technique tests/unit/AUR-442.go and
// tests/unit/AUR-462.go document: this proof is TestAUR466, matching the
// card id, and does not collide with any existing Test* function in this
// package.
package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR466(t *testing.T) {
	t.Run("AC001DocPlaceholderIsNotFound", testAUR466AC001DocPlaceholderIsNotFound)
	t.Run("AC001HelpTextExportIsNotFound", testAUR466AC001HelpTextExportIsNotFound)
	t.Run("AC001SmokeFixtureValueIsNotFound", testAUR466AC001SmokeFixtureValueIsNotFound)
	t.Run("AC001AnnouncedFakeValueIsNotFound", testAUR466AC001AnnouncedFakeValueIsNotFound)
	t.Run("AC001ConstantSQLConcatIsNotFound", testAUR466AC001ConstantSQLConcatIsNotFound)
	t.Run("AC002RealSecretIsFound", testAUR466AC002RealSecretIsFound)
	t.Run("AC002SQLVariableConcatIsFound", testAUR466AC002SQLVariableConcatIsFound)
	t.Run("AC002ShellVariableConcatIsFound", testAUR466AC002ShellVariableConcatIsFound)
	t.Run("AC003PythonSQLInjectionUnaffected", testAUR466AC003PythonSQLInjectionUnaffected)
	t.Run("AC003HardcodedSecretValuesUnaffected", testAUR466AC003HardcodedSecretValuesUnaffected)
	t.Run("FixtureEndToEnd", testAUR466FixtureEndToEnd)
	t.Run("ScanIsDeterministic", testAUR466ScanIsDeterministic)
}

// aur466Diff builds a one-file diff whose hunk lines are given verbatim
// (markers included), mirroring tests/unit/AUR-462.go's aur462Diff.
func aur466Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func aur466Scan(t *testing.T, diff *types.Diff) []types.ReviewIssue {
	t.Helper()
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	return findings
}

// --- AC-001: the five measured false-positive shapes -----------------

func testAUR466AC001DocPlaceholderIsNotFound(t *testing.T) {
	diff := aur466Diff("README.md", 1, `+    export GEMINI_API_KEY=sua-chave`)
	if f := aur466Scan(t, diff); len(f) != 0 {
		t.Fatalf("doc placeholder must not match: got %+v", f)
	}
}

func testAUR466AC001HelpTextExportIsNotFound(t *testing.T) {
	diff := aur466Diff("lib/setup-snippets.mjs", 1,
		`+const HELP_TEXT = "Run this first:\n  export GEMINI_API_KEY=sua-chave\n";`)
	if f := aur466Scan(t, diff); len(f) != 0 {
		t.Fatalf("help-text export must not match: got %+v", f)
	}
}

func testAUR466AC001SmokeFixtureValueIsNotFound(t *testing.T) {
	diff := aur466Diff("test/tui-smoke.mjs", 1,
		`+process.env.OPENAI_API_KEY = 'ttm-smoke-key';`)
	if f := aur466Scan(t, diff); len(f) != 0 {
		t.Fatalf("smoke-test fixture value must not match: got %+v", f)
	}
}

// testAUR466AC001AnnouncedFakeValueIsNotFound is the card's own
// cautionary example: `sk-body-ULTRA-SECRET` contains none of
// EXAMPLE/SAMPLE/DUMMY/FAKE/TEST/SMOKE and even carries a real-looking
// `sk-` vendor prefix, so a marker-word denylist would miss it. What
// distinguishes it is the absence of any digit -- this card's chosen
// signal.
func testAUR466AC001AnnouncedFakeValueIsNotFound(t *testing.T) {
	diff := aur466Diff("test/e2e.mjs", 1,
		`+process.env.OPENAI_API_KEY = 'sk-body-ULTRA-SECRET';`)
	if f := aur466Scan(t, diff); len(f) != 0 {
		t.Fatalf("digit-free announced-fake value must not match: got %+v", f)
	}
}

func testAUR466AC001ConstantSQLConcatIsNotFound(t *testing.T) {
	diff := aur466Diff("lib/db.mjs", 1,
		`+  return 'SELECT * FROM daily_rollups' + (where.length ? ' WHERE ' + where.join(' AND ') : '');`)
	if f := aur466Scan(t, diff); len(f) != 0 {
		t.Fatalf("SQL built from constants must not match: got %+v", f)
	}
}

// --- AC-002: the true positives that must survive ----------------------

func testAUR466AC002RealSecretIsFound(t *testing.T) {
	diff := aur466Diff("src/app.js", 1, `+const apiKey = "Xk29LmQ8vTz41BhWn7Yc9";`)
	f := aur466Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected one security/hardcoded-secret finding for a real digit-bearing secret, got %+v", f)
	}
}

func testAUR466AC002SQLVariableConcatIsFound(t *testing.T) {
	diff := aur466Diff("src/app.js", 1, `+  return 'SELECT * FROM users WHERE id = ' + userId;`)
	f := aur466Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected one security/sql-injection finding for SQL concatenating a variable, got %+v", f)
	}
}

func testAUR466AC002ShellVariableConcatIsFound(t *testing.T) {
	diff := aur466Diff("src/app.js", 1, `+  exec("ping -c 1 " + host);`)
	f := aur466Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/command-injection" {
		t.Fatalf("expected one security/command-injection finding for a shell command concatenating a variable, got %+v", f)
	}
}

// --- AC-003: pre-existing regression shapes stay unchanged --------------

func testAUR466AC003PythonSQLInjectionUnaffected(t *testing.T) {
	diff := aur466Diff("src/db.py", 1,
		`+    query = "SELECT id, name FROM users WHERE name = '" + name + "'"`)
	f := aur466Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected the pre-existing Python sql-injection shape to keep matching, got %+v", f)
	}
}

func testAUR466AC003HardcodedSecretValuesUnaffected(t *testing.T) {
	diff := aur466Diff("config/secrets.env", 1,
		`+DB_PASSWORD=AURUM-FAKE-PASSWORD-9000-1111`,
		`+api_key = "AURUM-FAKE-KEY-9000-2222"`,
	)
	f := aur466Scan(t, diff)
	if len(f) != 2 {
		t.Fatalf("expected both pre-existing digit-bearing fixture secrets to keep matching, got %+v", f)
	}
	for _, issue := range f {
		if issue.RuleID != "security/hardcoded-secret" {
			t.Fatalf("expected security/hardcoded-secret, got %+v", issue)
		}
	}
}

// aur466FixtureLines is the exact content of
// tests/fixtures/review/vuln/node-placeholder-vs-secret's second commit,
// src/app.js -- see that directory's history.spec. Embedding it here as a
// synthetic diff keeps this package-boundary proof independent of reading
// the committed bare repository, while still agreeing line-for-line with
// tests/integration/AUR-466.go and tests/e2e/AUR-466.sh, which both read
// the real fixture.
var aur466FixtureLines = []string{
	`+"use strict";`,
	`+`,
	`+// Help text that teaches the user how to export their own key. This is`,
	`+// documentation, not a credential -- the value is the literal placeholder`,
	`+// word the project's own setup docs use.`,
	`+const HELP_TEXT = "Run this first:\n  export GEMINI_API_KEY=sua-chave\n";`,
	`+`,
	`+// Test fixtures below assign literal, synthetic values so tests can`,
	`+// assert on redaction behavior. Nothing here is a real credential.`,
	`+process.env.OPENAI_API_KEY = 'ttm-smoke-key';`,
	`+process.env.OPENAI_API_KEY = 'sk-body-ULTRA-SECRET';`,
	`+`,
	`+// Query built from constants only; the actual values are bound through`,
	`+// '?' placeholders elsewhere via params.push(...), never concatenated in.`,
	`+function rollupQuery(where) {`,
	`+  return 'SELECT * FROM daily_rollups' + (where.length ? ' WHERE ' + where.join(' AND ') : '');`,
	`+}`,
	`+`,
	`+// AC-002: a real secret, a real SQL variable concat, and a real shell`,
	`+// variable concat -- all three must still be found.`,
	`+const apiKey = "Xk29LmQ8vTz41BhWn7Yc9";`,
	`+`,
	`+function userQuery(userId) {`,
	`+  return 'SELECT * FROM users WHERE id = ' + userId;`,
	`+}`,
	`+`,
	`+const { exec } = require("child_process");`,
	`+function pingHost(host) {`,
	`+  exec("ping -c 1 " + host);`,
	`+}`,
}

func testAUR466FixtureEndToEnd(t *testing.T) {
	diff := aur466Diff("src/app.js", 1, aur466FixtureLines...)
	f := aur466Scan(t, diff)
	if len(f) != 3 {
		t.Fatalf("expected exactly 3 findings (lines 21, 24, 29), got %d: %+v", len(f), f)
	}
	byLine := map[int]string{}
	for _, issue := range f {
		byLine[issue.Line] = issue.RuleID
	}
	want := map[int]string{
		21: "security/hardcoded-secret",
		24: "security/sql-injection",
		29: "security/command-injection",
	}
	for line, ruleID := range want {
		if got, ok := byLine[line]; !ok || got != ruleID {
			t.Fatalf("expected %s at line %d, got %+v", ruleID, line, f)
		}
	}
	for line := range byLine {
		if _, ok := want[line]; !ok {
			t.Fatalf("unexpected finding at line %d (false positive), full set: %+v", line, f)
		}
	}
}

func testAUR466ScanIsDeterministic(t *testing.T) {
	diff := aur466Diff("src/app.js", 1, aur466FixtureLines...)
	first := aur466Scan(t, diff)
	second := aur466Scan(t, diff)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic finding count: %d vs %d", len(first), len(second))
	}
	var firstMsgs, secondMsgs []string
	for _, i := range first {
		firstMsgs = append(firstMsgs, i.RuleID)
	}
	for _, i := range second {
		secondMsgs = append(secondMsgs, i.RuleID)
	}
	if strings.Join(firstMsgs, ",") != strings.Join(secondMsgs, ",") {
		t.Fatalf("non-deterministic rule order: %v vs %v", firstMsgs, secondMsgs)
	}
}
