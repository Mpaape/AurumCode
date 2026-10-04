package govet_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/scanner/govet"
)

const (
	goMod     = "module example.com/m\n\ngo 1.21\n"
	cleanCalc = "package calc\n\nimport \"fmt\"\n\n// Show prints n.\nfunc Show(n int) { fmt.Printf(\"%d\\n\", n) }\n"
	// badCalc adds a printf with a wrong argument type on line 9.
	badCalc = cleanCalc + "\n// Name prints s with a wrong verb.\nfunc Name(s string) { fmt.Printf(\"%d\\n\", s) }\n"
	// legacy is a vet diagnostic the reviewed range does not touch.
	legacy = "package legado\n\nimport \"fmt\"\n\n// Old prints s with a wrong verb.\nfunc Old(s string) { fmt.Printf(\"%d\\n\", s) }\n"
	// broken does not type-check: go vet cannot vet its package.
	broken = "package quebrado\n\nfunc F() int { return \"x\" }\n"
)

// repo is a git repository whose base commit holds base and whose head
// commit applies head over it.
type repo struct {
	root       string
	base, head string
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newRepo(t *testing.T, base, head map[string]string) repo {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main", root)
	write(t, root, base)
	git(t, root, "-C", root, "add", "-A")
	git(t, root, "-C", root, "commit", "-q", "-m", "base")
	r := repo{root: root, base: git(t, root, "-C", root, "rev-parse", "HEAD")}
	write(t, root, head)
	git(t, root, "-C", root, "add", "-A")
	git(t, root, "-C", root, "commit", "-q", "--allow-empty", "-m", "head")
	r.head = git(t, root, "-C", root, "rev-parse", "HEAD")
	return r
}

// tree is a reviewed tree without git: the files of the head, and the diff
// git would print for the lines the range added (path -> lines).
type tree struct {
	files map[string]string
	added map[string][]int
}

// cannedDiff is the zero-context diff `git diff --unified=0` prints for
// added lines, one hunk per line.
func (tr tree) cannedDiff() string {
	var b strings.Builder
	for path, lines := range tr.added {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", path, path, path, path)
		for _, n := range lines {
			fmt.Fprintf(&b, "@@ -%d,0 +%d @@\n+x\n", n-1, n)
		}
	}
	return b.String()
}

// scanTree runs the registered engine through the executor with the real
// go binary (the production command, with the engine's own environment)
// and git answered by the canned diff, so the proof needs no git binary.
func scanTree(t *testing.T, tr tree) scanner.Outcome {
	t.Helper()
	root := t.TempDir()
	write(t, root, tr.files)
	engine, ok := scanner.Lookup(govet.Name)
	if !ok {
		t.Fatal("govet is not registered")
	}
	real := scanner.IsolatedCommand(scanner.ChildEnvironment(os.LookupEnv, engine.Environment))
	command := func(ctx context.Context, dir, binary string, args ...string) (string, string, error) {
		if binary == "git" {
			return tr.cannedDiff(), "", nil
		}
		return real(ctx, dir, binary, args...)
	}
	r := scanner.Range{Base: strings.Repeat("a", 40), Head: strings.Repeat("b", 40)}
	return scanner.Executor{Command: command}.Scan(context.Background(), engine, scanner.Request{Root: root, Range: r})
}

func scan(t *testing.T, r repo) scanner.Outcome {
	t.Helper()
	engine, ok := scanner.Lookup(govet.Name)
	if !ok {
		t.Fatal("govet is not registered")
	}
	return scanner.Executor{}.Scan(context.Background(), engine, scanner.Request{Root: r.root, Range: scanner.Range{Base: r.base, Head: r.head}})
}

// AC-001: a real go vet diagnostic on a line the range added is a finding
// with file, line, the analyzer's rule and the engine's origin.
func TestRealVetFindingOnAddedLine(t *testing.T) {
	out := scanTree(t, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": badCalc}, added: map[string][]int{"calc/calc.go": {7, 8, 9}}})
	if out.Reason != "" {
		t.Fatalf("inconclusive: %s", out.Reason)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", out.Findings)
	}
	f := out.Findings[0]
	if f.Path != "calc/calc.go" || f.Line != 9 || f.RuleID != "go-vet/printf" || !strings.Contains(f.Message, "wrong type string") {
		t.Fatalf("finding = %+v", f)
	}
	if issue := f.ToIssue(out.Engine.TypedOrigin()); issue.Origin != govet.Name || out.Engine.Category != govet.Category {
		t.Fatalf("origin = %q category = %q", issue.Origin, out.Engine.Category)
	}
	if !strings.HasPrefix(out.Version, "go vet go1.") {
		t.Fatalf("version = %q", out.Version)
	}
}

// AC-001: the corrected change produces no finding, and is a complete scan.
func TestRealVetCorrectedChangeIsClean(t *testing.T) {
	fixed := strings.Replace(badCalc, `"%d\n", s`, `"%s\n", s`, 1)
	out := scanTree(t, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": fixed}, added: map[string][]int{"calc/calc.go": {7, 8, 9}}})
	if out.Reason != "" || len(out.Findings) != 0 {
		t.Fatalf("outcome = %+v, want clean", out)
	}
}

// MUT-001: a diagnostic outside the reviewed range is never this change's
// finding.
func TestRealVetIgnoresUntouchedLines(t *testing.T) {
	out := scanTree(t, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": badCalc, "legado/legado.go": legacy}, added: map[string][]int{"calc/calc.go": {7, 8, 9}}})
	if out.Reason != "" || len(out.Findings) != 1 || out.Findings[0].Path != "calc/calc.go" {
		t.Fatalf("outcome = %+v, want only the calc finding", out)
	}
}

// MUT-001: a package go vet cannot vet makes the scan inconclusive, even
// when another package has a diagnostic on an added line.
func TestRealVetFailureIsInconclusive(t *testing.T) {
	out := scanTree(t, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": badCalc, "quebrado/q.go": broken}, added: map[string][]int{"calc/calc.go": {7, 8, 9}, "quebrado/q.go": {1, 2, 3}}})
	if out.Reason != "lint_execution_error" || out.Findings != nil {
		t.Fatalf("outcome = %+v, want lint_execution_error and no findings", out)
	}
}

// AC-002: a missing go binary is a coverage limitation, not an accusation.
func TestMissingGoIsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	out := scanTree(t, tree{files: map[string]string{"go.mod": goMod, "calc/calc.go": badCalc}, added: map[string][]int{"calc/calc.go": {7, 8, 9}}})
	if out.Reason != "lint_unavailable" || out.Findings != nil {
		t.Fatalf("outcome = %+v, want lint_unavailable", out)
	}
}

// The same proof through the production command for git too: a real
// repository, a real `git diff` over the range. Skipped where git is absent
// (the sealed acceptance image); the canned-diff tests above carry the proof
// there.
func TestRealGitRangeEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := newRepo(t, map[string]string{"go.mod": goMod, "calc/calc.go": cleanCalc, "legado/legado.go": legacy}, map[string]string{"calc/calc.go": badCalc})
	out := scan(t, r)
	if out.Reason != "" || len(out.Findings) != 1 || out.Findings[0].Path != "calc/calc.go" || out.Findings[0].Line != 9 {
		t.Fatalf("outcome = %+v, want only calc/calc.go:9", out)
	}
}

// A root without go.mod (go vet would resolve a parent module whose paths
// the root's diff cannot map) is inconclusive, never clean.
func TestRootWithoutModuleIsInconclusive(t *testing.T) {
	out := scanTree(t, tree{files: map[string]string{"calc/calc.go": badCalc}, added: map[string][]int{"calc/calc.go": {9}}})
	if out.Reason != "lint_execution_error" || out.Findings != nil {
		t.Fatalf("outcome = %+v, want lint_execution_error", out)
	}
}

// The root may be a subdirectory of the repository: git diff --relative
// maps vet's root-relative paths, so the finding is kept.
func TestRealGitSubdirectoryRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := newRepo(t, map[string]string{"svc/go.mod": goMod, "svc/calc/calc.go": cleanCalc}, map[string]string{"svc/calc/calc.go": badCalc})
	r.root = filepath.Join(r.root, "svc")
	out := scan(t, r)
	if out.Reason != "" || len(out.Findings) != 1 || out.Findings[0].Path != "calc/calc.go" || out.Findings[0].Line != 9 {
		t.Fatalf("outcome = %+v, want calc/calc.go:9", out)
	}
}

// The child go receives CGO_ENABLED=0 whatever the reviewing process says.
func TestGoChildHasCgoDisabled(t *testing.T) {
	t.Setenv("CGO_ENABLED", "1")
	engine, _ := scanner.Lookup(govet.Name)
	env := scanner.ChildEnvironment(os.LookupEnv, engine.Environment)
	out, _, err := scanner.IsolatedCommand(env)(context.Background(), t.TempDir(), "/usr/bin/env")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CGO_ENABLED=0\n") || strings.Contains(out, "CGO_ENABLED=1") {
		t.Fatalf("child env = %q, want CGO_ENABLED=0 only", out)
	}
}

// Without a reviewed range nothing can be anchored: inconclusive, never a
// whole-tree scan.
func TestNoRangeIsInconclusive(t *testing.T) {
	engine, _ := scanner.Lookup(govet.Name)
	out := scanner.Executor{}.Scan(context.Background(), engine, scanner.Request{Root: t.TempDir()})
	if out.Reason != "lint_execution_error" {
		t.Fatalf("reason = %q", out.Reason)
	}
}

func TestRefusesOptions(t *testing.T) {
	engine, _ := scanner.Lookup(govet.Name)
	if err := engine.ValidateOptions(scanner.Options{"rules": "x"}); err == nil {
		t.Fatal("an option was accepted")
	}
}
