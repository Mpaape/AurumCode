package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// maxFunctionLines is AUR-557's ceiling for any function of this package.
const maxFunctionLines = 150

// TestAUR557NoFunctionExceedsLineLimit is AC-004: no function or method of
// cmd/aurumcode (production files) spans more than maxFunctionLines lines,
// so the --base and --pr paths stay explicit phases instead of regrowing
// into one monolith.
func TestAUR557NoFunctionExceedsLineLimit(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	var offenders []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checked++
			lines := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
			if lines > maxFunctionLines {
				offenders = append(offenders, name+": "+fn.Name.Name)
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only %d functions inspected; the glob no longer sees the package", checked)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("functions over %d lines: %s", maxFunctionLines, strings.Join(offenders, ", "))
	}
}

// TestAUR557EntryPointsAreShort pins the two entry points the card exists
// for, so a raised ceiling cannot hide them.
func TestAUR557EntryPointsAreShort(t *testing.T) {
	for file, name := range map[string]string{"review_base.go": "runReview", "review_pr.go": "runPRReview"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name && fn.Recv == nil {
				found = true
				if lines := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1; lines > maxFunctionLines {
					t.Errorf("%s has %d lines", name, lines)
				}
			}
		}
		if !found {
			t.Errorf("%s not found in %s", name, file)
		}
	}
}
