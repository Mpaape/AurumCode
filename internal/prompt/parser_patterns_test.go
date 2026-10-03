package prompt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// cachedPatternBuilders may compile inside a function: each caches what it
// compiles, so a pattern is still compiled once per process.
var cachedPatternBuilders = map[string]bool{"languageFencePattern": true}

// TestParserCompilesPatternsOnce: no function of the parser files compiles a
// regular expression per call; patterns live at package level.
func TestParserCompilesPatternsOnce(t *testing.T) {
	files, err := filepath.Glob("parse*.go")
	if err != nil || len(files) < 5 {
		t.Fatalf("parser files not found: %v %v", files, err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || cachedPatternBuilders[fn.Name.Name] {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "regexp" && strings.Contains(sel.Sel.Name, "Compile") {
						t.Errorf("%s: %s compiles a pattern per call; move it to parser_patterns.go", fset.Position(call.Pos()), fn.Name.Name)
					}
				}
				return true
			})
		}
	}
}
