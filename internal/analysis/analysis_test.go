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
			diff: singleHunk("app.go", 1, 1, `+password := "hunter2"`),
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
			diff: singleHunk("app.go", 1, 1, ` password := "hunter2"`),
			want: nil,
		},
		{
			name: "line numbers advance across context and additions",
			diff: singleHunk("app.go", 10, 1,
				`+// header`,
				`+password = "x"`,
				` context line`,
				`+token = "y"`),
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
				{Path: "b.go", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: []string{`+token = "z"`}}}},
				{Path: "a.go", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: []string{`+password = "p"`}}}},
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
		`+password = "x"`,
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
	findings := r.Analyze(singleHunk("x.go", 1, 1, `+secret := "s3cret"`))
	if len(findings) != 1 || findings[0].RuleID != RuleHardcodedSecret {
		t.Fatalf("expected one hardcoded-secret finding, got %#v", findings)
	}
}
