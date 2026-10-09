package analysis

import (
	"testing"
)

// catalogRule returns the embedded catalog entry with the given id.
func catalogRule(t *testing.T, id string) rule {
	t.Helper()
	for _, rl := range embeddedRules {
		if rl.id == id {
			return rl
		}
	}
	t.Fatalf("rule %s missing from the embedded catalog", id)
	return rule{}
}

// countRule counts the findings of one rule.
func countRule(findings []Finding, id string) int {
	n := 0
	for _, f := range findings {
		if f.RuleID == id {
			n++
		}
	}
	return n
}

// AC-003: a docstring or a string that documents the dangerous call is a
// mention, not command injection. Each body is first shown to match the
// rule's raw pattern, so the empty result is the literal guard's doing.
func TestAUR609DocstringMentionIsNotCommandInjection(t *testing.T) {
	re := catalogRule(t, RuleCommandInjection).re
	cases := []struct{ name, path, body string }{
		{"python one-line docstring", "tool.py", `    """Wrapper seguro; nunca faça subprocess.call("rm " + name)."""`},
		{"javascript message documenting execSync", "run.js", `const tip = "avoid execSync('rm ' + file) in handlers";`},
		{"go message documenting exec", "run.go", `msg := "never exec('sh -c ' + input) here"`},
		{"single-quoted python string documenting system", "tool.py", `HELP = 'do not call os.system("rm " + path)'`},
	}
	r := NewRunner()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !re.MatchString(tc.body) {
				t.Fatalf("the raw pattern does not match %q: the case proves nothing", tc.body)
			}
			if n := countRule(r.Analyze(singleHunk(tc.path, 1, 1, "+"+tc.body)), RuleCommandInjection); n != 0 {
				t.Fatalf("Analyze(%q) reported %d command-injection findings, want 0", tc.body, n)
			}
		})
	}
	t.Run("raw string continuation", func(t *testing.T) {
		body := `subprocess.call("rm " + name)`
		if !matchOutsideLiteral(re, body, false) {
			t.Fatalf("without the entering raw-string state %q must match: the case proves nothing", body)
		}
		diff := singleHunk("doc.go", 1, 1, "+doc := `Never call:", "+"+body, "+`")
		if n := countRule(r.Analyze(diff), RuleCommandInjection); n != 0 {
			t.Fatalf("a raw-string continuation line reported %d command-injection findings, want 0", n)
		}
	})
}

// AC-003: the real call keeps its finding, including when a mention of
// another dangerous call precedes it on the same line.
func TestAUR609RealCallStillCommandInjection(t *testing.T) {
	r := NewRunner()
	for _, tc := range []struct{ name, path, body string }{
		{"python call", "run.py", `subprocess.run("ls " + x)`},
		{"java exec", "Runner.java", `return Runtime.getRuntime().exec("ping -c 1 " + host);`},
		{"mention before the real call", "run.py", `x = "system(" ; subprocess.run("ls " + y)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if n := countRule(r.Analyze(singleHunk(tc.path, 1, 1, "+"+tc.body)), RuleCommandInjection); n != 1 {
				t.Fatalf("Analyze(%q) reported %d command-injection findings, want 1", tc.body, n)
			}
		})
	}
}

// AC-004: a code-shaped rule never runs on a prose file, even on a line that
// is a finding in a code file.
func TestAUR609ProseFilesSkipCodeRules(t *testing.T) {
	r := NewRunner()
	lines := map[string]string{
		RuleCommandInjection: `subprocess.run("ls " + x)`,
		RuleSQLInjection:     `q := "SELECT * FROM users WHERE id = '" + id`,
		RuleFilePermissions:  `os.Chmod("x", 0777)`,
	}
	for id, body := range lines {
		if n := countRule(r.Analyze(singleHunk("main.go", 1, 1, "+"+body)), id); n != 1 {
			t.Fatalf("control: %q in main.go reported %d %s findings, want 1", body, n, id)
		}
		for _, path := range []string{"README.md", "logs/run.log", "notes.TXT"} {
			if got := r.Analyze(singleHunk(path, 1, 1, "+"+body)); len(got) != 0 {
				t.Fatalf("%q in %s reported %#v, want no finding", body, path, got)
			}
		}
	}
}

// AC-004: a secret written in a prose file is still a secret.
func TestAUR609SecretInProseStillFound(t *testing.T) {
	r := NewRunner()
	for _, path := range []string{"README.md", "notes.txt", "logs/run.log"} {
		got := r.Analyze(singleHunk(path, 1, 1, `+password = "abcdefgh12"`))
		if countRule(got, RuleHardcodedSecret) != 1 {
			t.Fatalf("a secret in %s reported %#v, want one %s finding", path, got, RuleHardcodedSecret)
		}
	}
}

// SQL concatenated inside the string is a true positive whose keyword sits
// inside the literal: the literal guard must not reach this rule.
func TestAUR609SQLInsideStringStillFound(t *testing.T) {
	if catalogRule(t, RuleSQLInjection).outsideLiteral {
		t.Fatal("the SQL rule must not carry the literal guard: its keyword is inside the string")
	}
	r := NewRunner()
	for _, body := range []string{
		`q := "SELECT * FROM users WHERE id = '" + id`,
		`cursor.execute("DELETE FROM t WHERE name = '" + name + "'")`,
	} {
		if n := countRule(r.Analyze(singleHunk("db.py", 1, 1, "+"+body)), RuleSQLInjection); n != 1 {
			t.Fatalf("Analyze(%q) reported %d SQL findings, want 1", body, n)
		}
	}
}

// AC-003: a multi-line Python docstring that documents the call is text on
// every line, while a real call after it closes, or a `"""` that only sits
// in a comment or a one-line string, keeps the finding.
func TestAUR609MultilineDocstringIsNotCommandInjection(t *testing.T) {
	const call = `    subprocess.call("rm " + name)`
	if !catalogRule(t, RuleCommandInjection).re.MatchString(call) {
		t.Fatalf("the raw pattern does not match %q: the cases prove nothing", call)
	}
	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		{"three-line docstring", []string{"+def run(name):", `+    """`, "+    Never call" + call[3:] + " directly.", `+    """`}, 0},
		{"call on the closing line", []string{`+    """Wrapper.`, "+" + call + ` is unsafe."""`}, 0},
		{"docstring opened on a context line", []string{` def run(name):`, `     """`, "+" + call, ` """`}, 0},
		{"real call after the docstring closes", []string{`+    """`, "+    Wrapper.", `+    """`, "+" + call}, 1},
		{"hash inside the docstring", []string{`+    """`, "+    see #12:" + call[3:], `+    """`, "+" + call}, 1},
		{"triple quote in a comment", []string{`+x = 1  # """ not a docstring`, "+" + call}, 1},
		{"triple quote in a one-line string", []string{`+DELIM = '"""'`, "+" + call}, 1},
		{"removed opener does not open", []string{`-    """`, "+" + call}, 1},
	}
	r := NewRunner()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := countRule(r.Analyze(singleHunk("tool.py", 1, 1, tc.lines...)), RuleCommandInjection)
			if got != tc.want {
				t.Fatalf("%q reported %d command-injection findings, want %d", tc.lines, got, tc.want)
			}
		})
	}
	t.Run("not python", func(t *testing.T) {
		lines := []string{`+    """`, "+" + call, `+    """`}
		if got := countRule(r.Analyze(singleHunk("run.go", 1, 1, lines...)), RuleCommandInjection); got != 1 {
			t.Fatalf("outside Python a triple quote hid the call: %d findings, want 1", got)
		}
	})
}
