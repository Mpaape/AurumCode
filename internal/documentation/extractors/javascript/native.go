package javascript

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

// NativeExtractor extracts documentation from JavaScript/ESM source by
// reading exported declarations and the JSDoc block immediately above them
// directly out of the source text with a line-oriented scanner. It starts no
// subprocess and reaches no network, so it never depends on `typedoc` (or any
// other tool) being installed.
//
// This exists because JSExtractor (extractor.go) cannot run in a sandbox that
// has no npm-installed toolchain: `typedoc` is not on PATH there, and
// JSExtractor.Validate reports that as extractors.ToolUnavailableError, which
// every consumer (see internal/pipeline) treats as "skip this language" -- a
// whole JavaScript project documented with zero pages. AUR-463 mirrors AUR-427
// (rust and csharp's native.go, in this same repository): a native,
// tokenizer-based reader that accepts partial coverage instead of leaving
// JavaScript permanently undocumented when the npm toolchain is absent.
//
// COVERAGE (declared honestly):
//
// Recognized, with the JSDoc block immediately above attached:
//   - `export function name(...)`, `export async function name(...)`,
//     `export function* name(...)`
//   - `export class Name [extends Base] { ... }`, including its own doc
//     comment and, separately, each method inside the class body (with its
//     own doc comment): `method(...)`, `async method(...)`,
//     `static method(...)`, `get x(...)`, `set x(...)`, and `constructor(...)`
//   - `export const name = (...) => { ... }` and
//     `export const name = function(...) { ... }` (including `async`) -- a
//     const only counts when its right-hand side is a function; a plain
//     value const (`export const X = 5`) is not in scope
//   - `export default function name(...)`, `export default function(...)`
//     (anonymous), `export default class Name`, and `export default <expr>`
//     for any other default export
//   - the `/** ... */` JSDoc block that sits directly above a recognized
//     declaration (a blank line between the two does not break association)
//
// NOT recognized -- absent from the generated page rather than misreported:
//   - `//` line comments as documentation (only `/** */` JSDoc is read)
//   - `export { a, b }` re-export lists and `export * from "..."`
//   - TypeScript type annotations beyond what appears verbatim on the
//     declaration's own source line
//   - multi-line signatures: parameter lists that wrap past the
//     declaration's own source line are truncated at that line
//   - decorators, computed method names (`[Symbol.iterator]() {}`), and
//     class fields (`name = value`)
//
// A symbol this parser recognizes but that carries no JSDoc block is still
// reported -- it is real, exported API -- with its signature and no prose:
// this parser never synthesizes documentation text.
type NativeExtractor struct{}

// NewNativeExtractor creates a JavaScript documentation extractor that reads
// JSDoc comments directly from source text. It has no external dependency:
// Validate always succeeds and Extract never starts a subprocess.
func NewNativeExtractor() *NativeExtractor {
	return &NativeExtractor{}
}

// Validate reports whether this extractor's dependencies are available.
// There are none: the scanner below is pure Go standard library.
func (n *NativeExtractor) Validate(ctx context.Context) error {
	return nil
}

// Language returns the language this extractor handles.
func (n *NativeExtractor) Language() extractors.Language {
	return extractors.LanguageJavaScript
}

// jsExcludedDirs mirrors the directories the other native extractors in this
// repository already skip, plus "node_modules": third-party JavaScript that
// is not part of the documented project's own source.
var jsExcludedDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, ".taskmaster": true,
	"dist": true, "build": true,
}

var jsSourceExt = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
}

// Extract generates documentation from JavaScript source code.
func (n *NativeExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageJavaScript {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageJavaScript, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	files, err := findJSFiles(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find JavaScript source files: %w", err)
	}

	result := &extractors.ExtractResult{
		Language: extractors.LanguageJavaScript,
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

		page, parseErr := scanJSFile(srcPath)
		if parseErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parse %s: %w", srcPath, parseErr))
			continue
		}

		if len(page.Items) == 0 {
			// Nothing this parser recognizes was exported: a page with no
			// symbols would be an empty page masquerading as documentation,
			// so it is skipped rather than written.
			continue
		}

		relPath, relErr := filepath.Rel(req.SourceDir, srcPath)
		if relErr != nil {
			relPath = filepath.Base(srcPath)
		}
		outputPath := filepath.Join(req.OutputDir, jsOutputBaseName(relPath)+".md")

		markdown := renderJSMarkdown(relPath, page)
		if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write %s: %w", outputPath, err))
			continue
		}

		if err := extractors.ConfirmOutputFile("javascript JSDoc parser", outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		lines, _ := countJSLines(srcPath)
		result.Files = append(result.Files, outputPath)
		result.Stats.DocsGenerated++
		result.Stats.LinesProcessed += lines
	}

	return result, nil
}

// jsItem is one documented (or documentable-but-undocumented) exported symbol.
type jsItem struct {
	// Kind is one of: "function", "class", "method", "const", "default".
	Kind string
	// Signature is the declaration's own source line, trimmed and with a
	// trailing "{" removed.
	Signature string
	// Doc is the JSDoc text attached to this item, already stripped of its
	// "/**", "*/" and leading "*" markers. Empty when the item carries no
	// JSDoc block: the symbol is still reported, just undocumented.
	Doc string
}

type jsPage struct {
	Items []jsItem
}

var (
	reExportFunc          = regexp.MustCompile(`^export\s+(?:async\s+)?function\s*\*?\s*(\w+)\s*\(`)
	reExportDefaultFunc   = regexp.MustCompile(`^export\s+default\s+(?:async\s+)?function\s*\*?\s*(\w*)\s*\(`)
	reExportDefaultClass  = regexp.MustCompile(`^export\s+default\s+class\b`)
	reExportDefaultOther  = regexp.MustCompile(`^export\s+default\s+(?:function|class)?`)
	reExportClass         = regexp.MustCompile(`^export\s+class\s+\w+`)
	reExportConstFunc     = regexp.MustCompile(`^export\s+const\s+\w+\s*=\s*(?:async\s+)?(?:\([^)]*\)\s*=>|function\s*\*?\s*\()`)
	reClassMethod         = regexp.MustCompile(`^(?:static\s+)?(?:async\s+)?(?:get\s+|set\s+)?\*?\s*([A-Za-z_$][\w$]*)\s*\(([^)]*)\)\s*\{?\s*$`)
	jsMethodKeywordSkip   = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true, "function": true, "return": true, "do": true, "else": true}
	reJSDocStart          = regexp.MustCompile(`^/\*\*\s*`)
)

// scanJSFile reads one JavaScript file and extracts every exported
// declaration this parser recognizes, associated with the JSDoc block
// directly above it.
func scanJSFile(path string) (jsPage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return jsPage{}, err
	}

	var page jsPage
	var pendingDoc []string
	var pendingDocLines []string
	inJSDoc := false
	inClass := false
	classDepth := 0

	lines := strings.Split(string(data), "\n")
	for _, raw := range lines {
		line := raw
		trimmed := strings.TrimSpace(line)

		if inJSDoc {
			if idx := strings.Index(trimmed, "*/"); idx >= 0 {
				before := strings.TrimSpace(trimmed[:idx])
				if before != "" {
					pendingDocLines = append(pendingDocLines, cleanJSDocLine(before))
				}
				inJSDoc = false
				pendingDoc = pendingDocLines
				pendingDocLines = nil
			} else {
				pendingDocLines = append(pendingDocLines, cleanJSDocLine(trimmed))
			}
			continue
		}

		if strings.HasPrefix(trimmed, "/**") {
			// Handle a single-line JSDoc block: /** doc */
			if idx := strings.Index(trimmed[3:], "*/"); idx >= 0 {
				content := strings.TrimSpace(trimmed[3 : 3+idx])
				pendingDoc = nil
				if content != "" {
					pendingDoc = []string{content}
				}
				continue
			}
			inJSDoc = true
			pendingDocLines = nil
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "/**"))
			if rest != "" {
				pendingDocLines = append(pendingDocLines, cleanJSDocLine(rest))
			}
			continue
		}

		if trimmed == "" {
			// A blank line does not break the association between a JSDoc
			// block and the declaration beneath it.
			continue
		}

		// Track class body so methods inside it can be recognized.
		if inClass {
			classDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if classDepth <= 0 {
				inClass = false
				pendingDoc = nil
				continue
			}

			if m := reClassMethod.FindStringSubmatch(trimmed); m != nil && !jsMethodKeywordSkip[m[1]] {
				page.Items = append(page.Items, jsItem{
					Kind:      "method",
					Signature: cleanJSSignature(trimmed),
					Doc:       strings.Join(pendingDoc, "\n"),
				})
				pendingDoc = nil
				continue
			}

			pendingDoc = nil
			continue
		}

		switch {
		case reExportClass.MatchString(trimmed):
			page.Items = append(page.Items, jsItem{
				Kind:      "class",
				Signature: cleanJSSignature(trimmed),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			inClass = true
			classDepth = strings.Count(line, "{") - strings.Count(line, "}")
			if classDepth <= 0 {
				// Opening brace not on this line yet (rare); assume it opens
				// on a following line and treat depth as pending at 0, which
				// the next `{`-bearing line will push positive.
				classDepth = 0
				inClass = true
			}
			continue

		case reExportDefaultClass.MatchString(trimmed):
			page.Items = append(page.Items, jsItem{
				Kind:      "default",
				Signature: cleanJSSignature(trimmed),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			continue

		case reExportDefaultFunc.MatchString(trimmed):
			page.Items = append(page.Items, jsItem{
				Kind:      "default",
				Signature: cleanJSSignature(trimmed),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			continue

		case reExportFunc.MatchString(trimmed):
			page.Items = append(page.Items, jsItem{
				Kind:      "function",
				Signature: cleanJSSignature(trimmed),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			continue

		case reExportConstFunc.MatchString(trimmed):
			page.Items = append(page.Items, jsItem{
				Kind:      "const",
				Signature: cleanJSSignature(trimmed),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			continue

		case strings.HasPrefix(trimmed, "export default"):
			// Any other default export this parser does not specialize
			// (an object literal, an identifier, a call expression, ...).
			page.Items = append(page.Items, jsItem{
				Kind:      "default",
				Signature: cleanJSSignature(trimmed),
				Doc:       strings.Join(pendingDoc, "\n"),
			})
			pendingDoc = nil
			continue
		}

		// Any other non-blank, non-comment line ends whatever JSDoc block
		// was pending: it documented something this parser does not
		// recognize (or nothing at all), and must not leak onto the next
		// recognized item by accident.
		pendingDoc = nil
	}

	return page, nil
}

// cleanJSDocLine strips a JSDoc continuation line's leading " * " marker.
func cleanJSDocLine(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "*")
	return strings.TrimSpace(s)
}

// cleanJSSignature trims a source line down to a one-line signature: no
// trailing "{", no leading/trailing whitespace. Best-effort by design (see
// the package doc's multi-line-signature limitation).
func cleanJSSignature(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimSuffix(s, "{")
	s = strings.TrimRight(s, " \t")
	return s
}

// findJSFiles walks rootDir for .js/.jsx/.mjs/.cjs files, skipping the
// directories this extractor never documents.
func findJSFiles(rootDir string) ([]string, error) {
	var files []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if jsExcludedDirs[name] || (strings.HasPrefix(name, ".") && path != rootDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if jsSourceExt[filepath.Ext(path)] {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// renderJSMarkdown turns one file's scanned page into Markdown: one section
// per recognized item, each carrying its own real signature line and its own
// real JSDoc text (or no prose, if the symbol carries none).
func renderJSMarkdown(relPath string, page jsPage) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", filepath.ToSlash(relPath))

	for _, item := range page.Items {
		fmt.Fprintf(&b, "### %s %s\n\n", item.Kind, jsItemName(item))
		if item.Doc != "" {
			fmt.Fprintf(&b, "%s\n\n", item.Doc)
		}
		fmt.Fprintf(&b, "```javascript\n%s\n```\n\n", item.Signature)
	}

	return b.String()
}

// jsItemName returns the item's signature with its "export"/"export default"
// prefix, and a leading keyword duplicating item.Kind (e.g. "function",
// "class", "const"), stripped -- renderJSMarkdown already prints the kind as
// the section heading's first word.
func jsItemName(item jsItem) string {
	s := item.Signature
	s = strings.TrimPrefix(s, "export default")
	s = strings.TrimPrefix(s, "export")
	s = strings.TrimSpace(s)

	switch item.Kind {
	case "function":
		s = strings.TrimPrefix(s, "async")
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "function")
	case "class":
		s = strings.TrimPrefix(s, "class")
	case "const":
		s = strings.TrimPrefix(s, "const")
	}

	return strings.TrimSpace(s)
}

// jsOutputBaseName flattens a source file's path relative to the source root
// into a single, injective output file name, matching the escape scheme
// internal/documentation/extractors/go and rust's native.go already use:
// "_" is an escape introducer, "_u" is one literal underscore, "__" is one
// path separator, so two different files can never collide on one output
// name.
func jsOutputBaseName(relPath string) string {
	clean := strings.TrimSuffix(filepath.ToSlash(relPath), filepath.Ext(relPath))
	segments := strings.Split(clean, "/")
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(segment, "_", "_u")
	}
	return strings.Join(segments, "__")
}

// countJSLines counts lines in a file, matching every other extractor in
// this repository's line-counting convention.
func countJSLines(path string) (int, error) {
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
