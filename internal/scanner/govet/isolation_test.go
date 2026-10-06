package govet_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/scanner/govet"
)

// cgoCalc is a package file that imports "C": with CGO_ENABLED=0 go vet
// drops it from the package without a diagnostic.
const cgoCalc = "package calc\n\n// #include <stdio.h>\nimport \"C\"\n\n// Hello is a cgo function.\nfunc Hello() { C.puts(C.CString(\"x\")) }\n"

// A go.work in a directory above the review root must not decide what go
// vet loads: the child runs with GOWORK=off, so the root's own module is
// vetted and its finding on an added line is kept. Without GOWORK=off the
// parent workspace (which does not list the root) makes go vet fail.
func TestParentWorkspaceDoesNotReachVet(t *testing.T) {
	parent := t.TempDir()
	write(t, parent, map[string]string{
		"go.work":      "go 1.21\n\nuse ./outro\n",
		"outro/go.mod": "module example.com/outro\n\ngo 1.21\n",
		"outro/o.go":   "package outro\n",
	})
	root := filepath.Join(parent, "svc")
	out := scanTreeAt(t, root, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": badCalc}, added: map[string][]int{"calc/calc.go": {7, 8, 9}}})
	if out.Reason != "" || len(out.Findings) != 1 || out.Findings[0].Path != "calc/calc.go" || out.Findings[0].Line != 9 {
		t.Fatalf("outcome = %+v, want only calc/calc.go:9 (a parent go.work reached go vet)", out)
	}
}

// The child go receives GOWORK=off whatever the reviewing process says.
func TestGoChildHasWorkspaceOff(t *testing.T) {
	t.Setenv("GOWORK", filepath.Join(t.TempDir(), "go.work"))
	engine, _ := scanner.Lookup(govet.Name)
	env := scanner.ChildEnvironment(os.LookupEnv, engine.Environment)
	out, _, err := scanner.IsolatedCommand(env)(context.Background(), t.TempDir(), "/usr/bin/env")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "GOWORK=") != 1 || !strings.Contains(out, "GOWORK=off\n") {
		t.Fatalf("child env = %q, want GOWORK=off only", out)
	}
}

// A touched package with a cgo file is inconclusive and the engine's error
// names the file, never a clean report over a file go vet did not read.
func TestCgoFileInTouchedPackageIsInconclusive(t *testing.T) {
	tr := tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": cleanCalc, "calc/hello.go": cgoCalc}, added: map[string][]int{"calc/calc.go": {6}}}
	out := scanTree(t, tr)
	if out.Reason != "lint_execution_error" || out.Findings != nil {
		t.Fatalf("outcome = %+v, want lint_execution_error and no findings", out)
	}
	root := t.TempDir()
	write(t, root, tr.files)
	git := func(context.Context, string, string, ...string) (string, string, error) {
		return tr.cannedDiff(), "", nil
	}
	command := func(ctx context.Context, dir, binary string, args ...string) (string, string, error) {
		if binary == "git" {
			return git(ctx, dir, binary, args...)
		}
		return "go1.27.1\n", "", nil
	}
	r := scanner.Range{Base: strings.Repeat("a", 40), Head: strings.Repeat("b", 40)}
	_, err := govet.Engine{}.Run(context.Background(), scanner.Request{Root: root, Range: r, Command: command})
	if !errors.Is(err, govet.ErrCgoUnvetted) || !strings.Contains(err.Error(), "calc/hello.go") {
		t.Fatalf("error = %v, want ErrCgoUnvetted naming calc/hello.go", err)
	}
}

// A cgo file in a package the range did not touch changes nothing: no
// finding outside the added lines is kept anyway.
func TestCgoFileInUntouchedPackageIsIgnored(t *testing.T) {
	out := scanTree(t, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": badCalc, "nativo/hello.go": strings.Replace(cgoCalc, "package calc", "package nativo", 1)}, added: map[string][]int{"calc/calc.go": {7, 8, 9}}})
	if out.Reason != "" || len(out.Findings) != 1 || out.Findings[0].Line != 9 {
		t.Fatalf("outcome = %+v, want only calc/calc.go:9", out)
	}
}
