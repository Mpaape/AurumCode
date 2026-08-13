// Package unit holds AUR-427's Unit-layer proof: the native, tool-free
// documentation extractors for Rust (internal/documentation/extractors/rust)
// and C# (internal/documentation/extractors/csharp) parse real `///`/`//!`
// doc comments out of real source text, render real Markdown pages from
// them, and start no subprocess to do it.
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office (AUR-402..AUR-411, AUR-422, AUR-424, AUR-426):
// tests/acceptance/AUR-427.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR427, so the
// assertions below run inside the sandboxed acceptance instead of being
// swept into an unrelated top-level `go test ./...`.
package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	csharpextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	rustextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
)

const rustFixtureAUR427 = `//! Fixture crate for AUR-427's Unit-layer proof.

/// A widget with a label.
pub struct Widget {
    /// The widget's label.
    pub label: String,
}

/// Builds a widget with the given label.
pub fn make_widget(label: &str) -> Widget {
    Widget { label: label.to_string() }
}

/// A greeting behavior any widget-like type can implement.
pub trait Greet {
    /// Says hello.
    fn hello(&self) -> String;
}

/// Helper functions for widgets.
pub mod helpers {
    /// Returns the default label.
    pub fn default_label() -> &'static str {
        "widget"
    }
}

/// The default widget label, shared across the crate.
pub static DEFAULT_LABEL: &str = "widget";

/// A type alias for a widget identifier.
pub type WidgetId = u64;

/// Additional widget behavior, documented on the impl block itself.
impl Widget {
    /// Renders the widget as a string.
    pub fn render(&self) -> String {
        self.label.clone()
    }
}

// Not pub: must never appear on the generated page.
fn private_helper() -> bool {
    true
}

// A macro-generated item: invisible to a text scanner by construction (its
// name is a substitution parameter, not literal source text), and must
// never appear on the generated page either.
macro_rules! define_hidden_fn {
    ($name:ident) => {
        pub fn $name() -> bool { true }
    };
}
define_hidden_fn!(macro_generated);
`

const csharpFixtureAUR427 = `namespace Fixture
{
    /// <summary>
    /// A widget with a label.
    /// </summary>
    public class Widget
    {
        /// <summary>
        /// Builds a widget with the given label.
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

        private void PrivateHelper() {}
    }

    /// <summary>
    /// A point in 2D space.
    /// </summary>
    public struct Point
    {
        public int X;
        public int Y;
    }

    /// <summary>
    /// A named color.
    /// </summary>
    /// <seealso cref="Widget"/>
    public record ColorName(string Name);
}
`

func writeFileAUR427(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFileAUR427(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated page %s: %v", path, err)
	}
	return string(b)
}

// TestAUR427 is AUR-427's Unit-layer selector. It proves, for both
// extractors, that:
//
//  1. Feeding the extractor real Rust/C# source produces a real Markdown
//     page containing the real public symbols and their real doc comments
//     (not an empty file, not a placeholder).
//  2. Validate never depends on anything external: it always returns nil, so
//     the pipeline can never classify either language as "tool unavailable"
//     and skip it (the exact defect AUR-427 closes; see
//     cmd/regenerate-docs/repo_code_execution.go for why the OLD
//     cargo/dotnet-backed extractors, untouched by this card, remain gated
//     behind an explicit opt-in instead).
//  3. Coverage is honestly partial: a private member and a macro-generated
//     symbol -- both real, both present in the source -- never appear on the
//     generated page, because this parser genuinely cannot recognize them.
func TestAUR427(t *testing.T) {
	t.Run("rust", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR427(t, filepath.Join(srcDir, "lib.rs"), rustFixtureAUR427)

		ext := rustextractor.NewNativeExtractor()

		if err := ext.Validate(context.Background()); err != nil {
			t.Fatalf("AUR-427/AC-001/behavior-missing: Validate depends on something external: %v", err)
		}

		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language:  extractors.LanguageRust,
			SourceDir: srcDir,
			OutputDir: outDir,
		})
		if err != nil {
			t.Fatalf("AUR-427/AC-001/behavior-missing: Extract failed: %v", err)
		}
		if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
			t.Fatalf("AUR-427/AC-001/behavior-missing: zero Rust pages generated (processed=%d generated=%d errors=%v)",
				result.Stats.FilesProcessed, result.Stats.DocsGenerated, result.Errors)
		}

		page := readFileAUR427(t, result.Files[0])
		for _, want := range []string{
			"Fixture crate for AUR-427",
			"pub struct Widget", "A widget with a label.",
			"pub fn make_widget", "Builds a widget with the given label.",
			// Every remaining `pub` kind this parser claims to cover in
			// docs/specs/AUR-427.md's coverage table, each proven here so the
			// spec cannot out-claim what the accept actually exercises.
			"pub trait Greet", "A greeting behavior any widget-like type can implement.",
			"pub mod helpers", "Helper functions for widgets.",
			"pub static DEFAULT_LABEL", "The default widget label, shared across the crate.",
			"pub type WidgetId", "A type alias for a widget identifier.",
			// A doc-commented impl block: the one Rust kind gated behind a
			// conditional branch (native.go only records it when a doc
			// comment precedes it), so it gets its own explicit proof.
			"### impl Widget", "Additional widget behavior, documented on the impl block itself.",
		} {
			if !strings.Contains(page, want) {
				t.Fatalf("AUR-427/AC-001/behavior-missing: generated page missing %q:\n%s", want, page)
			}
		}

		for _, notWant := range []string{"private_helper", "macro_generated"} {
			if strings.Contains(page, notWant) {
				t.Fatalf("AUR-427/AC-001/false-claim: generated page mentions %q, which this parser cannot extract:\n%s",
					notWant, page)
			}
		}
	})

	t.Run("csharp", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR427(t, filepath.Join(srcDir, "Widget.cs"), csharpFixtureAUR427)

		ext := csharpextractor.NewNativeExtractor()

		if err := ext.Validate(context.Background()); err != nil {
			t.Fatalf("AUR-427/AC-001/behavior-missing: Validate depends on something external: %v", err)
		}

		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language:  extractors.LanguageCSharp,
			SourceDir: srcDir,
			OutputDir: outDir,
		})
		if err != nil {
			t.Fatalf("AUR-427/AC-001/behavior-missing: Extract failed: %v", err)
		}
		if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
			t.Fatalf("AUR-427/AC-001/behavior-missing: zero C# pages generated (processed=%d generated=%d errors=%v)",
				result.Stats.FilesProcessed, result.Stats.DocsGenerated, result.Errors)
		}

		page := readFileAUR427(t, result.Files[0])
		for _, want := range []string{
			"public class Widget", "A widget with a label.",
			"public Widget(string label)", "Builds a widget with the given label.",
			"public string Label { get; }",
			// Every remaining `public` kind this parser claims to cover in
			// docs/specs/AUR-427.md's coverage table.
			"public struct Point", "A point in 2D space.",
			"public record ColorName(string Name)", "A named color.",
		} {
			if !strings.Contains(page, want) {
				t.Fatalf("AUR-427/AC-001/behavior-missing: generated page missing %q:\n%s", want, page)
			}
		}

		// The record's doc comment carries an unrecognized <seealso> element
		// alongside the recognized <summary>. It must be ignored, not cause
		// the whole comment to be rejected -- proving the spec's "unrecognized
		// elements inside the comment are ignored" claim.
		if strings.Contains(page, "seealso") {
			t.Fatalf("AUR-427/AC-001/behavior-missing: unrecognized XML element leaked into the rendered page instead of being ignored:\n%s", page)
		}

		if strings.Contains(page, "PrivateHelper") {
			t.Fatalf("AUR-427/AC-001/false-claim: generated page mentions non-public member PrivateHelper:\n%s", page)
		}
	})

	t.Logf("AUR-427/AC-001/pass rust=ok csharp=ok")
}
