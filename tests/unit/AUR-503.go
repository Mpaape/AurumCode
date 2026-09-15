// TestAUR503 proves the AUR-503 outcome at the package boundary: the four
// command-injection branches AUR-486 added (Go `exec.Command("sh", "-c")`,
// C# `Process.Start` shell forms, PowerShell `Invoke-Expression`/`iex`, and
// bash `eval`) must NOT fire on a textual mention inside a comment (`//`,
// `#`, `/* */`, `--`) or a string literal, and `eval :=`/`eval =` must not
// fire either -- while the REAL invocation of each branch is still found.
//
// The 2026-09-15 measurement reproduced, in the branches AUR-486 added, the
// same false-positive class AUR-486's round 2 fixed only in `sh -c`/SQL:
// `// exec.Command("sh","-c",...)`,
// `// Process.Start("cmd.exe","/c ping "+host)`, `# Invoke-Expression ...`,
// `Write-Host "Invoke-Expression is unsafe"`, `# iex is dangerous` and
// `eval := compute()` all produced a finding. The AUR-462 rule applies: a
// false positive in a merge gate teaches the team to disable the gate.
//
// The fix (internal/review/rules/security.yml) anchors each of those
// branches to a statement context -- start of line after optional
// whitespace, or a `;`/`|`/`&` separator -- instead of a bare token match
// anywhere, and requires bash `eval` to carry a whitespace-separated
// argument that is neither `:` nor `=`. This file proves AC-001 (mention
// in a comment/literal -> no finding), AC-002 (real invocation -> finding),
// AC-003 (`eval :=`/`eval =` -> none, `eval "..."`/`eval $cmd` -> found),
// and that the AUR-462/AUR-481 regression shapes are unchanged.
package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR503(t *testing.T) {
	t.Run("AC001GoExecCommandMentionsAreNotFound", testAUR503AC001GoExecCommandMentionsAreNotFound)
	t.Run("AC001CSharpProcessStartMentionsAreNotFound", testAUR503AC001CSharpProcessStartMentionsAreNotFound)
	t.Run("AC001PowerShellMentionsAreNotFound", testAUR503AC001PowerShellMentionsAreNotFound)
	t.Run("AC001BashEvalMentionsAreNotFound", testAUR503AC001BashEvalMentionsAreNotFound)
	t.Run("AC002GoExecCommandInvocationIsFound", testAUR503AC002GoExecCommandInvocationIsFound)
	t.Run("AC002CSharpProcessStartInvocationIsFound", testAUR503AC002CSharpProcessStartInvocationIsFound)
	t.Run("AC002PowerShellInvocationIsFound", testAUR503AC002PowerShellInvocationIsFound)
	t.Run("AC002BashShellInvocationIsFound", testAUR503AC002BashShellInvocationIsFound)
	t.Run("AC002QualifiedCSharpSpellingIsFound", testAUR503AC002QualifiedCSharpSpellingIsFound)
	t.Run("AC002SeparatedStatementIsFound", testAUR503AC002SeparatedStatementIsFound)
	t.Run("AC002SQLViaShellInterpolationIsFound", testAUR503AC002SQLViaShellInterpolationIsFound)
	t.Run("AC002SafeFormsAreNotFound", testAUR503AC002SafeFormsAreNotFound)
	t.Run("AC003EvalDeclarationsAreNotFound", testAUR503AC003EvalDeclarationsAreNotFound)
	t.Run("AC003EvalInvocationsAreFound", testAUR503AC003EvalInvocationsAreFound)
	t.Run("RegressionExistingCommandInjectionShapes", testAUR503RegressionExistingCommandInjectionShapes)
	t.Run("MessageNeverEchoesSource", testAUR503MessageNeverEchoesSource)
	t.Run("FixtureEndToEnd", testAUR503FixtureEndToEnd)
	t.Run("ScanIsDeterministic", testAUR503ScanIsDeterministic)
}

func aur503Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func aur503Scan(t *testing.T, diff *types.Diff) []types.ReviewIssue {
	t.Helper()
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	return findings
}

func aur503ExpectOne(t *testing.T, path, ruleID, text string) {
	t.Helper()
	f := aur503Scan(t, aur503Diff(path, 1, "+"+text))
	if len(f) != 1 || f[0].RuleID != ruleID {
		t.Fatalf("expected exactly one %s finding for %q, got %+v", ruleID, text, f)
	}
	if f[0].Line != 1 {
		t.Fatalf("expected the finding at line 1 for %q, got %+v", text, f[0])
	}
}

func aur503ExpectNone(t *testing.T, path, text string) {
	t.Helper()
	if f := aur503Scan(t, aur503Diff(path, 1, "+"+text)); len(f) != 0 {
		t.Fatalf("benign line %q must not produce a finding, got %+v", text, f)
	}
}

// --- AC-001: a mention in a comment or string literal is not a defect -----

func testAUR503AC001GoExecCommandMentionsAreNotFound(t *testing.T) {
	for _, line := range []string{
		`	// exec.Command("sh","-c","ping "+host)`,
		`	# exec.Command("sh","-c","ping "+host)`,
		`	/* exec.Command("sh","-c","ping "+host) */`,
		`	-- exec.Command("sh","-c","ping "+host)`,
		`	fmt.Println("exec.Command(\"sh\",\"-c\",cmd) is unsafe")`,
	} {
		aur503ExpectNone(t, "main.go", line)
	}
}

func testAUR503AC001CSharpProcessStartMentionsAreNotFound(t *testing.T) {
	for _, line := range []string{
		`	// Process.Start("cmd.exe", "/c ping " + host)`,
		`	# Process.Start("cmd.exe", "/c ping " + host)`,
		`	/* Process.Start("cmd.exe", "/c ping " + host) */`,
		`	-- Process.Start("cmd.exe", "/c ping " + host)`,
		`	Console.WriteLine("Process.Start is unsafe");`,
	} {
		aur503ExpectNone(t, "App.cs", line)
	}
}

func testAUR503AC001PowerShellMentionsAreNotFound(t *testing.T) {
	for _, line := range []string{
		`	# Invoke-Expression "ping $host"`,
		`	// Invoke-Expression "ping $host"`,
		`	-- Invoke-Expression "ping $host"`,
		`	Write-Host "Invoke-Expression is unsafe"`,
		`	# iex is dangerous`,
		`	Write-Host "iex is a string literal"`,
	} {
		aur503ExpectNone(t, "script.ps1", line)
	}
}

func testAUR503AC001BashEvalMentionsAreNotFound(t *testing.T) {
	for _, line := range []string{
		`	# eval "ping $1" is dangerous`,
		`	// eval "ping $1" is dangerous`,
		`	-- eval "ping $1" is dangerous`,
		`	echo "the literal eval \"ping $1\" must not fire"`,
		`	msg="eval ping \$1 in a variable"`,
	} {
		aur503ExpectNone(t, "deploy.sh", line)
	}
}

// --- AC-002: the real invocation of each branch is still found ------------

func testAUR503AC002GoExecCommandInvocationIsFound(t *testing.T) {
	aur503ExpectOne(t, "main.go", "security/command-injection",
		`	exec.Command("sh", "-c", "ping "+host).Run()`)
	aur503ExpectOne(t, "main.go", "security/command-injection",
		`	exec.Command("bash", "-c", "ping "+host).Run()`)
}

func testAUR503AC002CSharpProcessStartInvocationIsFound(t *testing.T) {
	aur503ExpectOne(t, "App.cs", "security/command-injection",
		`	Process.Start("cmd.exe", "/c ping " + host);`)
	aur503ExpectOne(t, "App.cs", "security/command-injection",
		`	Process.Start("powershell.exe");`)
}

func testAUR503AC002PowerShellInvocationIsFound(t *testing.T) {
	aur503ExpectOne(t, "script.ps1", "security/command-injection",
		`	Invoke-Expression "ping $host"`)
	aur503ExpectOne(t, "script.ps1", "security/command-injection",
		`	iex $payload`)
}

func testAUR503AC002BashShellInvocationIsFound(t *testing.T) {
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	eval "ping $1"`)
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	eval $cmd`)
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	sh -c "ping $1"`)
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	bash -c "ping $1"`)
}

// testAUR503AC002QualifiedCSharpSpellingIsFound proves the statement anchor
// does not lose a dotted qualifier: `System.Diagnostics.Process.Start(...)`
// is a real invocation and must still be found.
func testAUR503AC002QualifiedCSharpSpellingIsFound(t *testing.T) {
	aur503ExpectOne(t, "App.cs", "security/command-injection",
		`	System.Diagnostics.Process.Start("cmd.exe", "/c ping " + host);`)
}

// testAUR503AC002SeparatedStatementIsFound is the other half of the
// statement-context anchor: a real invocation reached after a `;`/`|`
// separator is still an invocation, not a mention.
func testAUR503AC002SeparatedStatementIsFound(t *testing.T) {
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	set -e; eval "$cmd"`)
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	cat script | iex`)
}

func testAUR503AC002SQLViaShellInterpolationIsFound(t *testing.T) {
	aur503ExpectOne(t, "deploy.sh", "security/sql-injection",
		`	query="SELECT * FROM users WHERE name = '$1'"`)
}

func testAUR503AC002SafeFormsAreNotFound(t *testing.T) {
	for _, line := range []string{
		`	exec.Command("ping", host).Run()`,
		`	exec.Command("ls", "-la", dir)`,
		`	Process.Start("ping", host);`,
		`	Process.Start(new ProcessStartInfo("ping") { ArgumentList = { host } });`,
		`  system("ping example.com");`,
	} {
		aur503ExpectNone(t, "src/main.go", line)
	}
}

// --- AC-003: declaration vs invocation -----------------------------------

func testAUR503AC003EvalDeclarationsAreNotFound(t *testing.T) {
	for _, line := range []string{
		`	eval := compute()`,
		`	eval = compute()`,
		`	eval   :=   compute()`,
		`	evaluate(x)`,
	} {
		aur503ExpectNone(t, "main.go", line)
	}
}

func testAUR503AC003EvalInvocationsAreFound(t *testing.T) {
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	eval "ping $1"`)
	aur503ExpectOne(t, "deploy.sh", "security/command-injection", `	eval $cmd`)
}

// --- regression: the shapes earlier cards proved still match -------------

func testAUR503RegressionExistingCommandInjectionShapes(t *testing.T) {
	for _, line := range []string{
		`  exec("ping -c 1 " + host);`,
		`  execSync("ping -c 1 " + host);`,
		`  spawn("ping -c 1 " + host, { shell: true });`,
		`    Command::new("sh").arg("-c").arg("ping ".to_owned() + &h).spawn().unwrap();`,
		`    os.system("rm -rf " + target)`,
		`execve("/bin/sh " + suffix, argv, envp);`,
		`subprocess.run("echo " + msg, shell=True)`,
		`  system("ping " + host);`,
	} {
		aur503ExpectOne(t, "src/legacy.go", "security/command-injection", line)
	}
	// A bare "exec" in a comment was never command injection and still is not.
	aur503ExpectNone(t, "src/app.js", `// exec is dangerous when combined with string concatenation.`)
}

// testAUR503MessageNeverEchoesSource proves the trust-boundary guarantee: a
// finding's message is the trusted catalog description, never the reviewed
// line.
func testAUR503MessageNeverEchoesSource(t *testing.T) {
	secretish := "AURUM-FAKE-ECHO-CANARY-9000-1234"
	f := aur503Scan(t, aur503Diff("deploy.sh", 1, `+	eval "ping `+secretish+`"`))
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

// aur503FixtureLines is the added-file content the integration and
// acceptance programs also use: five real command-injection defects (Go,
// C#, PowerShell x2, bash x2 -- plus the SQL-by-shell shape) and the
// card's mention/deduction lines, which must contribute nothing.
var aur503FixtureLines = map[string][]string{
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
		``,
		`// exec.Command("sh", "-c", "ping "+host) in a comment must not fire`,
	},
	"App.cs": {
		`using System.Diagnostics;`,
		``,
		`class App {`,
		`    static void Ping(string host) {`,
		`        Process.Start("cmd.exe", "/c ping " + host);`,
		`        Process.Start("ping", host);`,
		`        // Process.Start("cmd.exe", "/c ping " + host) must not fire`,
		`    }`,
		`}`,
	},
	"script.ps1": {
		`function Invoke-Ping($host) {`,
		`    Invoke-Expression "ping $host"`,
		`}`,
		`# Invoke-Expression "ping $host" in a comment must not fire`,
		`iex $payload`,
		`Write-Host "iex is a string literal, not a call"`,
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
		`# eval "ping $1" in a comment must not fire`,
		`msg="eval ping \$1 in a variable"`,
		`eval := compute()`,
		`query="SELECT * FROM users WHERE name = '$1'"`,
	},
}

func aur503FixtureDiff() *types.Diff {
	var diff types.Diff
	order := []string{"App.cs", "deploy.sh", "main.go", "script.ps1"}
	for _, path := range order {
		var lines []string
		for _, l := range aur503FixtureLines[path] {
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

func testAUR503FixtureEndToEnd(t *testing.T) {
	f := aur503Scan(t, aur503FixtureDiff())
	want := map[string]map[int]string{
		"main.go":    {6: "security/command-injection"},
		"App.cs":     {5: "security/command-injection"},
		"script.ps1": {2: "security/command-injection", 5: "security/command-injection"},
		"deploy.sh":  {4: "security/command-injection", 7: "security/command-injection", 12: "security/sql-injection"},
	}
	wantCount := 7
	if len(f) != wantCount {
		t.Fatalf("expected exactly %d findings, got %d (a mention fired?): %+v", wantCount, len(f), f)
	}
	for _, issue := range f {
		byLine, ok := want[issue.File]
		if !ok {
			t.Fatalf("unexpected file in findings (false positive?): %+v", issue)
		}
		ruleID, ok := byLine[issue.Line]
		if !ok || ruleID != issue.RuleID {
			t.Fatalf("unexpected finding (false positive?): %+v", issue)
		}
	}
}

func testAUR503ScanIsDeterministic(t *testing.T) {
	first := aur503Scan(t, aur503FixtureDiff())
	second := aur503Scan(t, aur503FixtureDiff())
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
