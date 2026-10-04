package analysis

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// vetReport is go vet -json's real shape (measured on go1.27): a stream of
// one JSON object per package, with absolute positions.
const vetReport = "{}\n{\n\t\"example.com/m/p\": {\n\t\t\"printf\": [\n\t\t\t{\"posn\": \"/sandbox/p/a.go:5:24\", \"end\": \"/sandbox/p/a.go:5:26\", \"message\": \"fmt.Printf format %d has arg \\\"x\\\" of wrong type string\"}\n\t\t]\n\t}\n}\n"

func TestVetReadsJSONReport(t *testing.T) {
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return vetReport, "", nil
	}
	got, err := NewRunner().Vet(context.Background(), "/sandbox", fake)
	if err != nil {
		t.Fatalf("Vet: %v", err)
	}
	want := []Finding{{Path: "p/a.go", Line: 5, Side: SideRight, RuleID: "go-vet/printf", Severity: "warning", Message: `fmt.Printf format %d has arg "x" of wrong type string`}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Vet = %#v, want %#v", got, want)
	}
}

func TestVetCleanReportHasNoFinding(t *testing.T) {
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) { return "{}\n", "", nil }
	got, err := NewRunner().Vet(context.Background(), "/sandbox", fake)
	if err != nil || len(got) != 0 {
		t.Fatalf("Vet = %v, %v; want no finding, no error", got, err)
	}
}

// A non-zero exit is a package go vet did not vet: even with diagnostics
// on stdout, the result is an error and no findings.
func TestVetFailureNeverYieldsFindings(t *testing.T) {
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return vetReport, "# example.com/m/a\nvet: a/a.go:3:23: cannot use \"x\"\n", errors.New("exit status 1")
	}
	got, err := NewRunner().Vet(context.Background(), "/sandbox", fake)
	if !errors.Is(err, ErrVetFailed) || got != nil {
		t.Fatalf("Vet = %v, %v; want nil, ErrVetFailed", got, err)
	}
}

func TestVetKeepsMissingBinary(t *testing.T) {
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return "", "", &exec.Error{Name: "go", Err: exec.ErrNotFound}
	}
	_, err := NewRunner().Vet(context.Background(), "/sandbox", fake)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("err = %v, want exec.ErrNotFound", err)
	}
}

func TestVetRefusesUntrustedReport(t *testing.T) {
	for name, out := range map[string]string{
		"not json":       "p/a.go:1:1: something\n",
		"outside root":   `{"m": {"printf": [{"posn": "/etc/x.go:1:1", "message": "m"}]}}`,
		"no line":        `{"m": {"printf": [{"posn": "/sandbox/a.go", "message": "m"}]}}`,
		"empty analyzer": `{"m": {"": [{"posn": "/sandbox/a.go:1:1", "message": "m"}]}}`,
	} {
		fake := func(ctx context.Context, dir string, args ...string) (string, string, error) { return out, "", nil }
		if _, err := NewRunner().Vet(context.Background(), "/sandbox", fake); !errors.Is(err, scanner.ErrInvalidOutput) {
			t.Errorf("%s: err = %v, want ErrInvalidOutput", name, err)
		}
	}
}

func TestVetInvokesJSONMode(t *testing.T) {
	var gotDir string
	var gotArgs []string
	fake := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		gotDir, gotArgs = dir, append([]string(nil), args...)
		return "", "", nil
	}
	_, _ = NewRunner().Vet(context.Background(), "/work", fake)
	if gotDir != "/work" || !reflect.DeepEqual(gotArgs, []string{"vet", "-json", "./..."}) {
		t.Fatalf("ran %v in %q", gotArgs, gotDir)
	}
}

func TestVetNilRunner(t *testing.T) {
	if _, err := NewRunner().Vet(context.Background(), "/x", nil); err == nil {
		t.Fatal("expected error for nil commandRunner")
	}
}
