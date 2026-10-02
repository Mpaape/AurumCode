package grammar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AUR-559 AC-002: a comment line is recognised by the grammar of each
// language, and the same text outside a comment is not.
func TestAUR559IsCommentByGrammar(t *testing.T) {
	cases := []struct {
		file, comment, code string
	}{
		{"a.go", "// total", "total := 1"},
		{"a.py", "# total", "total = 1"},
		{"a.rb", "# total", "total = 1"},
		{"A.java", "// total", "int total = 1;"},
		{"main.tf", "# total", "total = 1"},
		{"Dockerfile", "# total", "RUN total"},
	}
	rt := Runtime{}
	for _, c := range cases {
		lang := rt.Detect(c.file, nil)
		if lang == "" {
			t.Fatalf("%s: runtime has no grammar", c.file)
		}
		if got, ok := rt.IsComment(lang, c.comment); !ok || !got {
			t.Errorf("%s (%s): %q = (%v,%v), want comment", c.file, lang, c.comment, got, ok)
		}
		if got, ok := rt.IsComment(lang, c.code); !ok || got {
			t.Errorf("%s (%s): %q = (%v,%v), want not comment", c.file, lang, c.code, got, ok)
		}
	}
	if got, ok := rt.IsComment("go", "x := 1 // note"); !ok || got {
		t.Errorf("trailing comment is not a comment line: (%v,%v)", got, ok)
	}
	if _, ok := rt.IsComment("no-such-grammar", "// x"); ok {
		t.Error("unknown grammar must report ok=false")
	}
}

func TestAUR559AliasesEmbeddedAreValid(t *testing.T) {
	a, err := LoadAliases(Default(), "")
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	for alias, want := range map[string]string{"golang": "go", "TS": "typescript", "terraform": "hcl", "go": "go"} {
		if got, ok := a.Resolve(alias); !ok || got != want {
			t.Errorf("Resolve(%q) = %q,%v want %q", alias, got, ok, want)
		}
	}
	if _, ok := a.Resolve("klingon"); ok {
		t.Error("unknown name must not resolve")
	}
}

func writePolicy(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, filepath.FromSlash(PolicyAliasesPath))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestAUR559PolicyAliasesReplaceSection(t *testing.T) {
	root := writePolicy(t, "version: 1\naliases:\n  gopher: go\n")
	a, err := LoadAliases(Default(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := a.Resolve("gopher"); !ok || got != "go" || a.Source != "policy" {
		t.Errorf("policy alias not applied: %q %v %s", got, ok, a.Source)
	}
	if _, ok := a.Resolve("golang"); ok {
		t.Error("policy section must replace the embedded one")
	}
}

func TestAUR559AliasToMissingGrammarIsLoadError(t *testing.T) {
	root := writePolicy(t, "version: 1\naliases:\n  zz: no-such-grammar\n")
	_, err := LoadAliases(Default(), root)
	if err == nil || !strings.Contains(err.Error(), "no-such-grammar") {
		t.Fatalf("want load error naming the target, got %v", err)
	}
	root = writePolicy(t, "version: 1\nbogus: 1\n")
	if _, err := LoadAliases(Default(), root); err == nil {
		t.Fatal("unknown key must be a strict load error")
	}
}
