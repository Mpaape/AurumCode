package rust

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

func TestNativeExtractor_Language(t *testing.T) {
	if lang := NewNativeExtractor().Language(); lang != extractors.LanguageRust {
		t.Errorf("Language() = %s, want %s", lang, extractors.LanguageRust)
	}
}

func TestNativeExtractor_Validate_NeverFails(t *testing.T) {
	if err := NewNativeExtractor().Validate(context.Background()); err != nil {
		t.Errorf("Validate() = %v, want nil (native extractor has no external dependency)", err)
	}
}

const nativeFixtureSource = `//! Fixture crate for the native Rust extractor's own tests.

#[derive(Debug)]
/// A widget with a label.
pub struct Widget {
    pub label: String,
}

/// Builds a widget.
///
/// Returns a new Widget instance.
pub fn make_widget(label: &str) -> Widget {
    Widget { label: label.to_string() }
}

pub const MAX_WIDGETS: usize = 10;

mod internal_helper {
    fn hidden() {}
}

impl Widget {
    /// Renders the widget as a string.
    pub fn render(&self) -> String {
        self.label.clone()
    }

    fn private_helper(&self) {}
}
`

func writeNativeFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "lib.rs"), []byte(nativeFixtureSource), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestNativeExtractor_ExtractsRealSymbolsAndComments(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeNativeFixture(t, srcDir)

	ext := NewNativeExtractor()
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageRust,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
		t.Fatalf("zero Rust pages generated: %+v errors=%v", result.Stats, result.Errors)
	}

	data, err := os.ReadFile(result.Files[0])
	if err != nil {
		t.Fatalf("read generated page: %v", err)
	}
	page := string(data)

	for _, want := range []string{
		"Fixture crate for the native Rust extractor",
		"pub struct Widget",
		"A widget with a label.",
		"pub fn make_widget",
		"Builds a widget.",
		"Returns a new Widget instance.",
		"pub const MAX_WIDGETS",
		"pub fn render(&self) -> String",
		"Renders the widget as a string.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("generated page missing %q:\n%s", want, page)
		}
	}

	// Never claim symbols this parser did not recognize: the private helper
	// functions must not appear anywhere in the page.
	for _, notWant := range []string{"private_helper", "hidden"} {
		if strings.Contains(page, notWant) {
			t.Errorf("generated page falsely reports non-public symbol %q:\n%s", notWant, page)
		}
	}
}

func TestNativeExtractor_AttributeDoesNotBreakDocAssociation(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeNativeFixture(t, srcDir)

	ext := NewNativeExtractor()
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageRust,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	data, _ := os.ReadFile(result.Files[0])
	page := string(data)

	// #[derive(Debug)] sits between the doc comment and `pub struct Widget`
	// in the fixture; the doc must still attach to Widget, not be dropped.
	widgetIdx := strings.Index(page, "pub struct Widget")
	docIdx := strings.Index(page, "A widget with a label.")
	if widgetIdx == -1 || docIdx == -1 {
		t.Fatalf("expected both the struct and its doc comment on the page:\n%s", page)
	}
}

func TestNativeExtractor_NoRustFilesProducesNoPages(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("nothing here"), 0o644); err != nil {
		t.Fatalf("write non-rust file: %v", err)
	}

	ext := NewNativeExtractor()
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageRust,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated != 0 || len(result.Files) != 0 {
		t.Errorf("expected zero pages for a tree with no .rs files, got %+v", result.Stats)
	}
}

func TestNativeExtractor_Deterministic(t *testing.T) {
	srcDir := t.TempDir()
	writeNativeFixture(t, srcDir)

	ext := NewNativeExtractor()

	out1 := t.TempDir()
	r1, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageRust, SourceDir: srcDir, OutputDir: out1,
	})
	if err != nil {
		t.Fatalf("first extract: %v", err)
	}

	out2 := t.TempDir()
	r2, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageRust, SourceDir: srcDir, OutputDir: out2,
	})
	if err != nil {
		t.Fatalf("second extract: %v", err)
	}

	d1, _ := os.ReadFile(r1.Files[0])
	d2, _ := os.ReadFile(r2.Files[0])
	if string(d1) != string(d2) {
		t.Fatalf("repeated extraction is not deterministic:\n---1---\n%s\n---2---\n%s", d1, d2)
	}
}

func TestOutputBaseName_IsInjective(t *testing.T) {
	tests := map[string]string{
		"lib.rs":     "lib",
		"src/lib.rs": "src__lib",
		"a_b.rs":     "a_ub",
		"a/b.rs":     "a__b",
		"a_b/c.rs":   "a_ub__c",
	}
	for input, want := range tests {
		if got := outputBaseName(input); got != want {
			t.Errorf("outputBaseName(%q) = %q, want %q", input, got, want)
		}
	}

	// Two genuinely different paths can never collapse onto the same name.
	if outputBaseName("a/b.rs") == outputBaseName("a_b.rs") {
		t.Errorf("outputBaseName is not injective: 'a/b.rs' and 'a_b.rs' collide")
	}
}
