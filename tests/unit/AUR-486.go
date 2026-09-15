// TestAUR486 proves the AUR-486 outcome at the package boundary: command
// injection must be found in the IDIOMATIC spelling of Go, C#, PowerShell
// and bash, and SQL injection built by shell interpolation must be found
// too.
//
// The 2026-08-26 measurement planted the same four defects (secret, sql,
// cmd, xss) in eight languages and found the command-injection cell missed
// in all four of these languages, each for a different concrete reason:
//
//   - Go: `exec.Command("sh", "-c", "ping "+host)` never matched -- the
//     `exec[lv]p?e?` branch requires an `l`/`v` right after `exec`, and the
//     dot in `exec.Command` breaks the sequence anyway.
//   - C#: `Process.Start(...)` was simply not in the pattern.
//   - PowerShell: `Invoke-Expression` was simply not in the pattern.
//   - bash: `eval "ping $1"` and `sh -c "ping $1"` were not in the pattern,
//     and SQL assembled by shell interpolation
//     (`query="... WHERE name = '$1'"`) has no `+` for the SQL rule to
//     anchor on.
//
// This file proves the fix (internal/review/rules/security.yml): each new
// true-positive shape is found (AC-001); each card-named safe form -- the
// Go argv form, C# separated arguments, PowerShell without
// Invoke-Expression, `eval` inside a comment or a string literal, and a
// parametrized query in every language -- is not (AC-002); and the
// pre-existing Rust/Node/Python regression shapes keep matching unchanged
// (AC-003). Findings never echo reviewed source: the message is the trusted
// catalog description.
package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR486(t *testing.T) {
	t.Run("AC001GoShellCommandIsFound", testAUR486AC001GoShellCommandIsFound)
	t.Run("AC001GoBashShellCommandIsFound", testAUR486AC001GoBashShellCommandIsFound)
	t.Run("AC001CSharpProcessStartConcatIsFound", testAUR486AC001CSharpProcessStartConcatIsFound)
	t.Run("AC001CSharpProcessStartShellFileNameIsFound", testAUR486AC001CSharpProcessStartShellFileNameIsFound)
	t.Run("AC001PowerShellInvokeExpressionIsFound", testAUR486AC001PowerShellInvokeExpressionIsFound)
	t.Run("AC001PowerShellIexIsFound", testAUR486AC001PowerShellIexIsFound)
	t.Run("AC001BashEvalIsFound", testAUR486AC001BashEvalIsFound)
	t.Run("AC001BashShCIsFound", testAUR486AC001BashShCIsFound)
	t.Run("AC001BashBashCIsFound", testAUR486AC001BashBashCIsFound)
	t.Run("AC001SQLViaShellInterpolationIsFound", testAUR486AC001SQLViaShellInterpolationIsFound)
	t.Run("AC002GoArgvIsNotFound", testAUR486AC002GoArgvIsNotFound)
	t.Run("AC002GoArgvListIsNotFound", testAUR486AC002GoArgvListIsNotFound)
	t.Run("AC002CSharpSeparateArgsIsNotFound", testAUR486AC002CSharpSeparateArgsIsNotFound)
	t.Run("AC002EvalInCommentIsNotFound", testAUR486AC002EvalInCommentIsNotFound)
	t.Run("AC002EvalInStringLiteralIsNotFound", testAUR486AC002EvalInStringLiteralIsNotFound)
	t.Run("AC002ParametrizedSQLIsNotFound", testAUR486AC002ParametrizedSQLIsNotFound)
	t.Run("AC002CppLiteralCommandIsNotFound", testAUR486AC002CppLiteralCommandIsNotFound)
	t.Run("AC003RustSQLInjectionUnaffected", testAUR486AC003RustSQLInjectionUnaffected)
	t.Run("AC003NodeCommandInjectionUnaffected", testAUR486AC003NodeCommandInjectionUnaffected)
	t.Run("AC003PythonSQLInjectionUnaffected", testAUR486AC003PythonSQLInjectionUnaffected)
	t.Run("AC003CppCommandInjectionUnaffected", testAUR486AC003CppCommandInjectionUnaffected)
	t.Run("MessageNeverEchoesSource", testAUR486MessageNeverEchoesSource)
	t.Run("FixtureEndToEnd", testAUR486FixtureEndToEnd)
	t.Run("ScanIsDeterministic", testAUR486ScanIsDeterministic)
}

func aur486Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func aur486Scan(t *testing.T, diff *types.Diff) []types.ReviewIssue {
	t.Helper()
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	return findings
}

func aur486ExpectOne(t *testing.T, path string, line int, ruleID string, text string) {
	t.Helper()
	diff := aur486Diff(path, 1, "+"+text)
	f := aur486Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != ruleID {
		t.Fatalf("expected exactly one %s finding for %q, got %+v", ruleID, text, f)
	}
	if f[0].Line != 1 {
		t.Fatalf("expected finding at line 1 for %q, got %+v", text, f[0])
	}
	_ = line
}

func aur486ExpectNone(t *testing.T, path string, text string) {
	t.Helper()
	diff := aur486Diff(path, 1, "+"+text)
	if f := aur486Scan(t, diff); len(f) != 0 {
		t.Fatalf("benign line %q must not produce a finding, got %+v", text, f)
	}
}

// --- AC-001: the four languages' idiomatic command-injection spellings ---

func testAUR486AC001GoShellCommandIsFound(t *testing.T) {
	aur486ExpectOne(t, "main.go", 6, "security/command-injection",
		`	exec.Command("sh", "-c", "ping "+host).Run()`)
}

func testAUR486AC001GoBashShellCommandIsFound(t *testing.T) {
	aur486ExpectOne(t, "main.go", 6, "security/command-injection",
		`	exec.Command("bash", "-c", "ping "+host).Run()`)
}

func testAUR486AC001CSharpProcessStartConcatIsFound(t *testing.T) {
	aur486ExpectOne(t, "App.cs", 5, "security/command-injection",
		`	Process.Start("cmd.exe", "/c ping " + host);`)
}

func testAUR486AC001CSharpProcessStartShellFileNameIsFound(t *testing.T) {
	aur486ExpectOne(t, "App.cs", 5, "security/command-injection",
		`	Process.Start("powershell.exe");`)
}

func testAUR486AC001PowerShellInvokeExpressionIsFound(t *testing.T) {
	aur486ExpectOne(t, "script.ps1", 2, "security/command-injection",
		`	Invoke-Expression "ping $host"`)
}

func testAUR486AC001PowerShellIexIsFound(t *testing.T) {
	aur486ExpectOne(t, "script.ps1", 2, "security/command-injection",
		`	iex $payload`)
}

func testAUR486AC001BashEvalIsFound(t *testing.T) {
	aur486ExpectOne(t, "deploy.sh", 4, "security/command-injection",
		`	eval "ping $1"`)
}

func testAUR486AC001BashShCIsFound(t *testing.T) {
	aur486ExpectOne(t, "deploy.sh", 7, "security/command-injection",
		`	sh -c "ping $1"`)
}

func testAUR486AC001BashBashCIsFound(t *testing.T) {
	aur486ExpectOne(t, "deploy.sh", 7, "security/command-injection",
		`	bash -c "ping $1"`)
}

func testAUR486AC001SQLViaShellInterpolationIsFound(t *testing.T) {
	aur486ExpectOne(t, "deploy.sh", 1, "security/sql-injection",
		`	query="SELECT * FROM users WHERE name = '$1'"`)
}

// --- AC-002: the card-named safe forms must NOT produce a finding --------

func testAUR486AC002GoArgvIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "main.go", `	exec.Command("ping", host).Run()`)
}

func testAUR486AC002GoArgvListIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "main.go", `	exec.Command("ls", "-la", dir)`)
}

func testAUR486AC002CSharpSeparateArgsIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "App.cs", `	Process.Start("ping", host);`)
	aur486ExpectNone(t, "App.cs", `	Process.Start(new ProcessStartInfo("ping") { ArgumentList = { host } });`)
}

func testAUR486AC002EvalInCommentIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "deploy.sh", `	# eval "ping $1" is dangerous`)
}

func testAUR486AC002EvalInStringLiteralIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "deploy.sh", `	echo "the literal eval \"ping $1\" must not fire"`)
	aur486ExpectNone(t, "deploy.sh", `	msg="eval ping \$1 in a variable"`)
}

func testAUR486AC002ParametrizedSQLIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "src/main.rs", `	conn.query("SELECT * FROM t WHERE n = $1", &[&name]);`)
	aur486ExpectNone(t, "src/db.py", `	db.execute("SELECT * FROM t WHERE n = ?", (name,))`)
	aur486ExpectNone(t, "main.go", `	db.Query("SELECT * FROM t WHERE n = $1", name)`)
}

// testAUR486AC002CppLiteralCommandIsNotFound is the C++ benign half:
// `system("ping example.com")` names no concatenation, so the pre-existing
// `system(`-with-quote-then-`+` branch must not fire on it.
func testAUR486AC002CppLiteralCommandIsNotFound(t *testing.T) {
	aur486ExpectNone(t, "src/ping.cpp", `  system("ping example.com");`)
}

// --- AC-003: pre-existing regression shapes stay unchanged ---------------

func testAUR486AC003RustSQLInjectionUnaffected(t *testing.T) {
	aur486ExpectOne(t, "src/main.rs", 1, "security/sql-injection",
		`	conn.query(&("SELECT * FROM t WHERE n = '".to_owned() + name));`)
}

func testAUR486AC003NodeCommandInjectionUnaffected(t *testing.T) {
	aur486ExpectOne(t, "src/app.js", 1, "security/command-injection",
		`  exec("ping -c 1 " + host);`)
}

func testAUR486AC003PythonSQLInjectionUnaffected(t *testing.T) {
	aur486ExpectOne(t, "src/db.py", 1, "security/sql-injection",
		`    query = "SELECT id, name FROM users WHERE name = '" + name + "'"`)
}

// testAUR486AC003CppCommandInjectionUnaffected is AC-003's C++ half: the
// pre-existing `system(`-with-quote-then-`+` branch (AUR-442/AUR-461) must
// still find the idiomatic C++ command-injection shape, unchanged.
func testAUR486AC003CppCommandInjectionUnaffected(t *testing.T) {
	aur486ExpectOne(t, "src/ping.cpp", 1, "security/command-injection",
		`  system("ping " + host);`)
}

// testAUR486MessageNeverEchoesSource proves the card's trust-boundary
// guarantee: a finding's message is the trusted catalog description, never
// any part of the reviewed line.
func testAUR486MessageNeverEchoesSource(t *testing.T) {
	secretish := "AURUM-FAKE-ECHO-CANARY-9000-1234"
	diff := aur486Diff("deploy.sh", 1, `+	eval "ping `+secretish+`"`)
	f := aur486Scan(t, diff)
	if len(f) != 1 {
		t.Fatalf("expected one command-injection finding, got %+v", f)
	}
	if strings.Contains(f[0].Message, secretish) {
		t.Fatalf("finding message must never echo reviewed source, got %q", f[0].Message)
	}
	if !strings.HasPrefix(f[0].Message, "Potential command injection vulnerability") {
		t.Fatalf("expected the trusted catalog description, got %q", f[0].Message)
	}
}

// aur486FixtureLines is the exact added-file content of the AUR-486 fixture
// (tests/integration/AUR-486.go and tests/e2e/AUR-486.sh generate the same
// repository at runtime, since a committed fixture directory is not in this
// card's paths). Line numbers are asserted against this slice.
var aur486FixtureLines = map[string][]string{
	"main.go": {
		`package main`,
		``,
		`import "os/exec"`,
		``,
		`func pingUnsafe(host string) {`,
		`	exec.Command("sh", "-c", "ping "+host).Run()`,
		`}`,
		``,
		`func pingSafe(host string) {`,
		`	exec.Command("ping", host).Run()`,
		`}`,
	},
	"App.cs": {
		`using System.Diagnostics;`,
		``,
		`class App {`,
		`    static void Ping(string host) {`,
		`        Process.Start("cmd.exe", "/c ping " + host);`,
		`        Process.Start("ping", host);`,
		`    }`,
		`}`,
	},
	"script.ps1": {
		`function Invoke-Ping($host) {`,
		`    Invoke-Expression "ping $host"`,
		`}`,
		`Get-Process`,
	},
	"deploy.sh": {
		`#!/bin/bash`,
		`set -euo pipefail`,
		`ping_unsafe() {`,
		`    eval "ping $1"`,
		`}`,
		`ping_shell_unsafe() {`,
		`    sh -c "ping $1"`,
		`}`,
		`ping_safe() {`,
		`    ping "$1"`,
		`}`,
		`# eval "ping $1" in a comment must not fire`,
		`echo "the literal eval \"ping $1\" must not fire"`,
		`query="SELECT * FROM users WHERE name = '$1'"`,
	},
}

func aur486FixtureDiff() *types.Diff {
	var diff types.Diff
	// Deterministic file order.
	order := []string{"App.cs", "deploy.sh", "main.go", "script.ps1"}
	for _, path := range order {
		var lines []string
		for _, l := range aur486FixtureLines[path] {
			lines = append(lines, "+"+l)
		}
		diff.Files = append(diff.Files, types.DiffFile{
			Path: path,
			Hunks: []types.DiffHunk{{
				OldStart: 1,
				NewStart: 1,
				Lines:    lines,
			}},
		})
	}
	return &diff
}

func testAUR486FixtureEndToEnd(t *testing.T) {
	f := aur486Scan(t, aur486FixtureDiff())
	// Exactly the six planted defects, no false positive.
	want := map[string]map[int]string{
		"main.go":    {6: "security/command-injection"},
		"App.cs":     {5: "security/command-injection"},
		"script.ps1": {2: "security/command-injection"},
		"deploy.sh":  {4: "security/command-injection", 7: "security/command-injection", 14: "security/sql-injection"},
	}
	if len(f) != 6 {
		t.Fatalf("expected exactly 6 findings, got %d: %+v", len(f), f)
	}
	for _, issue := range f {
		byLine, ok := want[issue.File]
		if !ok {
			t.Fatalf("unexpected file in findings: %+v", issue)
		}
		ruleID, ok := byLine[issue.Line]
		if !ok || ruleID != issue.RuleID {
			t.Fatalf("unexpected finding (false positive?): %+v", issue)
		}
	}
}

func testAUR486ScanIsDeterministic(t *testing.T) {
	first := aur486Scan(t, aur486FixtureDiff())
	second := aur486Scan(t, aur486FixtureDiff())
	if len(first) != len(second) {
		t.Fatalf("non-deterministic finding count: %d vs %d", len(first), len(second))
	}
	var a, b []string
	for _, i := range first {
		a = append(a, i.File+":"+i.RuleID)
	}
	for _, i := range second {
		b = append(b, i.File+":"+i.RuleID)
	}
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("non-deterministic finding order: %v vs %v", a, b)
	}
}
