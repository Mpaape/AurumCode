package analysis

import (
	"context"
	"testing"
)

// TestNormalizeSemgrepSeverity is B2's table test: Semgrep 1.x's real
// severity vocabulary (CRITICAL/HIGH/MEDIUM/LOW/INFO, plus INVENTORY/
// EXPERIMENT for non-gating rules) must map onto this project's rank
// exactly, never silently drifting to a lower rank for an unrecognized
// spelling.
func TestNormalizeSemgrepSeverity(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"CRITICAL", "error"},
		{"HIGH", "error"},
		{"MEDIUM", "warning"},
		{"LOW", "info"},
		{"INFO", "info"},
		{"ERROR", "error"},
		{"WARNING", "warning"},
		{"INVENTORY", "error"},
		{"EXPERIMENT", "error"},
		{"", "error"},
		{"something-unknown", "error"},
		{"critical", "error"},
		{"  high  ", "error"},
	}
	for _, c := range cases {
		if got := normalizeSemgrepSeverity(c.in); got != c.want {
			t.Errorf("normalizeSemgrepSeverity(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fakeSemgrepRunner returns a commandRunner that always returns the given
// stdout/exitErr, ignoring args -- a minimal fake for exercising Semgrep's
// own report-decoding logic directly, without a real executable.
func fakeSemgrepRunner(stdout string, exitErr error) commandRunner {
	return func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return stdout, "", exitErr
	}
}

type exitError struct{ code int }

func (e exitError) Error() string { return "exit status" }

// TestSemgrepReportedErrorsNeverReadAsClean is B1's direct unit proof: a
// decodable report (results present, even as an empty array) whose own
// "errors" array is non-empty must be refused, regardless of the runner's
// own exit code -- this is the exact gap the independent review found
// reproducible (rc=0 after a rule-pack download failure).
func TestSemgrepReportedErrorsNeverReadAsClean(t *testing.T) {
	cases := []struct {
		name    string
		stdout  string
		exitErr error
	}{
		{
			name:    "NonZeroExitWithErrors",
			stdout:  `{"errors":[{"level":"error","type":"RuleParseError","message":"could not download rule pack"}],"results":[]}`,
			exitErr: exitError{code: 2},
		},
		{
			name:    "ZeroExitWithErrors",
			stdout:  `{"errors":[{"level":"error","type":"RuleParseError","message":"could not download rule pack"}],"results":[]}`,
			exitErr: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewRunner().Semgrep(context.Background(), t.TempDir(), nil, false, fakeSemgrepRunner(c.stdout, c.exitErr))
			if err == nil {
				t.Fatal("Semgrep returned no error for a report with a non-empty \"errors\" array -- a fatal failure read as a clean pass")
			}
		})
	}
}

// TestSemgrepNonZeroExitWithoutErrorFlagIsFailure is B1's second half: this
// command never passes Semgrep's own --error flag, so exit code 1 here is
// NOT "findings were reported" the way some other CI integrations read it
// -- any non-zero exit, even with a decodable, error-free report, is a
// failure.
func TestSemgrepNonZeroExitWithoutErrorFlagIsFailure(t *testing.T) {
	stdout := `{"results":[{"check_id":"demo.rule","path":"app.go","start":{"line":1},"extra":{"severity":"ERROR","message":"demo"}}]}`
	_, err := NewRunner().Semgrep(context.Background(), t.TempDir(), nil, false, fakeSemgrepRunner(stdout, exitError{code: 1}))
	if err == nil {
		t.Fatal("Semgrep returned no error for a non-zero exit (no --error flag passed, so exit 1 is not \"findings reported\")")
	}
}

// TestSemgrepCleanExitZeroWithFindingsSucceeds is the positive contrast:
// exit 0 with a decodable, error-free report succeeds and returns the
// findings.
func TestSemgrepCleanExitZeroWithFindingsSucceeds(t *testing.T) {
	stdout := `{"results":[{"check_id":"demo.rule","path":"app.go","start":{"line":1},"extra":{"severity":"ERROR","message":"demo"}}]}`
	findings, err := NewRunner().Semgrep(context.Background(), t.TempDir(), nil, false, fakeSemgrepRunner(stdout, nil))
	if err != nil {
		t.Fatalf("Semgrep returned an error for a clean exit 0 report: %v", err)
	}
	if len(findings) != 1 || findings[0].RuleID != "semgrep:demo.rule" {
		t.Fatalf("findings = %+v, want one semgrep:demo.rule finding", findings)
	}
}

// TestSemgrepDisableNosemFlag proves the --disable-nosem flag is actually
// passed to the runner when requested (B3).
func TestSemgrepDisableNosemFlag(t *testing.T) {
	var gotArgs []string
	run := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		gotArgs = args
		return `{"results":[]}`, "", nil
	}
	if _, err := NewRunner().Semgrep(context.Background(), t.TempDir(), nil, true, run); err != nil {
		t.Fatalf("Semgrep: %v", err)
	}
	found := false
	for _, a := range gotArgs {
		if a == "--disable-nosem" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --disable-nosem in args, got %v", gotArgs)
	}
}
