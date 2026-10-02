package context

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeTree writes the given slash-separated path -> content map under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveGoPackage(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"internal/foo/foo.go": "package foo\n\n// Helper does a thing.\nfunc Helper() {}\n\ntype Widget struct{}\n\nvar Global int\n",
		"cmd/main.go":         "package main\n\nimport \"internal/foo\"\n\nfunc main() { foo.Helper() }\n",
		"README.md":           "# repo\n",
	})

	pack, err := NewResolver().Resolve(root, []string{"internal/foo/foo.go"})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"Global", "Helper", "Widget"}; !reflect.DeepEqual(pack.Symbols, want) {
		t.Fatalf("symbols = %v, want %v", pack.Symbols, want)
	}
	if want := []string{"cmd/main.go"}; !reflect.DeepEqual(pack.Dependents, want) {
		t.Fatalf("dependents = %v, want %v", pack.Dependents, want)
	}

	foundImport := false
	foundSymbol := false
	for _, ref := range pack.References {
		if ref.File != "cmd/main.go" {
			t.Fatalf("unexpected reference file %q", ref.File)
		}
		if ref.Symbol == "internal/foo" {
			foundImport = true
			if ref.Line != 3 {
				t.Fatalf("import line = %d, want 3", ref.Line)
			}
		}
		if ref.Symbol == "Helper" {
			foundSymbol = true
			if ref.Line != 5 {
				t.Fatalf("symbol line = %d, want 5", ref.Line)
			}
		}
	}
	if !foundImport || !foundSymbol {
		t.Fatalf("missing import/symbol reference edges: %+v", pack.References)
	}
}

func TestResolvePython(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"pkg/util.py":     "def helper(x):\n    return x\n\nclass Widget:\n    pass\n",
		"app/main.py":     "from pkg.util import helper\n\ndef run():\n    return helper(1)\n",
		"app/consumer.py": "import pkg.util\n\nw = pkg.util.Widget()\n",
	})

	pack, err := NewResolver().Resolve(root, []string{"pkg/util.py"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Widget", "helper"}; !reflect.DeepEqual(pack.Symbols, want) {
		t.Fatalf("symbols = %v, want %v", pack.Symbols, want)
	}
	if want := []string{"app/consumer.py", "app/main.py"}; !reflect.DeepEqual(pack.Dependents, want) {
		t.Fatalf("dependents = %v, want %v", pack.Dependents, want)
	}
}

func TestResolveMissingAndSymlink(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"real.go": "package main\n",
	})
	target := filepath.Join(root, "real.go")
	if err := os.Symlink(target, filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}

	pack, err := NewResolver().Resolve(root, []string{"real.go", "does/not/exist.go", "link.go"})
	if err != nil {
		t.Fatal(err)
	}

	if len(pack.Symbols) != 0 {
		t.Fatalf("unexpected symbols: %v", pack.Symbols)
	}
	joined := strings.Join(pack.Dropped, "\n")
	if !strings.Contains(joined, "missing or non-regular changed file does/not/exist.go") {
		t.Fatalf("missing file not recorded: %v", pack.Dropped)
	}
	if !strings.Contains(joined, "link.go") {
		t.Fatalf("symlink not recorded: %v", pack.Dropped)
	}
}

func TestResolveDeterministic(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/a.go": "package a\n\nfunc One() {}\n",
		"b/b.go": "package b\n\nimport \"a\"\n\nfunc Two() { a.One() }\n",
	})

	r := NewResolver()
	first, err := r.Resolve(root, []string{"a/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Resolve(root, []string{"a/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic output:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestResolveLimits(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("x", 200)
	writeTree(t, root, map[string]string{
		"big.go":      "package big\n\nfunc F() { /* " + big + " */ }\n",
		"depender.go": "package main\n\nimport \"big\"\n\nfunc main() { big.F() }\n",
	})

	r := NewResolverWithLimits(Limits{MaxBytes: 64, MaxFiles: 1, MaxDependents: 1})
	pack, err := r.Resolve(root, []string{"big.go"})
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(pack.Dropped, "\n")
	if !strings.Contains(joined, "truncated big.go") {
		t.Fatalf("truncation not recorded: %v", pack.Dropped)
	}
	if len(pack.Dependents) > 1 {
		t.Fatalf("dependents not bounded: %v", pack.Dependents)
	}
	if !strings.Contains(joined, "file enumeration truncated") {
		t.Fatalf("enumeration truncation not recorded: %v", pack.Dropped)
	}
}

func TestResolveRootMissing(t *testing.T) {
	_, err := NewResolver().Resolve(filepath.Join(t.TempDir(), "nope"), nil)
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestResolveAbsoluteChangedPath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"x/x.go": "package x\n\nfunc X() {}\n",
		"y/y.go": "package y\n\nimport \"x\"\n\nfunc Y() { x.X() }\n",
	})

	pack, err := NewResolver().Resolve(root, []string{filepath.Join(root, "x", "x.go")})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"X"}; !reflect.DeepEqual(pack.Symbols, want) {
		t.Fatalf("symbols = %v, want %v", pack.Symbols, want)
	}
	if want := []string{"y/y.go"}; !reflect.DeepEqual(pack.Dependents, want) {
		t.Fatalf("dependents = %v, want %v", pack.Dependents, want)
	}
}

func TestResolveCrossChangedNotDependent(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/a.go": "package a\n\nfunc One() {}\n",
		"b/b.go": "package b\n\nimport \"a\"\n\nfunc Two() { a.One() }\n",
	})

	pack, err := NewResolver().Resolve(root, []string{"a/a.go", "b/b.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Dependents) != 0 {
		t.Fatalf("changed files must not be listed as dependents: %v", pack.Dependents)
	}
}

// TestEnumerateSkipsGitDirectoryAtAnyDepth covers AUR-536's B1: enumerate's
// own filesystem walk must never descend into a ".git" directory, whether
// it is the checkout's own top-level one or one nested under an ordinary
// subdirectory (a copied or embedded nested clone). Before this fix, a file
// sitting in either would be read like any other repo file and could reach
// Pack.References/Dependents.
func TestEnumerateSkipsGitDirectoryAtAnyDepth(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"app.go":        "package demo\n\nfunc Changed() {}\n",
		".git/zz.go":    "package demo\n\n// Changed\nvar _ = \"Changed\"\n",
		"sub/.git/x.go": "package demo\n\n// Changed\nvar _ = \"Changed\"\n",
		"sub/ok.go":     "package demo\n\n// Changed\nvar _ = \"Changed\"\n",
	})

	pack, err := NewResolver().Resolve(root, []string{"app.go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range pack.Dependents {
		if strings.Contains(dep, ".git") {
			t.Fatalf("a file under .git reached Dependents: %v", pack.Dependents)
		}
	}
	for _, ref := range pack.References {
		if strings.Contains(ref.File, ".git") {
			t.Fatalf("a file under .git reached References: %v", pack.References)
		}
	}
	// sub/ok.go sits next to the nested ".git" dir, outside it, and must
	// still be found -- proving the skip is scoped to ".git" itself, not
	// to "sub" as a whole.
	found := false
	for _, dep := range pack.Dependents {
		if dep == "sub/ok.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sub/ok.go, outside the nested .git, should still be scanned: %v", pack.Dependents)
	}
}

// TestEnumerateDroppedPathsAreRepoRelative covers AUR-536's B5: every
// Dropped note enumerate records -- a skipped symlink included -- must name
// a path relative to root, never WalkDir's own absolute path. A published
// review's "Codebase context" section is exactly where Dropped ends up
// (Pack is serialized whole), so an absolute note would leak this host's
// own directory layout into the model prompt and the published review.
func TestEnumerateDroppedPathsAreRepoRelative(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"real.go": "package main\n"})
	if err := os.Symlink(filepath.Join(root, "real.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}

	pack, err := NewResolver().Resolve(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(pack.Dropped, "\n")
	if !strings.Contains(joined, "skipped symlink link.go") {
		t.Fatalf("expected a repo-relative symlink note, got: %v", pack.Dropped)
	}
	if strings.Contains(joined, root) {
		t.Fatalf("Dropped leaked the absolute host path %q: %v", root, pack.Dropped)
	}
}

// TestResolveWithFilesRestrictsScanToExactSet covers AUR-536's B1: the
// repo-wide reference scan must read exactly the files ResolveWithFiles was
// given, never anything its own filesystem walk would otherwise also find.
// extra exists on disk, references the changed symbol, and would appear in
// Resolve's own Dependents (proving the fixture is not vacuous) -- but
// ResolveWithFiles, given a files set that excludes it, must never surface
// it, because it never reads it at all.
func TestResolveWithFilesRestrictsScanToExactSet(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"app.go":   "package demo\n\nfunc Changed() {}\n",
		"extra.go": "package demo\n\n// Changed\nvar _ = \"Changed\"\n",
	})

	walked, err := NewResolver().Resolve(root, []string{"app.go"})
	if err != nil {
		t.Fatal(err)
	}
	foundExtra := false
	for _, dep := range walked.Dependents {
		if dep == "extra.go" {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Fatalf("fixture is vacuous: Resolve's own walk never found extra.go: %v", walked.Dependents)
	}

	restricted, err := NewResolver().ResolveWithFiles(root, []string{"app.go"}, []string{"app.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(restricted.Dependents) != 0 {
		t.Fatalf("ResolveWithFiles read a file outside its allowed set: %v", restricted.Dependents)
	}
}
