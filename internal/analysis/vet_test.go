package analysis

import (
	"context"
	"errors"
	"testing"
)

func TestVetParsesDiagnostics(t *testing.T) {
	r := NewRunner()
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return "", "# example.com/x\n./pkg/a.go:10:2: printf: non-constant format string\n./pkg/b.go:3:5: unreachable code\n", errors.New("exit status 1")
	}
	got, err := r.Vet(context.Background(), "/sandbox", fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Finding{
		{Path: "pkg/a.go", Line: 10, RuleID: RuleGoVet, Severity: "warning", Message: "printf: non-constant format string"},
		{Path: "pkg/b.go", Line: 3, RuleID: RuleGoVet, Severity: "warning", Message: "unreachable code"},
	}
	assertFindings(t, got, want)
}

func TestVetParsesNoColumnFormat(t *testing.T) {
	r := NewRunner()
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return "pkg.go:12: suspicious construct\n", "", nil
	}
	got, err := r.Vet(context.Background(), "/sandbox", fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Finding{{Path: "pkg.go", Line: 12, RuleID: RuleGoVet, Severity: "warning", Message: "suspicious construct"}}
	assertFindings(t, got, want)
}

func TestVetSkipsHeadersAndStrayLines(t *testing.T) {
	r := NewRunner()
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return "", "# github.com/x/y\nexit status 1\n", nil
	}
	got, err := r.Vet(context.Background(), "/sandbox", fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFindings(t, got, nil)
}

func TestVetInvokesGoVetWithDotSlashDot(t *testing.T) {
	var gotDir string
	var gotArgs []string
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		gotDir = dir
		gotArgs = append([]string(nil), args...)
		return "", "", nil
	}
	_, _ = NewRunner().Vet(context.Background(), "/work", fake)
	if gotDir != "/work" {
		t.Errorf("dir = %q, want %q", gotDir, "/work")
	}
	if len(gotArgs) != 2 || gotArgs[0] != "vet" || gotArgs[1] != "./..." {
		t.Errorf("args = %v, want [vet ./...]", gotArgs)
	}
}

func TestVetNilRunner(t *testing.T) {
	if _, err := NewRunner().Vet(context.Background(), "/x", nil); err == nil {
		t.Fatal("expected error for nil commandRunner")
	}
}

func TestVetRunnerErrorWithNoFindingsPropagates(t *testing.T) {
	wantErr := errors.New("sandbox: go binary missing")
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return "", "", wantErr
	}
	_, err := NewRunner().Vet(context.Background(), "/x", fake)
	if err != wantErr {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestVetCombinesStdoutAndStderr(t *testing.T) {
	r := NewRunner()
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return "a.go:1:2: out diag\n", "b.go:2:3: err diag\n", nil
	}
	got, err := r.Vet(context.Background(), "/sandbox", fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Finding{
		{Path: "a.go", Line: 1, RuleID: RuleGoVet, Severity: "warning", Message: "out diag"},
		{Path: "b.go", Line: 2, RuleID: RuleGoVet, Severity: "warning", Message: "err diag"},
	}
	assertFindings(t, got, want)
}
