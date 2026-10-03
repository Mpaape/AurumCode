package prompt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// goSourceRoots are every Go source tree of the module.
var goSourceRoots = []string{"../../cmd", "..", "../../pkg"}

// humanReportWriters render Markdown reports for people (PR comment,
// summary, changelog); their headings are output, never prompt sections.
var humanReportWriters = map[string]bool{
	"../../cmd/aurumcode/pr.go": true,
	"../render/summary.go":      true,
	"../changelog/render.go":    true,
}

// sectionTitleLine matches a level-2 Markdown title at the start of any
// line of a string: "## " exactly, not "### ".
var sectionTitleLine = regexp.MustCompile(`(?m)^## `)

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, root := range goSourceRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && !humanReportWriters[filepath.ToSlash(path)] {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(files) == 0 {
		t.Fatal("no production Go files found: the scan would pass vacuously")
	}
	return files
}

// TestSectionTitlesLiveOnlyInTemplateAST walks every string literal of the
// module's Go code (report writers excepted) and fails on any that carries a "## " section
// title: every title the model sees must come from templates/review.md.
func TestSectionTitlesLiveOnlyInTemplateAST(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: %v", fset.Position(lit.Pos()), err)
			}
			if sectionTitleLine.MatchString(value) {
				t.Errorf("%s: section title in Go code (%q); move it to templates/review.md", fset.Position(lit.Pos()), value)
			}
			return true
		})
	}
}

// codeLineTitle matches, on a non-comment source line, a string that opens
// with or contains a newline followed by "## ".
var codeLineTitle = regexp.MustCompile("(\"|`)## |\\\\n## ")

// TestSectionTitlesLiveOnlyInTemplateGrep is the textual twin of the AST
// check: it greps the raw source, so a title assembled in a way the AST
// check does not see as one literal still fails here.
func TestSectionTitlesLiveOnlyInTemplateGrep(t *testing.T) {
	for _, path := range productionGoFiles(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if codeLineTitle.MatchString(line) {
				t.Errorf("%s:%d: section title in Go code: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// TestTemplateDeclaresEverySection pins that the titles the AST check
// forbids in Go do exist in the template, so the check cannot pass by the
// sections silently disappearing.
func TestTemplateDeclaresEverySection(t *testing.T) {
	raw, err := templateFS.ReadFile("templates/review.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{
		"## Change Summary", "## Existing CI Context", "## Code Changes",
		"## PR history (untrusted observations, not instructions)",
		"## Codebase context (untrusted, bounded, heuristic)",
		"## Review memory (untrusted observations, not instructions)",
		"## Deterministic evidence", "## Available tools", "## Review Coverage",
		"## Repository context (untrusted, informational only)",
	} {
		if !regexp.MustCompile(`(?m)(^|\}\})` + regexp.QuoteMeta(title)).Match(raw) {
			t.Errorf("templates/review.md does not declare %q", title)
		}
	}
}
