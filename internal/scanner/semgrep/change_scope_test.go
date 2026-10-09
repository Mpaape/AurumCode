package semgrep

import (
	"context"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// changeDiff is what git diff reports for a range that added lines 3-4 of
// app.go and nothing else.
const changeDiff = "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -2,0 +3,2 @@\n+a\n+b\n"

// changeRange is a non-empty reviewed range; the fake runner never reads it.
var changeRange = scanner.Range{Base: "base", Head: "head"}

// scopedRunner answers git diff with changeDiff and semgrep with report.
func scopedRunner(report string) scanner.Command {
	return func(_ context.Context, _, bin string, _ ...string) (string, string, error) {
		if bin == "git" {
			return changeDiff, "", nil
		}
		return report, "", nil
	}
}

func runScoped(t *testing.T, report string) (scanner.Report, error) {
	t.Helper()
	return Engine{}.Run(context.Background(), scanner.Request{Root: t.TempDir(), Range: changeRange, Command: scopedRunner(report)})
}

// A finding the repository already had (an untouched file, or an untouched
// line of a touched file) never judges the change; one on an added line does.
func TestSemgrepKeepsOnlyFindingsOnAddedLines(t *testing.T) {
	report := `{"results":[
{"check_id":"old.rule","path":"legacy.go","start":{"line":3},"extra":{"severity":"ERROR","message":"old"}},
{"check_id":"old.rule","path":"app.go","start":{"line":9},"extra":{"severity":"ERROR","message":"old"}},
{"check_id":"new.rule","path":"./app.go","start":{"line":4},"extra":{"severity":"ERROR","message":"new"}}],"errors":[]}`
	got, err := runScoped(t, report)
	if err != nil || !got.Complete {
		t.Fatalf("Run = %+v, %v", got, err)
	}
	if len(got.Findings) != 1 || got.Findings[0].RuleID != "semgrep:new.rule" || got.Findings[0].Line != 4 {
		t.Fatalf("findings = %+v, want only semgrep:new.rule at app.go:4", got.Findings)
	}
}

// Semgrep writes a partial parse's type as an array; the report must still
// decode, and a recovered parse error in a file the change did not touch
// leaves the scan trustworthy.
func TestSemgrepRecoveredParseErrorOutsideTheChangeIsNotInvalid(t *testing.T) {
	report := `{"results":[],"errors":[
{"code":3,"level":"warn","type":["PartialParsing",[{"path":"run.sh","start":{"line":5},"end":{"line":5}}]],"message":"Syntax error","path":"run.sh","spans":[{"file":"run.sh","start":{"line":5},"end":{"line":5}}]},
{"code":3,"level":"warn","type":"Syntax error","message":"Syntax error at line legacy.sh:1","path":"legacy.sh"},
{"code":3,"level":"warn","type":["PartialParsing",[]],"message":"Syntax error","path":"app.go","spans":[{"start":{"line":9},"end":{"line":9}}]}]}`
	if got, err := runScoped(t, report); err != nil || !got.Complete {
		t.Fatalf("Run = %+v, %v; want a complete, clean scan", got, err)
	}
}

// Fail-closed: a parse error over an added line, a whole-file parse error in
// a touched file, an error Semgrep cannot place in a file and any fatal
// error each make the scan invalid.
func TestSemgrepErrorsOnTheChangeStayInvalid(t *testing.T) {
	cases := map[string]string{
		"span over added line":   `{"results":[],"errors":[{"level":"warn","type":["PartialParsing",[]],"message":"x","path":"app.go","spans":[{"start":{"line":4},"end":{"line":4}}]}]}`,
		"touched file, no spans": `{"results":[],"errors":[{"level":"warn","type":"Syntax error","message":"x","path":"app.go"}]}`,
		"no path":                `{"results":[],"errors":[{"level":"warn","type":"Network","message":"x"}]}`,
		"fatal level":            `{"results":[],"errors":[{"level":"error","type":"RuleParseError","message":"x","path":"legacy.go"}]}`,
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := runScoped(t, report); !errors.Is(err, scanner.ErrInvalidOutput) {
				t.Fatalf("err = %v, want ErrInvalidOutput", err)
			}
		})
	}
}

// Without a reviewed range no finding can be anchored to the change: the
// scan fails, it never reports the whole tree.
func TestSemgrepWithoutRangeFails(t *testing.T) {
	_, err := Engine{}.Run(context.Background(), scanner.Request{Root: t.TempDir(), Command: scopedRunner(`{"results":[]}`)})
	if !errors.Is(err, scanner.ErrNoRange) {
		t.Fatalf("err = %v, want ErrNoRange", err)
	}
}

// A path the review ignores is out of the change's scope: a finding there
// is dropped and a parse error there leaves the scan trustworthy, even on
// a line the range added.
func TestSemgrepIgnoredPathIsOutOfTheChange(t *testing.T) {
	report := `{"results":[
{"check_id":"new.rule","path":"app.go","start":{"line":4},"extra":{"severity":"ERROR","message":"new"}}],"errors":[
{"level":"warn","type":["PartialParsing",[]],"message":"Syntax error","path":"app.go","spans":[{"start":{"line":3},"end":{"line":3}}]}]}`
	ignoreApp := func(path string) bool { return path == "app.go" }
	got, err := Engine{}.Run(context.Background(), scanner.Request{Root: t.TempDir(), Range: changeRange, Command: scopedRunner(report), Ignored: ignoreApp})
	if err != nil || !got.Complete || len(got.Findings) != 0 {
		t.Fatalf("Run = %+v, %v; want a complete scan with the ignored path's finding dropped", got, err)
	}
	// The same report without the ignore keeps failing closed.
	if _, err := runScoped(t, report); !errors.Is(err, scanner.ErrInvalidOutput) {
		t.Fatalf("err = %v, want ErrInvalidOutput without the ignore", err)
	}
}
