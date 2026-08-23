// TestAUR462 proves the AUR-462 outcome at the package boundary: the
// 2026-08-14 measurement found the security pass caught a Node repository's
// planted secret and SQL injection but missed its two most common defects --
// `exec("ping -c 1 " + host)` (child_process.exec with concatenation) and
// `innerHTML = userInput` (a direct, unescaped write) -- because
// security/command-injection's pattern required an `l`/`v` right after
// `exec` (matching only C/Python's execl/execv/execve) and security/xss
// carried no pattern at all. This file proves both are now matched, that
// the three named false-positive shapes (AC-002: an argv-form exec call, an
// innerHTML literal, and a comment mentioning "exec") are never matched, and
// that the Python and command-injection shapes the catalog already matched
// before this card still match identically (AC-003).
//
// Selector naming note, the same technique tests/unit/AUR-442.go and
// tests/unit/AUR-445.go document: this proof is TestAUR462, matching the
// card id, and does not collide with any existing Test* function in this
// package.
package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR462(t *testing.T) {
	t.Run("CommandInjectionCarriesTheRestoredMatcher", testAUR462CommandInjectionCarriesTheRestoredMatcher)
	t.Run("XSSNowCarriesAMatcher", testAUR462XSSNowCarriesAMatcher)
	t.Run("NodeExecConcatIsFound", testAUR462NodeExecConcatIsFound)
	t.Run("NodeExecSyncConcatIsFound", testAUR462NodeExecSyncConcatIsFound)
	t.Run("NodeSpawnShellTrueIsFound", testAUR462NodeSpawnShellTrueIsFound)
	t.Run("NodeInnerHTMLDirectWriteIsFound", testAUR462NodeInnerHTMLDirectWriteIsFound)
	t.Run("ACNodeFixtureEndToEnd", testAUR462ACNodeFixtureEndToEnd)
	t.Run("AC002ArgvFormExecIsNotFound", testAUR462AC002ArgvFormExecIsNotFound)
	t.Run("AC002InnerHTMLLiteralIsNotFound", testAUR462AC002InnerHTMLLiteralIsNotFound)
	t.Run("AC002CommentMentioningExecIsNotFound", testAUR462AC002CommentMentioningExecIsNotFound)
	t.Run("AC002SpawnWithoutShellTrueIsNotFound", testAUR462AC002SpawnWithoutShellTrueIsNotFound)
	t.Run("AC002OuterHTMLTemplateLiteralIsNotFound", testAUR462AC002OuterHTMLTemplateLiteralIsNotFound)
	t.Run("AC003PythonSQLInjectionUnaffected", testAUR462AC003PythonSQLInjectionUnaffected)
	t.Run("AC003ExistingCommandInjectionShapesUnaffected", testAUR462AC003ExistingCommandInjectionShapesUnaffected)
	t.Run("AC003HardcodedSecretUnaffected", testAUR462AC003HardcodedSecretUnaffected)
	t.Run("FindingsCiteOnlySecurityCategoryRules", testAUR462FindingsCiteOnlySecurityCategoryRules)
	t.Run("ScanIsDeterministic", testAUR462ScanIsDeterministic)
}

// aur462Diff builds a one-file diff whose hunk lines are given verbatim
// (markers included), mirroring tests/unit/AUR-435.go's aur435Diff and
// tests/unit/AUR-442.go's aur442Diff.
func aur462Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func testAUR462CommandInjectionCarriesTheRestoredMatcher(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	re, ok := loader.PatternFor("security/command-injection")
	if !ok || re == nil {
		t.Fatal("security/command-injection must carry a compiled matcher pattern")
	}
}

func testAUR462XSSNowCarriesAMatcher(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	re, ok := loader.PatternFor("security/xss")
	if !ok || re == nil {
		t.Fatal("security/xss must now carry a compiled matcher pattern (AUR-462's priority restoration)")
	}
	rule, ok := loader.Get("security/xss")
	if !ok {
		t.Fatal("security/xss must resolve in the catalog")
	}
	if rule.Category != "security" {
		t.Fatalf("security/xss must stay category security, got %q", rule.Category)
	}
}

func testAUR462NodeExecConcatIsFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  exec("ping -c 1 " + host);`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "security/command-injection" {
		t.Fatalf("expected one security/command-injection finding for child_process.exec concatenation, got %+v", findings)
	}
}

func testAUR462NodeExecSyncConcatIsFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  execSync("ping -c 1 " + host);`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "security/command-injection" {
		t.Fatalf("expected one security/command-injection finding for execSync concatenation, got %+v", findings)
	}
}

func testAUR462NodeSpawnShellTrueIsFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  spawn("ping -c 1 " + host, { shell: true });`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "security/command-injection" {
		t.Fatalf("expected one security/command-injection finding for spawn(..., {shell: true}), got %+v", findings)
	}
}

func testAUR462NodeInnerHTMLDirectWriteIsFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  document.getElementById("out").innerHTML = userInput;`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "security/xss" {
		t.Fatalf("expected one security/xss finding for a direct innerHTML write, got %+v", findings)
	}
}

// aur462NodeFixtureLines is the exact 27-line Node source this card's
// integration and e2e programs also commit into an ephemeral git
// repository (see tests/integration/AUR-462.go and tests/e2e/AUR-462.sh):
// four planted defects (exec concat, execSync concat, spawn shell:true,
// innerHTML direct write) and four deliberately benign neighbors (an argv
// exec call, a shell-less spawn, a comment mentioning "exec", and an
// innerHTML literal). Keeping the same content and line numbers at all
// three test boundaries (unit/integration/e2e) means the same "exactly
// these four lines, no others" assertion is provable at every layer.
var aur462NodeFixtureLines = []string{
	`"use strict";`,
	`const { exec, execSync, spawn } = require("child_process");`,
	``,
	`function pingConcat(host) {`,
	`  exec("ping -c 1 " + host);`,
	`}`,
	``,
	`function pingSyncConcat(host) {`,
	`  execSync("ping -c 1 " + host);`,
	`}`,
	``,
	`function pingArgv(host) {`,
	`  exec(["ping", host]);`,
	`}`,
	``,
	`function pingShell(host) {`,
	`  spawn("ping -c 1 " + host, { shell: true });`,
	`}`,
	``,
	`// exec is dangerous when combined with string concatenation.`,
	`function renderUser(userInput) {`,
	`  document.getElementById("out").innerHTML = userInput;`,
	`}`,
	``,
	`function renderStatic() {`,
	`  document.getElementById("out").innerHTML = "<b>Static content</b>";`,
	`}`,
}

func aur462NodeFixtureAddedLines() []string {
	added := make([]string, len(aur462NodeFixtureLines))
	for i, l := range aur462NodeFixtureLines {
		added[i] = "+" + l
	}
	return added
}

// testAUR462ACNodeFixtureEndToEnd is the card's central AC-001+AC-002 proof
// in one shot: over the full 27-line Node fixture (a brand-new file, every
// line added), exactly four findings are produced, at exactly the four
// planted lines, each citing the rule the outcome names -- and nowhere
// else, which is the AC-002 guarantee that the four deliberately benign
// neighbor lines produce nothing.
func testAUR462ACNodeFixtureEndToEnd(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, aur462NodeFixtureAddedLines()...)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	type want struct {
		line int
		rule string
	}
	wants := []want{
		{5, "security/command-injection"},
		{9, "security/command-injection"},
		{17, "security/command-injection"},
		{22, "security/xss"},
	}
	if len(findings) != len(wants) {
		t.Fatalf("expected exactly %d findings, got %d: %+v", len(wants), len(findings), findings)
	}
	for i, w := range wants {
		f := findings[i]
		if f.File != "src/app.js" || f.Line != w.line {
			t.Fatalf("finding %d: expected src/app.js:%d, got %s:%d", i, w.line, f.File, f.Line)
		}
		if f.RuleID != w.rule {
			t.Fatalf("finding %d: expected rule %s at line %d, got %q", i, w.rule, w.line, f.RuleID)
		}
	}
}

func testAUR462AC002ArgvFormExecIsNotFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  exec(["ping", host]);`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("argv-form exec (no shell, no concatenation) must not be found, got %+v", findings)
	}
}

func testAUR462AC002InnerHTMLLiteralIsNotFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  document.getElementById("out").innerHTML = "<b>Static content</b>";`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("innerHTML receiving a literal constant must not be found, got %+v", findings)
	}
}

func testAUR462AC002CommentMentioningExecIsNotFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+// exec is dangerous when combined with string concatenation.`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a comment merely mentioning \"exec\" must not be found, got %+v", findings)
	}
}

func testAUR462AC002SpawnWithoutShellTrueIsNotFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, `+  spawn("ping", ["-c", "1", host]);`)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("spawn without shell:true (argv form, no shell) must not be found, got %+v", findings)
	}
}

func testAUR462AC002OuterHTMLTemplateLiteralIsNotFound(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, "+  el.outerHTML = `<b>Static content</b>`;")
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("outerHTML receiving a template-literal constant (no interpolation) must not be found, got %+v", findings)
	}
}

// testAUR462AC003PythonSQLInjectionUnaffected is AC-003's Python half: the
// exact planted line tests/unit/AUR-435.go and tests/fixtures/review/vuln
// already prove security/sql-injection matches must still match, unchanged.
func testAUR462AC003PythonSQLInjectionUnaffected(t *testing.T) {
	const sqlLine = `    query = "SELECT id, name FROM users WHERE name = '" + name + "'"`
	diff := aur462Diff("src/db.py", 1, "+"+sqlLine, "+    return db.execute(query)")
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding (the benign db.execute line must not match), got %d: %+v", len(findings), findings)
	}
	if findings[0].Line != 1 || findings[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected security/sql-injection at line 1, got %+v", findings[0])
	}
}

// testAUR462AC003ExistingCommandInjectionShapesUnaffected is AC-003's other
// half: the C/Python-style shapes command-injection already matched before
// this card (os.system, execve, subprocess.run, all with concatenation)
// still match identically, and the already-benign db.execute call and a
// bare exec.Command (Go's real os/exec idiom, never matched before this
// card either -- it has no `l`/`v` after `exec` and no `(` right after
// `exec`) still do not.
func testAUR462AC003ExistingCommandInjectionShapesUnaffected(t *testing.T) {
	matching := []string{
		`    os.system("rm -rf " + target)`,
		`execve("/bin/sh " + suffix, argv, envp);`,
		`subprocess.run("echo " + msg, shell=True)`,
	}
	for _, line := range matching {
		diff := aur462Diff("src/legacy.py", 1, "+"+line)
		findings, err := review.SecurityScan(diff)
		if err != nil {
			t.Fatalf("SecurityScan(%q): %v", line, err)
		}
		if len(findings) != 1 || findings[0].RuleID != "security/command-injection" {
			t.Fatalf("expected security/command-injection for pre-existing shape %q, got %+v", line, findings)
		}
	}
	benign := []string{
		`    return db.execute(query)`,
		`    cmd := exec.Command("ls", "-la", dir)`,
	}
	for _, line := range benign {
		diff := aur462Diff("src/legacy.go", 1, "+"+line)
		findings, err := review.SecurityScan(diff)
		if err != nil {
			t.Fatalf("SecurityScan(%q): %v", line, err)
		}
		if len(findings) != 0 {
			t.Fatalf("benign line %q must still not be found, got %+v", line, findings)
		}
	}
}

// testAUR462AC003HardcodedSecretUnaffected proves this card's changes
// (command-injection, xss) left the sibling security/hardcoded-secret
// matcher AUR-442 restored completely untouched.
func testAUR462AC003HardcodedSecretUnaffected(t *testing.T) {
	diff := aur462Diff("config/secrets.env", 1, "+DEMO_API_TOKEN=AURUM-FAKE-TOKEN-0000-0001")
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected security/hardcoded-secret unchanged, got %+v", findings)
	}
}

func testAUR462FindingsCiteOnlySecurityCategoryRules(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	diff := aur462Diff("src/app.js", 1, aur462NodeFixtureAddedLines()...)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding for the Node fixture")
	}
	for _, f := range findings {
		rule, ok := loader.Get(f.RuleID)
		if !ok {
			t.Fatalf("finding cites unknown rule %q", f.RuleID)
		}
		if rule.Category != "security" {
			t.Fatalf("the security pass cited a %s-category rule (%s): the rubric separation is broken", rule.Category, rule.ID)
		}
		if !strings.Contains(f.Message, "(rule "+f.RuleID) {
			t.Fatalf("finding must cite its sustaining rule (AUR-434 gate), got %q", f.Message)
		}
	}
}

func testAUR462ScanIsDeterministic(t *testing.T) {
	diff := aur462Diff("src/app.js", 1, aur462NodeFixtureAddedLines()...)
	first, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	second, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("SecurityScan is not deterministic in count: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("SecurityScan is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
		}
	}
}
