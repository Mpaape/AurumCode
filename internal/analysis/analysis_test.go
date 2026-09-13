package analysis

import (
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// singleHunk builds a one-file, one-hunk diff for table tests.
func singleHunk(path string, oldStart, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path:  path,
		Hunks: []types.DiffHunk{{OldStart: oldStart, NewStart: newStart, Lines: lines}},
	}}}
}

func assertFindings(t *testing.T, got, want []Finding) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAnalyzeTable(t *testing.T) {
	r := NewRunner()
	tests := []struct {
		name string
		diff *types.Diff
		want []Finding
	}{
		{
			name: "hardcoded secret on added line is RIGHT",
			diff: singleHunk("app.go", 1, 1, `+password := "hunter2-super-secret"`),
			want: []Finding{{Path: "app.go", Line: 1, Side: SideRight,
				RuleID: RuleHardcodedSecret, Severity: "error", Message: msgHardcodedSecret}},
		},
		{
			name: "command injection on added line",
			diff: singleHunk("run.py", 1, 1, `+subprocess.run("ls -la " + userInput)`),
			want: []Finding{{Path: "run.py", Line: 1, Side: SideRight,
				RuleID: RuleCommandInjection, Severity: "error", Message: msgCommandInjection}},
		},
		{
			name: "permissive os file perms on added line",
			diff: singleHunk("cfg.go", 1, 1, `+os.OpenFile("x", os.O_CREATE, 0666)`),
			want: []Finding{{Path: "cfg.go", Line: 1, Side: SideRight,
				RuleID: RuleFilePermissions, Severity: "warning", Message: msgFilePermissions}},
		},
		{
			name: "sql string concat on added line",
			diff: singleHunk("db.go", 1, 1, `+q := "SELECT * FROM users WHERE id = '" + id`),
			want: []Finding{{Path: "db.go", Line: 1, Side: SideRight,
				RuleID: RuleSQLInjection, Severity: "error", Message: msgSQLInjection}},
		},
		{
			name: "removed secret is LEFT with old-file line",
			diff: singleHunk("app.go", 5, 1, `-api_key = "abc123def456"`),
			want: []Finding{{Path: "app.go", Line: 5, Side: SideLeft,
				RuleID: RuleHardcodedSecret, Severity: "error", Message: msgHardcodedSecret}},
		},
		{
			name: "context line never matches",
			diff: singleHunk("app.go", 1, 1, ` password := "hunter2-super-secret"`),
			want: nil,
		},
		{
			name: "line numbers advance across context and additions",
			diff: singleHunk("app.go", 10, 1,
				`+// header`,
				`+password = "xxxxxxxx"`,
				` context line`,
				`+token = "yyyyyyyy"`),
			want: []Finding{
				{Path: "app.go", Line: 2, Side: SideRight, RuleID: RuleHardcodedSecret, Severity: "error", Message: msgHardcodedSecret},
				{Path: "app.go", Line: 4, Side: SideRight, RuleID: RuleHardcodedSecret, Severity: "error", Message: msgHardcodedSecret},
			},
		},
		{
			name: "benign added line produces no finding",
			diff: singleHunk("app.go", 1, 1, `+x := fetchUser(42)`),
			want: nil,
		},
		{
			name: "nil diff",
			diff: nil,
			want: nil,
		},
		{
			name: "findings sorted by path then line",
			diff: &types.Diff{Files: []types.DiffFile{
				{Path: "b.go", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: []string{`+token = "zzzzzzzz"`}}}},
				{Path: "a.go", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: []string{`+password = "pppppppp"`}}}},
			}},
			want: []Finding{
				{Path: "a.go", Line: 1, Side: SideRight, RuleID: RuleHardcodedSecret, Severity: "error", Message: msgHardcodedSecret},
				{Path: "b.go", Line: 1, Side: SideRight, RuleID: RuleHardcodedSecret, Severity: "error", Message: msgHardcodedSecret},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertFindings(t, r.Analyze(tc.diff), tc.want)
		})
	}
}

func TestAnalyzeDeterministic(t *testing.T) {
	r := NewRunner()
	diff := singleHunk("app.go", 1, 1,
		`+password = "xxxxxxxx"`,
		`+q := "SELECT * FROM t WHERE id='" + id`)
	first := r.Analyze(diff)
	second := r.Analyze(diff)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic Analyze:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if len(first) != 2 {
		t.Fatalf("want 2 findings, got %d", len(first))
	}
}

func TestNewRunnerZeroConfig(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	if len(r.rules) == 0 {
		t.Fatal("NewRunner has an empty embedded catalog")
	}
	// Applying the embedded patterns must work with no configuration and no
	// external command: a diff with a known defect yields a finding.
	findings := r.Analyze(singleHunk("x.go", 1, 1, `+secret := "s3cret-value"`))
	if len(findings) != 1 || findings[0].RuleID != RuleHardcodedSecret {
		t.Fatalf("expected one hardcoded-secret finding, got %#v", findings)
	}
}

// TestAUR489SecretNaming pins AC-002 with the exact DEVE/NAO DEVE lists the
// card names, each as its own subtest so a regression on any single input
// is reported by name. The DEVE side reproduces the identifiers the
// 2026-09-13 adversarial review found silently unmatched (a camelCase or
// snake_case prefix in front of the keyword); the NAO DEVE side pins the
// two failure modes a naive "accept any prefix" fix would reintroduce: the
// keyword must be at the END of the identifier (not a substring, so
// "tokenizer" and "access_token_expiry" stay clear), and the value must be
// 8+ characters (so a placeholder or an empty literal stays clear).
func TestAUR489SecretNaming(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		// DEVE casar: prefix (camelCase or snake_case) + keyword at the
		// end of the identifier, value 8+ chars.
		{name: "camelCase prefix, password", line: `dbPassword := "hunter2-super-secret"`, want: true},
		{name: "camelCase prefix, token", line: `adminToken = 'ghp_abcdefgh'`, want: true},
		{name: "camelCase prefix, secret, colon", line: `userSecret: "sk-live-abcdefgh"`, want: true},
		{name: "bare keyword, upper snake case", line: `API_KEY="abcdefgh12"`, want: true},
		{name: "bare keyword, camelCase key", line: `apiKey = "abcdefgh12"`, want: true},
		// NAO DEVE casar: keyword is not at the end of the identifier
		// (a longer word merely contains it), or the value is too
		// short.
		{name: "keyword is a prefix of a longer word", line: `tokenizer := "utf8"`, want: false},
		{name: "keyword in the middle of the identifier", line: `access_token_expiry = "1h"`, want: false},
		{name: "keyword is the start, not the end", line: `password_hint = "your pet"`, want: false},
		{name: "empty value", line: `secret = ""`, want: false},
		{name: "value under 8 characters", line: `token = "x"`, want: false},
		// Same failure mode as "tokenizer": an arbitrary suffix after
		// the keyword (not the permitted plural "s") must stay clear.
		{name: "arbitrary suffix, not the plural s", line: `passwordless = "irrelevant-flag"`, want: false},
	}
	r := NewRunner()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			findings := r.Analyze(singleHunk("x.go", 1, 1, "+"+tc.line))
			got := len(findings) > 0
			if got != tc.want {
				t.Fatalf("Analyze(%q) matched=%v, want=%v (findings=%#v)", tc.line, got, tc.want, findings)
			}
		})
	}
}
