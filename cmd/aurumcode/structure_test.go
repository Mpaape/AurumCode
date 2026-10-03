package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/scanner"
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

// productionFiles lists the non-test Go files of this package.
func productionFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	if len(out) < 30 {
		t.Fatalf("only %d production files seen; the glob no longer sees the package", len(out))
	}
	return out
}

// TestAUR558NoProductionFileNamedByCard is AC-001: a production file is named
// by its responsibility, never by the card that created it. Tests may keep the
// card in their name.
func TestAUR558NoProductionFileNamedByCard(t *testing.T) {
	cardNamed := regexp.MustCompile(`^aur[0-9]+\.go$`)
	for _, f := range productionFiles(t) {
		if cardNamed.MatchString(f) {
			t.Errorf("%s is named by a card; name it by its responsibility", f)
		}
	}
}

// gateAssemblyFile is the one production file allowed to import internal/gate.
const gateAssemblyFile = "review_gate.go"

// TestAUR558GateImportedOnlyByAssembly is AC-004's layer guard: cmd/aurumcode
// assembles the gate pipeline in one file; everything else uses that file's
// names, so decision logic cannot leak back into the command.
func TestAUR558GateImportedOnlyByAssembly(t *testing.T) {
	fset := token.NewFileSet()
	assembled := false
	for _, name := range productionFiles(t) {
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) != "github.com/Mpaape/AurumCode/internal/gate" {
				continue
			}
			if name != gateAssemblyFile {
				t.Errorf("%s imports internal/gate; only %s may", name, gateAssemblyFile)
			}
			assembled = true
		}
	}
	if !assembled {
		t.Errorf("%s no longer imports internal/gate; the guard would pass vacuously", gateAssemblyFile)
	}
}

// repoRoot is the repository root, seen from this package's directory.
var repoRoot = filepath.Join("..", "..")

// modulePrefix is the import-path prefix of this repository's packages.
const modulePrefix = "github.com/Mpaape/AurumCode/"

// sourceFile is one parsed production file of the repository, with its path
// and package directory relative to the root ("internal/gate/result.go",
// "internal/gate").
type sourceFile struct {
	path string
	dir  string
	fset *token.FileSet
	file *ast.File
}

// productionTree parses every production Go file under the given top-level
// directories of the repository.
func productionTree(t *testing.T, tops ...string) []sourceFile {
	t.Helper()
	var out []sourceFile
	for _, top := range tops {
		err := filepath.WalkDir(filepath.Join(repoRoot, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			out = append(out, sourceFile{path: rel, dir: path.Dir(rel), fset: fset, file: f})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestNoFunctionExceedsLineLimitAnywhere extends the 150-line ceiling to
// every production function of cmd, internal and pkg.
func TestNoFunctionExceedsLineLimitAnywhere(t *testing.T) {
	files := productionTree(t, "cmd", "internal", "pkg")
	checked := 0
	var offenders []string
	for _, sf := range files {
		for _, decl := range sf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checked++
			lines := sf.fset.Position(fn.End()).Line - sf.fset.Position(fn.Pos()).Line + 1
			if lines > maxFunctionLines {
				offenders = append(offenders, fmt.Sprintf("%s: %s (%d)", sf.path, fn.Name.Name, lines))
			}
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d functions inspected; the walk no longer sees the tree", checked)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("functions over %d lines: %s", maxFunctionLines, strings.Join(offenders, ", "))
	}
}

// TestNoProductionFileNamedByCardAnywhere: no production file of cmd,
// internal or pkg is named by the card that created it.
func TestNoProductionFileNamedByCardAnywhere(t *testing.T) {
	cardNamed := regexp.MustCompile(`^aur[-_]?[0-9]+\.go$`)
	files := productionTree(t, "cmd", "internal", "pkg")
	if len(files) < 150 {
		t.Fatalf("only %d production files seen", len(files))
	}
	for _, sf := range files {
		if cardNamed.MatchString(strings.ToLower(path.Base(sf.path))) {
			t.Errorf("%s is named by a card; name it by its responsibility", sf.path)
		}
	}
}

// layerRule is one row of docs/architecture.md's layer table: production
// files under from must not import to, except one named file or only the
// named symbols of to.
type layerRule struct {
	from, to   string
	exceptFile string
	onlySyms   map[string]bool
	used       bool
}

var (
	layerRow   = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|\\s*`([^`]+)`\\s*\\|(.*)\\|\\s*$")
	backticked = regexp.MustCompile("`([^`]+)`")
)

// readLayerTable reads the "## Layers" table of docs/architecture.md.
func readLayerTable(t *testing.T, doc string) []*layerRule {
	t.Helper()
	start := strings.Index(doc, "\n## Layers\n")
	if start < 0 {
		t.Fatal("docs/architecture.md has no \"## Layers\" section")
	}
	section := doc[start+1:]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}
	var rules []*layerRule
	for _, line := range strings.Split(section, "\n") {
		m := layerRow.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		r := &layerRule{from: m[1], to: m[2]}
		except := strings.TrimSpace(m[3])
		names := backticked.FindAllStringSubmatch(except, -1)
		switch {
		case except == "":
		case strings.HasPrefix(except, "file ") && len(names) == 1:
			r.exceptFile = names[0][1]
		case strings.HasPrefix(except, "only ") && len(names) > 0:
			r.onlySyms = map[string]bool{}
			for _, n := range names {
				r.onlySyms[n[1]] = true
			}
		default:
			t.Fatalf("layer table: unreadable exception %q", except)
		}
		rules = append(rules, r)
	}
	if len(rules) < 6 {
		t.Fatalf("layer table has %d rows; the parser no longer reads it", len(rules))
	}
	return rules
}

// under reports whether dir is pkg or inside it.
func under(dir, pkg string) bool {
	return dir == pkg || strings.HasPrefix(dir, pkg+"/")
}

// importName is the name a file uses for imp.
func importName(imp *ast.ImportSpec, target string) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	return path.Base(target)
}

// symbolsUsed lists the selectors a file reads from the import named name.
func symbolsUsed(f *ast.File, name string) []string {
	var syms []string
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == name {
				syms = append(syms, sel.Sel.Name)
			}
		}
		return true
	})
	return syms
}

// layerViolations checks one file's imports against the rules.
func layerViolations(sf sourceFile, rules []*layerRule) []string {
	var out []string
	for _, imp := range sf.file.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if !strings.HasPrefix(p, modulePrefix) {
			continue
		}
		target := strings.TrimPrefix(p, modulePrefix)
		for _, r := range rules {
			if !under(sf.dir, r.from) || !under(target, r.to) || under(sf.dir, r.to) {
				continue
			}
			switch {
			case r.exceptFile == sf.path:
				r.used = true
			case r.onlySyms != nil:
				r.used = true
				for _, s := range symbolsUsed(sf.file, importName(imp, target)) {
					if !r.onlySyms[s] {
						out = append(out, fmt.Sprintf("%s uses %s.%s (%s may use only the listed symbols of %s)", sf.path, path.Base(target), s, r.from, r.to))
					}
				}
			default:
				out = append(out, fmt.Sprintf("%s imports %s (%s must not import %s)", sf.path, target, r.from, r.to))
			}
		}
	}
	return out
}

// TestImportsFollowLayerTable is the layer guard: docs/architecture.md
// declares which package must not import which; every production import of
// cmd, internal and pkg is checked against it, and an exception that no
// longer matches anything fails too.
func TestImportsFollowLayerTable(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join(repoRoot, "docs", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	rules := readLayerTable(t, string(doc))
	var violations []string
	for _, sf := range productionTree(t, "cmd", "internal", "pkg") {
		violations = append(violations, layerViolations(sf, rules)...)
	}
	for _, r := range rules {
		if (r.exceptFile != "" || r.onlySyms != nil) && !r.used {
			violations = append(violations, fmt.Sprintf("stale exception: %s -> %s matches no import; remove it", r.from, r.to))
		}
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("layer violations:\n%s", strings.Join(violations, "\n"))
	}
}

// gateOnlyExitCodes are the exit codes only the gate's exit ladder decides:
// the named constants of this package and of internal/gate, and the literal
// value of a breach. A review source still returns its own early-exit codes
// (usage 2, an operational failure 1) as literals, so 1 alone is not one.
var gateOnlyExitCodes = map[string]bool{
	"exitFindings": true, "exitQualityNotReviewed": true, "exitArtifactNotWritten": true,
	"gateExitFindings": true, "gateExitBehavioral": true,
}

// returnsInt reports whether fn declares an int among its results.
func returnsInt(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, r := range fn.Type.Results.List {
		if id, ok := r.Type.(*ast.Ident); ok && id.Name == "int" {
			return true
		}
	}
	return false
}

// isGateExitCode reports an expression that names a gate-only exit code:
// one of the constants above, any gate.Exit* selector, or the literal value
// of gate.ExitFindings.
func isGateExitCode(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return gateOnlyExitCodes[x.Name]
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && pkg.Name == "gate" && strings.HasPrefix(x.Sel.Name, "Exit")
	case *ast.BasicLit:
		return x.Kind == token.INT && x.Value == strconv.Itoa(gate.ExitFindings)
	case *ast.ParenExpr:
		return isGateExitCode(x.X)
	}
	return false
}

// TestReviewSourcesReturnNoGateExitCode: a method of a review source that
// returns an int never returns a gate-only exit code, however it is spelled
// (named constant, gate.Exit* selector or literal); the gate's codes come
// only from gate.ExitPolicy.
func TestReviewSourcesReturnNoGateExitCode(t *testing.T) {
	sources := map[string]bool{"baseReview": true, "prReview": true, "reviewState": true}
	inspected := 0
	var ladders []string
	for _, name := range productionFiles(t) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !sources[receiverName(fn)] || !returnsInt(fn) {
				continue
			}
			inspected++
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if _, nested := n.(*ast.FuncLit); nested {
					return false
				}
				if ret, ok := n.(*ast.ReturnStmt); ok {
					for _, r := range ret.Results {
						if isGateExitCode(r) {
							ladders = append(ladders, fmt.Sprintf("%s:%d %s.%s", name, fset.Position(ret.Pos()).Line, receiverName(fn), fn.Name.Name))
						}
					}
				}
				return true
			})
		}
	}
	if inspected < 10 {
		t.Fatalf("only %d source methods returning int inspected; the guard no longer sees them", inspected)
	}
	if len(ladders) > 0 {
		t.Errorf("review sources returning a gate exit code (use gate.ExitPolicy): %v", ladders)
	}
}

// engineNames is every registered scanner engine and category, lower-case.
func engineNames(t *testing.T) map[string]bool {
	t.Helper()
	engines := map[string]bool{}
	for _, name := range append(scanner.Names(), scanner.Categories()...) {
		engines[strings.ToLower(name)] = true
	}
	if !engines["semgrep"] {
		t.Fatal("semgrep must be registered in the binary")
	}
	return engines
}

// engineConstants collects the package-level constants of files whose value
// is a string literal naming an engine.
func engineConstants(files []*ast.File, engines map[string]bool) map[string]bool {
	consts := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, v := range vs.Values {
					if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if s, _ := strconv.Unquote(lit.Value); engines[strings.ToLower(s)] && i < len(vs.Names) {
							consts[vs.Names[i].Name] = true
						}
					}
				}
			}
		}
	}
	return consts
}

// engineBranches lists the places a file branches on an engine name: a
// case, a comparison, a call argument or a map key, spelled as a literal or
// as a constant holding one.
func engineBranches(name string, f *ast.File, engines, consts map[string]bool) []string {
	isEngine := func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.BasicLit:
			s, _ := strconv.Unquote(x.Value)
			return x.Kind == token.STRING && engines[strings.ToLower(s)]
		case *ast.Ident:
			return consts[x.Name]
		}
		return false
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CaseClause:
			for _, e := range x.List {
				if isEngine(e) {
					out = append(out, name+": a case on an engine name")
				}
			}
		case *ast.BinaryExpr:
			if (x.Op == token.EQL || x.Op == token.NEQ) && (isEngine(x.X) || isEngine(x.Y)) {
				out = append(out, name+": a comparison with an engine name")
			}
		case *ast.CallExpr:
			for _, a := range x.Args {
				if isEngine(a) {
					out = append(out, name+": an engine name passed as an argument")
				}
			}
		case *ast.KeyValueExpr:
			if isEngine(x.Key) {
				out = append(out, name+": an engine name as a map key")
			}
		case *ast.IndexExpr:
			if isEngine(x.Index) {
				out = append(out, name+": an engine name as an index")
			}
		}
		return true
	})
	return out
}

// TestNoEngineBranchInGateOrCmdByAnyName: neither cmd/aurumcode nor
// internal/gate branches on an engine name, whether the name is a literal,
// a named constant or the key of a map literal.
func TestNoEngineBranchInGateOrCmdByAnyName(t *testing.T) {
	engines := engineNames(t)
	for _, dir := range []string{".", filepath.Join(repoRoot, "internal", "gate")} {
		names, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		var files []*ast.File
		var paths []string
		for _, n := range names {
			if strings.HasSuffix(n, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), n, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files, paths = append(files, f), append(paths, n)
		}
		if len(files) < 10 {
			t.Fatalf("only %d files in %s", len(files), dir)
		}
		consts := engineConstants(files, engines)
		for i, f := range files {
			for _, b := range engineBranches(paths[i], f, engines, consts) {
				t.Error(b)
			}
		}
	}
}
