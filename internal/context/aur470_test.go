package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const aur470Contract = `package shop;

public class Contract {
    public static long priceOf(String sku, String currency) {
        return sku.length() * 100L;
    }
}
`

const aur470Caller = `package shop;

public class Caller {
    public long total(String sku) {
        long base = 1;
        return Contract.priceOf(sku) + base;
    }
}
`

func aur470Tree(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"src/shop/Contract.java":      aur470Contract,
		"src/shop/Caller.java":        aur470Caller,
		"test/shop/ContractTest.java": "package shop;\n\nclass ContractTest {\n    long t() { return Contract.priceOf(\"a\", \"BRL\"); }\n}\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	writeTree(t, root, files)
	return root
}

// snippetAt is the snippet of file at line, or the first of file when
// line is 0.
func snippetAt(pack *Pack, file string, line int) *Snippet {
	for i := range pack.Snippets {
		if pack.Snippets[i].File == file && (line == 0 || pack.Snippets[i].Line == line) {
			return &pack.Snippets[i]
		}
	}
	return nil
}

// AC-001: the caller left behind by the changed contract is excerpted with
// its file and line, and a test that uses it is labeled as a test.
func TestAUR470CallerExcerptWithFileAndLine(t *testing.T) {
	root := aur470Tree(t, nil)
	pack, err := NewResolver().Resolve(root, []string{"src/shop/Contract.java"})
	if err != nil {
		t.Fatal(err)
	}
	caller := snippetAt(pack, "src/shop/Caller.java", 6)
	if caller == nil || (caller.Symbol != "priceOf" && caller.Symbol != "Contract") || caller.Kind != SnippetUse {
		t.Fatalf("caller snippet = %+v; pack=%+v", caller, pack)
	}
	if !strings.Contains(caller.Text, "6:         return Contract.priceOf(sku) + base;") || !strings.Contains(caller.Text, "4:     public long total(String sku) {") {
		t.Fatalf("caller excerpt lacks the numbered lines:\n%s", caller.Text)
	}
	if test := snippetAt(pack, "test/shop/ContractTest.java", 4); test == nil || test.Kind != SnippetTest {
		t.Fatalf("test snippet = %+v", test)
	}
}

// AC-002: ignored and secret files, and symlinks, never enter the context;
// each omission is declared.
func TestAUR470ExcludedFilesAndSymlinksNeverEnter(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "Host.java"), []byte("class Host { long x = Contract.priceOf(\"h\"); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := aur470Tree(t, map[string]string{
		"vendor/Lib.java": "class Lib { long v = Contract.priceOf(\"v\"); }\n",
		"config/app.env":  "PRICE=Contract.priceOf\n",
	})
	if err := os.Symlink(filepath.Join(outside, "Host.java"), filepath.Join(root, "src", "Host.java")); err != nil {
		t.Fatal(err)
	}
	exclude := func(p string) bool { return strings.HasPrefix(p, "vendor/") || strings.HasSuffix(p, ".env") }
	pack, err := NewResolver().WithExclude(exclude).Resolve(root, []string{"src/shop/Contract.java"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pack)
	for _, leaked := range []string{"vendor/Lib.java", "Lib {", "PRICE=", "Host {"} {
		if strings.Contains(string(raw), leaked) && !strings.Contains(strings.Join(pack.Dropped, "\n"), leaked) {
			t.Fatalf("%q entered the context: %s", leaked, raw)
		}
	}
	for _, s := range pack.Snippets {
		if s.File == "vendor/Lib.java" || s.File == "config/app.env" || s.File == "src/Host.java" {
			t.Fatalf("excluded file excerpted: %+v", s)
		}
	}
	dropped := strings.Join(pack.Dropped, "\n")
	if !strings.Contains(dropped, "excluded by policy vendor/Lib.java") || !strings.Contains(dropped, "skipped symlink src/Host.java") {
		t.Fatalf("omissions not declared: %v", pack.Dropped)
	}
}

// AC-003: the same revision yields the same excerpts in the same order;
// the excerpt ceiling is declared.
func TestAUR470StableOrderAndDeclaredOmissions(t *testing.T) {
	extra := map[string]string{}
	for i := 0; i < DefaultMaxSnippets+3; i++ {
		extra[filepath.ToSlash(filepath.Join("src", "use", "U"+string(rune('a'+i%26))+string(rune('a'+i/26))+".java"))] = "class U { long v = Contract.priceOf(\"x\"); }\n"
	}
	root := aur470Tree(t, extra)
	first, err := NewResolver().Resolve(root, []string{"src/shop/Contract.java"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewResolver().Resolve(root, []string{"src/shop/Contract.java"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Snippets, second.Snippets) || len(first.Snippets) != DefaultMaxSnippets {
		t.Fatalf("snippets not stable or not bounded: %d vs %d", len(first.Snippets), len(second.Snippets))
	}
	for i := 1; i < len(first.Snippets); i++ {
		a, b := first.Snippets[i-1], first.Snippets[i]
		if a.File > b.File || (a.File == b.File && a.Line > b.Line) {
			t.Fatalf("snippets out of order: %+v before %+v", a, b)
		}
	}
	if !strings.Contains(strings.Join(first.Dropped, "\n"), "snippets truncated at") {
		t.Fatalf("the ceiling was not declared: %v", first.Dropped)
	}
}

// AC-004: a compatible caller is excerpted as a use, never labeled as a
// defect: a snippet carries no severity, verdict or finding.
func TestAUR470CompatibleCallerIsContextNotADefect(t *testing.T) {
	root := aur470Tree(t, map[string]string{
		"src/shop/Caller.java": "package shop;\n\nclass Caller {\n    long ok() { return Contract.priceOf(\"a\", \"BRL\"); }\n}\n",
	})
	pack, err := NewResolver().Resolve(root, []string{"src/shop/Contract.java"})
	if err != nil {
		t.Fatal(err)
	}
	caller := snippetAt(pack, "src/shop/Caller.java", 4)
	if caller == nil || caller.Kind != SnippetUse {
		t.Fatalf("compatible caller snippet = %+v", caller)
	}
	raw, _ := json.Marshal(caller)
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"severity", "verdict", "finding", "defect", "rule_id"} {
		if _, ok := fields[k]; ok {
			t.Fatalf("a snippet carries %q: a reference was labeled as a defect: %s", k, raw)
		}
	}
	if len(fields) != 5 {
		t.Fatalf("snippet fields = %v, want file, line, symbol, kind, text", fields)
	}
}
