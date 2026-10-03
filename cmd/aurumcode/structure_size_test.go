package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// maxProductionFileLines is the ceiling for one production file of this
// package: past it a file is holding more than one responsibility.
const maxProductionFileLines = 400

// cardReference matches a board card number in prose.
var cardReference = regexp.MustCompile(`AUR-[0-9]+`)

// TestProductionFilesStayShort is AC-001: no production file of cmd/aurumcode
// passes maxProductionFileLines lines.
func TestProductionFilesStayShort(t *testing.T) {
	for _, f := range productionFiles(t) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(data), "\n"); n > maxProductionFileLines {
			t.Errorf("%s has %d lines (max %d); split it by responsibility", f, n, maxProductionFileLines)
		}
	}
}

// TestPackageDocNamesNoCard is AC-001: the comment attached to a production
// file's package clause explains a concept, never a card's history.
func TestPackageDocNamesNoCard(t *testing.T) {
	for _, f := range productionFiles(t) {
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ParseComments|parser.PackageClauseOnly)
		if err != nil {
			t.Fatal(err)
		}
		if file.Doc == nil {
			continue
		}
		if ref := cardReference.FindString(file.Doc.Text()); ref != "" {
			t.Errorf("%s: package doc cites %s; describe the concept instead", f, ref)
		}
	}
}

// languageLiteral matches a Portuguese review-language tag written as a Go
// string: the selection belongs to internal/i18n.
var languageLiteral = regexp.MustCompile(`(?i)^pt(-[a-z]+)?$`)

// TestNoLanguageBranchOutsideCatalog is AC-002/MUT-001: no production file of
// cmd/aurumcode names a Portuguese language tag. Interface text per language
// lives in the internal/i18n catalog, so a branch on the language with its
// own literals cannot come back.
func TestNoLanguageBranchOutsideCatalog(t *testing.T) {
	for _, f := range productionFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if v, err := strconv.Unquote(lit.Value); err == nil && languageLiteral.MatchString(strings.TrimSpace(v)) {
				t.Errorf("%s: language tag %s outside the internal/i18n catalog", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
	}
}

// boolParamPackages are the trees whose exported functions this card's guard
// covers. internal/gate (ReuseOrStoreGateVerdict) is outside them: its
// signature belongs to the gate's own card.
var boolParamPackages = []string{".", "../../internal/i18n", "../../internal/prompt", "../../internal/render", "../../internal/reviewprofile", "../../internal/context"}

// TestNoBoolParameterInExportedFunction: an exported function or method of the
// covered packages takes no bool parameter; a choice travels as a named type
// or a field of an options struct, so a call site says what it chose.
func TestNoBoolParameterInExportedFunction(t *testing.T) {
	seen := 0
	for _, root := range boolParamPackages {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			seen++
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || !fn.Name.IsExported() {
					continue
				}
				for _, field := range fn.Type.Params.List {
					if id, ok := field.Type.(*ast.Ident); ok && id.Name == "bool" {
						t.Errorf("%s: exported %s takes a bool parameter", fset.Position(fn.Pos()), fn.Name.Name)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen < 50 {
		t.Fatalf("only %d files seen; the walk no longer sees the packages", seen)
	}
}
