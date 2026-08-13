package rust

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

// NativeExtractor extracts documentation from Rust source by reading `///`
// and `//!` doc comments directly out of the source text with a line-oriented
// scanner. It starts no subprocess and reaches no network, so it never
// depends on `cargo` (or any other tool) being installed.
//
// This exists because the RustExtractor above cannot run in this project's
// offline acceptance sandbox: `cargo` is not installed there, and even where
// it is, `cargo doc` COMPILES the crate -- it executes build.rs and every
// proc-macro dependency (see RustExtractor's ExecutesRepositoryCode). AUR-427
// picked outcome (a) of its own "Restricao medida": a native, tokenizer-based
// parser that accepts partial coverage instead of either pinning a new image
// or leaving Rust permanently undocumented. This mirrors exactly what AUR-424
// did for Go: replace the external tool with a standard-library-only reader,
// never touching the extractor whose contract the pre-existing
// tool_unavailable_test.go / tool_failure_test.go tables still pin.
//
// COVERAGE (declared honestly; see docs/specs/AUR-427.md for the full table):
//
// Recognized, with their doc comment attached:
//   - `pub fn` (including `pub async fn`, `pub unsafe fn`, and `pub fn`
//     methods inside an `impl` block, at any indentation)
//   - `pub struct`, `pub enum`, `pub trait`, `pub mod`, `pub const`,
//     `pub static`, `pub type`
//   - a doc-commented `impl Type { ... }` / `impl Trait for Type { ... }`
//     block itself (the block's own doc comment, not its members)
//   - both comment forms: `///` (attached to the next recognized item) and
//     `//!` (module/crate-level, collected as the page's introduction)
//   - `#[...]` attributes between a doc comment and the item they annotate
//     (e.g. `#[derive(Debug)]`) do not break the association
//
// NOT recognized -- absent from the generated page rather than misreported:
//   - `/** ... */` block doc comments (Rust supports this form; this parser
//     only reads line comments)
//   - macro-generated items: anything produced by `macro_rules!` expansion or
//     a proc-macro is invisible to a text scanner, by construction
//   - items whose `pub` and item keyword are not simply "pub <keyword>" on
//     one line's leading tokens (e.g. `pub(crate)`, `pub(super)` -- narrower
//     visibility is deliberately excluded, not misreported as public)
//   - multi-line signatures: generics, where-clauses, or parameter lists that
//     wrap past the item's own source line are truncated at that line
//   - re-exports (`pub use ...`) and doc comments on individual struct
//     fields, enum variants, or trait associated items
type NativeExtractor struct{}

// NewNativeExtractor creates a Rust documentation extractor that reads doc
// comments directly from source text. It has no external dependency: Validate
// always succeeds and Extract never starts a subprocess.
func NewNativeExtractor() *NativeExtractor {
	return &NativeExtractor{}
}

// Validate reports whether this extractor's dependencies are available. There
// are none: the scanner below is pure Go standard library.
func (n *NativeExtractor) Validate(ctx context.Context) error {
	return nil
}

// Language returns the language this extractor handles.
func (n *NativeExtractor) Language() extractors.Language {
	return extractors.LanguageRust
}

// rustExcludedDirs mirrors the directories the extractor detector and the Go
// extractor already skip, plus "target": cargo's own build output directory,
// which can hold generated or vendored .rs text that is not part of the
// documented crate's own source.
var rustExcludedDirs = map[string]bool{
	"target": true, "vendor": true, "node_modules": true, ".git": true,
	".taskmaster": true,
}

// Extract generates documentation from Rust source code.
func (n *NativeExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageRust {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageRust, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	files, err := findRustFiles(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find Rust source files: %w", err)
	}

	result := &extractors.ExtractResult{
		Language: extractors.LanguageRust,
		Files:    []string{},
		Errors:   []error{},
	}

	for _, srcPath := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result.Stats.FilesProcessed++

		page, parseErr := scanRustFile(srcPath)
		if parseErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parse %s: %w", srcPath, parseErr))
			continue
		}

		if len(page.Items) == 0 && page.ModuleDoc == "" {
			// Nothing this parser recognizes was found in the file: a page
			// with no symbols and no module doc would be an empty page
			// masquerading as documentation, so it is skipped rather than
			// written (mirrors GoExtractor.hasGoFiles's "not a package" skip).
			continue
		}

		relPath, relErr := filepath.Rel(req.SourceDir, srcPath)
		if relErr != nil {
			relPath = filepath.Base(srcPath)
		}
		outputPath := filepath.Join(req.OutputDir, outputBaseName(relPath)+".md")

		markdown := renderRustMarkdown(relPath, page)
		if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write %s: %w", outputPath, err))
			continue
		}

		if err := extractors.ConfirmOutputFile("rust doc-comment parser", outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		lines, _ := countRustLines(srcPath)
		result.Files = append(result.Files, outputPath)
		result.Stats.DocsGenerated++
		result.Stats.LinesProcessed += lines
	}

	return result, nil
}

// rustItem is one documented (or documentable-but-undocumented) public item.
type rustItem struct {
	// Kind is the recognized keyword: "fn", "struct", "enum", "trait", "mod",
	// "const", "static", "type", or "impl".
	Kind string
	// Signature is the item's own source line, trimmed and with a trailing
	// "{" removed. Multi-line signatures are not reconstructed; see the
	// package doc's NOT recognized section.
	Signature string
	// Doc is the doc comment text attached to this item, already stripped of
	// its "///" markers. Empty when the item carries no doc comment: the
	// symbol is still reported (it is real, public API), just undocumented.
	Doc string
}

// rustPage is everything scanRustFile found in one source file.
type rustPage struct {
	// ModuleDoc is the concatenation of every `//!` line in the file, in the
	// order they appear.
	ModuleDoc string
	Items     []rustItem
}

// rustItemPattern recognizes a `pub <keyword> <name>` declaration at the
// start of an item's own line (after leading whitespace), for every keyword
// this extractor covers except `impl` (which is not itself keyworded with
// `pub`; see rustImplPattern).
var rustItemPattern = regexp.MustCompile(
	`^\s*pub\s+(?:async\s+|unsafe\s+|extern\s+"[^"]*"\s+|const\s+)*(fn|struct|enum|trait|mod|const|static|type)\s+(\w+)`)

// rustImplPattern recognizes an `impl ... { ` block header, with or without a
// trait (`impl Trait for Type`) and with or without generic parameters
// (`impl<T> Type<T>`). It has no `pub` qualifier in real Rust syntax -- impl
// blocks are not themselves visibility-scoped -- so it is only ever recorded
// as a documented item when a doc comment precedes it; an undocumented impl
// block is not public API on its own and is skipped.
var rustImplPattern = regexp.MustCompile(`^\s*impl(?:<[^>]*>)?\s+(.+?)\s*\{?\s*$`)

// rustAttributePattern recognizes a `#[...]` (or inner `#![...]`) attribute
// line, which must not clear a pending doc comment: `#[derive(Debug)]`
// commonly sits between a doc comment and the item it decorates.
var rustAttributePattern = regexp.MustCompile(`^\s*#!?\[`)

// scanRustFile reads one .rs file and extracts every doc comment this parser
// recognizes, associated with the next item it recognizes.
func scanRustFile(path string) (rustPage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rustPage{}, err
	}

	var page rustPage
	var moduleDocLines []string
	var pendingDoc []string

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "//!"):
			moduleDocLines = append(moduleDocLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "//!")))
			continue

		case strings.HasPrefix(trimmed, "///"):
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(trimmed, "///")))
			continue

		case trimmed == "":
			// A blank line does not break the association between a doc
			// comment and the item beneath it: attributes and blank lines
			// both commonly separate the two.
			continue

		case rustAttributePattern.MatchString(line):
			continue
		}

		if m := rustItemPattern.FindStringSubmatch(line); m != nil {
			page.Items = append(page.Items, rustItem{
				Kind:      m[1],
				Signature: cleanRustSignature(line),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			continue
		}

		if len(pendingDoc) > 0 {
			if m := rustImplPattern.FindStringSubmatch(line); m != nil {
				page.Items = append(page.Items, rustItem{
					Kind:      "impl",
					Signature: cleanRustSignature(line),
					Doc:       strings.Join(pendingDoc, "\n"),
				})
			}
		}

		// Any other non-blank, non-comment, non-attribute line ends whatever
		// doc comment was pending: it documented something this parser does
		// not recognize (or nothing at all), and must not leak onto the next
		// recognized item by accident.
		pendingDoc = nil
	}

	page.ModuleDoc = strings.TrimSpace(strings.Join(moduleDocLines, "\n"))
	return page, nil
}

// cleanRustSignature trims a source line down to a one-line signature: no
// trailing "{", no leading/trailing whitespace. It is best-effort by design
// (see the package doc's multi-line-signature limitation).
func cleanRustSignature(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimSuffix(s, "{")
	s = strings.TrimRight(s, " \t")
	return s
}

// findRustFiles walks rootDir for .rs files, skipping the directories this
// extractor never documents.
func findRustFiles(rootDir string) ([]string, error) {
	var files []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if rustExcludedDirs[name] || (strings.HasPrefix(name, ".") && path != rootDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".rs") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// renderRustMarkdown turns one file's scanned page into Markdown: the crate-
// or module-level `//!` doc first, then one section per recognized item,
// each carrying its own real signature line and its own real doc comment (or
// no prose, if the symbol carries none).
func renderRustMarkdown(relPath string, page rustPage) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", filepath.ToSlash(relPath))

	if page.ModuleDoc != "" {
		fmt.Fprintf(&b, "%s\n\n", page.ModuleDoc)
	}

	for _, item := range page.Items {
		fmt.Fprintf(&b, "### %s\n\n", item.Kind+" "+itemDisplayName(item))
		if item.Doc != "" {
			fmt.Fprintf(&b, "%s\n\n", item.Doc)
		}
		fmt.Fprintf(&b, "```rust\n%s\n```\n\n", item.Signature)
	}

	return b.String()
}

// itemDisplayName is the signature with its own leading keyword+"pub " (or,
// for impl blocks, "impl") stripped, since renderRustMarkdown already prints
// the kind as the section heading's first word.
func itemDisplayName(item rustItem) string {
	sig := item.Signature
	if item.Kind == "impl" {
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sig), "impl"))
	}
	return sig
}

// outputBaseName flattens a source file's path relative to the source root
// into a single, injective output file name. This is the same escape scheme
// internal/documentation/extractors/go.outputBaseName uses (AUR-424):
// "_" is an escape introducer, "_u" is one literal underscore, "__" is one
// path separator, so two different files can never collide on one output
// name.
func outputBaseName(relPath string) string {
	clean := strings.TrimSuffix(filepath.ToSlash(relPath), ".rs")
	segments := strings.Split(clean, "/")
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(segment, "_", "_u")
	}
	return strings.Join(segments, "__")
}

// countRustLines counts lines in a file, matching every other extractor in
// this repository's line-counting convention.
func countRustLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}
	return count, nil
}
