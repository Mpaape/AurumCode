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
