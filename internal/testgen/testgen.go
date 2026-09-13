package testgen

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestCase names one test to generate for a single Go package. Name is the
// proposed test function identifier (e.g. "TestFoo"), Package is the directory
// of the changed file, and Focus is the function or file the test targets.
type TestCase struct {
	Name    string
	Package string
	Focus   string
}

// Plan is the ordered set of test cases proposed for a diff.
type Plan struct {
	Cases []TestCase
}

// Runner executes "go test" for one package inside a sandbox. dir is the
// checkout root and packagePath is the package directory relative to it. The
// injected implementation owns the sandbox; this package never runs a command.
type Runner func(ctx context.Context, dir, packagePath string) (Result, error)

// Result is the raw outcome of one sandboxed "go test" invocation.
type Result struct {
	Package  string
	ExitCode int
	Output   string
}

// Outcome is the normalized result reported back to the caller.
type Outcome struct {
	Package string
	Passed  bool
	Output  string
}

// Propose scans the added lines of a diff for Go function and method
// declarations and derives a deterministic test plan. Files that are not Go
// source (or that are _test.go files) are skipped, and nothing is executed.
//
// The returned plan is deterministic: files are processed in path order,
// function names within a file are sorted and deduplicated, and a Go file that
// adds lines but declares no function yields a single file-derived case.
func Propose(diff *types.Diff) *Plan {
	plan := &Plan{}
	if diff == nil {
		return plan
	}

	files := make([]types.DiffFile, len(diff.Files))
	copy(files, diff.Files)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	for _, file := range files {
		if !isGoSource(file.Path) {
			continue
		}

		pkg := packageDir(file.Path)
		seen := make(map[string]bool)
		var cases []TestCase
		hasAddition := false

		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if isAddition(line) {
					hasAddition = true
				}
				name, focus, ok := addedFunction(line)
				if !ok || seen[name] {
					continue
				}
				seen[name] = true
				cases = append(cases, TestCase{Name: name, Package: pkg, Focus: focus})
			}
		}

		if len(cases) == 0 && hasAddition {
			if name := fileTestCase(file.Path); name != "" {
				cases = append(cases, TestCase{Name: name, Package: pkg, Focus: file.Path})
			}
		}

		sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
		plan.Cases = append(plan.Cases, cases...)
	}

	return plan
}

// Run executes the plan by invoking runner once per distinct package, in
// sorted order, and returns one Outcome per package. It never runs a command
// itself. The first runner error aborts the run and is returned to the caller.
func Run(ctx context.Context, plan *Plan, root string, runner Runner) ([]Outcome, error) {
	if plan == nil {
		return nil, nil
	}

	pkgs := make(map[string]struct{})
	for _, c := range plan.Cases {
		if c.Package != "" {
			pkgs[c.Package] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(pkgs))
	for p := range pkgs {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	outcomes := make([]Outcome, 0, len(sorted))
	for _, pkg := range sorted {
		if runner == nil {
			return nil, fmt.Errorf("testgen: nil runner for package %q", pkg)
		}
		res, err := runner(ctx, root, pkg)
		if err != nil {
			return nil, err
		}
		pkgName := res.Package
		if pkgName == "" {
			pkgName = pkg
		}
		outcomes = append(outcomes, Outcome{
			Package: pkgName,
			Passed:  res.ExitCode == 0,
			Output:  res.Output,
		})
	}

	return outcomes, nil
}

// isGoSource reports whether p is a Go source file that is not a test file.
func isGoSource(p string) bool {
	return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")
}

// packageDir returns the package directory of a Go file path, normalized to
// "." for files at the checkout root.
func packageDir(p string) string {
	dir := path.Dir(p)
	if dir == "" || dir == "." {
		return "."
	}
	return dir
}

func isAddition(line string) bool {
	return len(line) > 0 && line[0] == '+'
}

// addedFunction extracts a test-case name and focus from an added diff line
// that declares a Go function or method. Lines that are not additions, or that
// do not declare a function, return ok == false.
func addedFunction(line string) (name, focus string, ok bool) {
	if !isAddition(line) {
		return "", "", false
	}
	return functionFrom(strings.TrimSpace(line[1:]))
}

func functionFrom(content string) (name, focus string, ok bool) {
	if !strings.HasPrefix(content, "func ") {
		return "", "", false
	}
	rest := strings.TrimSpace(content[len("func "):])

	if strings.HasPrefix(rest, "(") {
		closeIdx := strings.IndexByte(rest, ')')
		if closeIdx < 0 {
			return "", "", false
		}
		recv := receiverType(rest[1:closeIdx])
		after := strings.TrimSpace(rest[closeIdx+1:])
		if after == "" {
			return "", "", false
		}
		idx := strings.IndexByte(after, '(')
		if idx < 0 {
			return "", "", false
		}
		method := strings.TrimSpace(after[:idx])
		if method == "" || recv == "" {
			return "", "", false
		}
		return "Test" + exported(recv) + exported(method), recv + "." + method, true
	}

	idx := strings.IndexByte(rest, '(')
	if idx < 0 {
		return "", "", false
	}
	fname := strings.TrimSpace(rest[:idx])
	if fname == "" {
		return "", "", false
	}
	return "Test" + exported(fname), fname, true
}

// receiverType reduces a method receiver such as "*Server", "s *Server", or
// "s *Server[T]" to the bare type name "Server".
func receiverType(recv string) string {
	recv = strings.TrimSpace(recv)
	fields := strings.Fields(recv)
	if len(fields) == 0 {
		return ""
	}
	t := fields[len(fields)-1]
	if i := strings.IndexByte(t, '['); i >= 0 {
		t = t[:i]
	}
	return strings.TrimPrefix(t, "*")
}

// fileTestCase derives a test-case name from a Go source file path.
func fileTestCase(p string) string {
	base := path.Base(p)
	stem := strings.TrimSuffix(base, ".go")
	if stem == "" || stem == base {
		return ""
	}
	return "Test" + exported(stem)
}

// exported converts a snake/kebab/dot-delimited identifier to CamelCase by
// uppercasing the first rune of each segment.
func exported(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	var b strings.Builder
	for _, f := range fields {
		if f == "" {
			continue
		}
		rs := []rune(f)
		b.WriteRune(unicode.ToUpper(rs[0]))
		b.WriteString(string(rs[1:]))
	}
	if b.Len() == 0 {
		return name
	}
	return b.String()
}
