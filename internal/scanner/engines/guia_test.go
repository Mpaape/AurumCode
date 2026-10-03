package engines_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// repoRoot is the repository root seen from this package's directory.
var repoRoot = filepath.Join("..", "..", "..")

// guidePackages maps the package qualifiers the extension guide uses to the
// package directories that define them.
var guidePackages = map[string]string{
	"scanner":      "internal/scanner",
	"exemplo":      "internal/scanner/engines/exemplo",
	"deliberation": "internal/deliberation",
	"tools":        "internal/review/tools",
	"skills":       "internal/context/skills",
	"config":       "internal/config",
	"llm":          "internal/llm",
}

// requiredCitations are the contract names the guide must cite: a guide
// that cites nothing would otherwise pass.
var requiredCitations = []string{
	"scanner.Scanner", "scanner.Report", "scanner.Register", "scanner.Executor",
	"scanner.Engine", "scanner.FailureReason", "deliberation.Tool", "deliberation.Limits",
	"deliberation.Transcript", "deliberation.LimitError", "config.ContextProvider",
	"config.ProviderTimeout", "config.MaxProviderContributionBytes",
}

// citation is a qualified name in the guide: pkg.Name or pkg.Type.Member,
// not preceded by a path separator or an identifier character.
var citation = regexp.MustCompile(`(?:^|[^/\w])(scanner|exemplo|deliberation|tools|skills|config|llm)\.([A-Z]\w*)(?:\.([A-Z]\w*))?`)

// pkgIndex is what one package declares: top-level names and, per type,
// its methods, struct fields and interface methods.
type pkgIndex struct {
	names   map[string]bool
	members map[string]map[string]bool
}

func (p pkgIndex) addMember(typ, member string) {
	if p.members[typ] == nil {
		p.members[typ] = map[string]bool{}
	}
	p.members[typ][member] = true
}

func indexPackage(t *testing.T, dir string) pkgIndex {
	t.Helper()
	idx := pkgIndex{names: map[string]bool{}, members: map[string]map[string]bool{}}
	fset := token.NewFileSet()
	notTest := func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot, dir), notTest, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				indexDecl(idx, decl)
			}
		}
	}
	return idx
}

func indexDecl(idx pkgIndex, decl ast.Decl) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			idx.names[d.Name.Name] = true
			return
		}
		idx.addMember(receiverType(d.Recv.List[0].Type), d.Name.Name)
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				idx.names[s.Name.Name] = true
				indexTypeMembers(idx, s)
			case *ast.ValueSpec:
				for _, n := range s.Names {
					idx.names[n.Name] = true
				}
			}
		}
	}
}

func indexTypeMembers(idx pkgIndex, s *ast.TypeSpec) {
	var fields *ast.FieldList
	switch tt := s.Type.(type) {
	case *ast.StructType:
		fields = tt.Fields
	case *ast.InterfaceType:
		fields = tt.Methods
	default:
		return
	}
	for _, f := range fields.List {
		for _, n := range f.Names {
			idx.addMember(s.Name.Name, n.Name)
		}
	}
}

func receiverType(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return receiverType(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		return receiverType(e.X)
	}
	return ""
}

// TestExtensionGuideCitesTheRealContract: every pkg.Name and pkg.Type.Member
// the guide cites exists in the package it names, and the guide cites the
// contract of each extension point.
func TestExtensionGuideCitesTheRealContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "docs", "extensao.md"))
	if err != nil {
		t.Fatal(err)
	}
	indexes := map[string]pkgIndex{}
	for q, dir := range guidePackages {
		indexes[q] = indexPackage(t, dir)
	}
	cited := map[string]bool{}
	var missing []string
	for _, m := range citation.FindAllStringSubmatch(string(raw), -1) {
		pkg, name, member := m[1], m[2], m[3]
		idx := indexes[pkg]
		cited[pkg+"."+name] = true
		switch {
		case !idx.names[name]:
			missing = append(missing, pkg+"."+name)
		case member != "" && !idx.members[name][member]:
			missing = append(missing, pkg+"."+name+"."+member)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("docs/extensao.md cites names the packages do not declare: %s", strings.Join(missing, ", "))
	}
	for _, want := range requiredCitations {
		if !cited[want] {
			t.Errorf("docs/extensao.md does not cite %s", want)
		}
	}
	if len(cited) < 25 {
		t.Errorf("only %d distinct citations found; the extraction no longer reads the guide", len(cited))
	}
}
