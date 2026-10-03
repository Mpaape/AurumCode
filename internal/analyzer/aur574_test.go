package analyzer

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

// AUR-574: a file whose grammar the runtime picks only from a generic
// extension (.txt -> vimdoc) is reviewed as text ("other"), never filed as
// documentation. Strong documentation grammars (.md) still are.
func TestAUR574_CategoryOfPath(t *testing.T) {
	d := NewLanguageDetector()
	cases := []struct {
		path, want string
	}{
		{"config/demo-tokens.txt", "other"},
		{"NOTES.txt", "other"},
		{"docs/guide.rst", "documentation"},
		{"README.md", "documentation"},
		{"main.go", "backend"},
		{"LICENSE", "other"},
	}
	for _, c := range cases {
		got := d.GetLanguageCategory(d.DetectLanguage(c.path))
		if got != c.want {
			t.Errorf("category(%s) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestAUR574_VimdocIsNeverDocumentation(t *testing.T) {
	d := NewLanguageDetector()
	if got := d.GetLanguageCategory("vimdoc"); got == "documentation" {
		t.Fatalf("vimdoc is chosen by the generic .txt extension and must not be documentation, got %s", got)
	}
}

// An extensionless file holding code is not documentation either: with a
// shebang the runtime names a code grammar, without one it stays "other".
func TestAUR574_ExtensionlessCode(t *testing.T) {
	d := NewLanguageDetector()
	lang := grammar.Default().Detect("deploy", []byte("#!/bin/bash\necho hi\n"))
	if lang == "" {
		t.Fatal("runtime did not detect the shebang")
	}
	if got := d.GetLanguageCategory(lang); got == "documentation" || got == "other" {
		t.Errorf("shebang script category = %s, want a code category", got)
	}
	if got := d.GetLanguageCategory(d.DetectLanguage("deploy")); got != "other" {
		t.Errorf("extensionless unknown file category = %s, want other", got)
	}
}
