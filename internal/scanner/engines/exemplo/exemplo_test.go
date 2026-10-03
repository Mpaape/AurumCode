package exemplo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunReportsMarkedLinesDeterministically(t *testing.T) {
	root := t.TempDir()
	write(t, root, "b.txt", "nada\n// "+Marker+"\n")
	write(t, root, "a/x.go", Marker+"\nok\n"+Marker+"\n")
	write(t, root, ".git/HEAD", Marker+"\n")
	rep, err := Scanner{}.Run(context.Background(), scanner.Request{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Complete || rep.Version != Version {
		t.Fatalf("report = %+v", rep)
	}
	want := []struct {
		path string
		line int
	}{{"a/x.go", 1}, {"a/x.go", 3}, {"b.txt", 2}}
	if len(rep.Findings) != len(want) {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	for i, w := range want {
		f := rep.Findings[i]
		if f.Path != w.path || f.Line != w.line || f.RuleID != RuleID || f.Severity != severity {
			t.Fatalf("finding %d = %+v, want %s:%d", i, f, w.path, w.line)
		}
	}
}

func TestRunCleanTreeIsCompleteAndEmpty(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "sem marca\n")
	rep, err := Scanner{}.Run(context.Background(), scanner.Request{Root: root})
	if err != nil || !rep.Complete || len(rep.Findings) != 0 {
		t.Fatalf("report = %+v, err = %v", rep, err)
	}
}

func TestRunUnreadableRootIsAnError(t *testing.T) {
	e := Engine()
	out := scanner.Executor{}.Scan(context.Background(), e, scanner.Request{Root: filepath.Join(t.TempDir(), "ausente")})
	if out.Reason != Name+"_execution_error" || out.Findings != nil {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestEngineTakesNoOption(t *testing.T) {
	e := Engine()
	if e.TypedOrigin() != Name || e.Category != Name {
		t.Fatalf("engine = %+v", e)
	}
	if err := e.ValidateOptions(scanner.Options{"x": 1}); err == nil {
		t.Fatal("an option was accepted")
	}
}
