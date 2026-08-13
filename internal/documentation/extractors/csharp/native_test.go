package csharp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

func TestNativeExtractor_Language(t *testing.T) {
	if lang := NewNativeExtractor().Language(); lang != extractors.LanguageCSharp {
		t.Errorf("Language() = %s, want %s", lang, extractors.LanguageCSharp)
	}
}

func TestNativeExtractor_Validate_NeverFails(t *testing.T) {
	if err := NewNativeExtractor().Validate(context.Background()); err != nil {
		t.Errorf("Validate() = %v, want nil (native extractor has no external dependency)", err)
	}
}

const nativeFixtureSource = `using System;

namespace Fixture
{
    /// <summary>
    /// A widget with a label.
    /// </summary>
    public class Widget
    {
        /// <summary>
        /// Creates a widget.
        /// </summary>
        /// <param name="label">The widget's label.</param>
        public Widget(string label)
        {
            Label = label;
        }

        /// <summary>
        /// The widget's label.
        /// </summary>
        public string Label { get; }

        /// <summary>
        /// Renders the widget.
        /// </summary>
        /// <returns>The label as a string.</returns>
        /// <remarks>This is a trivial example.</remarks>
        public string Render()
        {
            return Label;
        }

        private void PrivateHelper() {}
    }

    /// Malformed doc: <notclosed
    public interface IGreeter
    {
        void Hello();
    }
}
`

func writeNativeFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Widget.cs"), []byte(nativeFixtureSource), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestNativeExtractor_ExtractsRealSymbolsAndComments(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeNativeFixture(t, srcDir)

	ext := NewNativeExtractor()
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
		t.Fatalf("zero C# pages generated: %+v errors=%v", result.Stats, result.Errors)
	}

	data, err := os.ReadFile(result.Files[0])
	if err != nil {
		t.Fatalf("read generated page: %v", err)
	}
	page := string(data)

	for _, want := range []string{
		"public class Widget",
		"A widget with a label.",
		"public Widget(string label)",
		"Creates a widget.",
		"label",
		"The widget's label.",
		"public string Label { get; }",
		"public string Render()",
		"Renders the widget.",
		"The label as a string.",
		"This is a trivial example.",
		// The interface's doc comment is not well-formed XML (an unclosed
		// tag); it must still be extracted and shown, as raw text.
		"public interface IGreeter",
		"Malformed doc:",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("generated page missing %q:\n%s", want, page)
		}
	}

	if strings.Contains(page, "PrivateHelper") {
		t.Errorf("generated page falsely reports non-public member PrivateHelper:\n%s", page)
	}
}

func TestNativeExtractor_ConstructorVsMethod(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeNativeFixture(t, srcDir)

	ext := NewNativeExtractor()
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	data, _ := os.ReadFile(result.Files[0])
	page := string(data)

	if !strings.Contains(page, "### constructor public Widget(string label)") {
		t.Errorf("Widget's constructor was not classified as a constructor:\n%s", page)
	}
	if !strings.Contains(page, "### method public string Render()") {
		t.Errorf("Render was not classified as a method:\n%s", page)
	}
	if !strings.Contains(page, "### property public string Label { get; }") {
		t.Errorf("Label was not classified as a property:\n%s", page)
	}
}

func TestNativeExtractor_NoCSharpFilesProducesNoPages(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("nothing here"), 0o644); err != nil {
		t.Fatalf("write non-csharp file: %v", err)
	}

	ext := NewNativeExtractor()
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated != 0 || len(result.Files) != 0 {
		t.Errorf("expected zero pages for a tree with no .cs files, got %+v", result.Stats)
	}
}

func TestNativeExtractor_Deterministic(t *testing.T) {
	srcDir := t.TempDir()
	writeNativeFixture(t, srcDir)

	ext := NewNativeExtractor()

	out1 := t.TempDir()
	r1, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageCSharp, SourceDir: srcDir, OutputDir: out1,
	})
	if err != nil {
		t.Fatalf("first extract: %v", err)
	}

	out2 := t.TempDir()
	r2, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageCSharp, SourceDir: srcDir, OutputDir: out2,
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
		"Widget.cs":     "Widget",
		"src/Widget.cs": "src__Widget",
		"a_b.cs":        "a_ub",
		"a/b.cs":        "a__b",
	}
	for input, want := range tests {
		if got := outputBaseName(input); got != want {
			t.Errorf("outputBaseName(%q) = %q, want %q", input, got, want)
		}
	}

	if outputBaseName("a/b.cs") == outputBaseName("a_b.cs") {
		t.Errorf("outputBaseName is not injective: 'a/b.cs' and 'a_b.cs' collide")
	}
}
