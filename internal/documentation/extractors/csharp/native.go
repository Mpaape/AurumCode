package csharp

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

// NativeExtractor extracts documentation from C# source by reading `///` XML
// doc comments directly out of the source text with a line-oriented scanner
// and the standard library's encoding/xml. It starts no subprocess and
// reaches no network, so it never depends on `dotnet` or `xmldocmd` being
// installed.
//
// This exists because the CSharpExtractor above cannot run in this project's
// offline acceptance sandbox: neither `dotnet` nor `xmldocmd` is installed
// there, and even where both are, `dotnet build` runs MSBuild, which executes
// <Exec> tasks, <UsingTask> assemblies and imported .targets/.props from the
// repository (see CSharpExtractor's ExecutesRepositoryCode). AUR-427 picked
// outcome (a) of its own "Restricao medida" for both languages: a native,
// tokenizer-based parser that accepts partial coverage rather than pinning a
// new image or leaving C# permanently undocumented. This never touches
// CSharpExtractor, whose contract the pre-existing tool_unavailable_test.go /
// tool_failure_test.go tables still pin.
//
// COVERAGE (declared honestly; see docs/specs/AUR-427.md for the full table):
//
// Recognized, with their `///` XML doc comment attached:
//   - `public class`, `public interface`, `public struct`, `public enum`,
//     `public record` (with any combination of `sealed`, `abstract`,
//     `static`, `partial`, `readonly`, `unsafe` modifiers before the keyword)
//   - `public` methods, matched by "public [modifiers] ReturnType Name(" --
//     including constructors ("public ClassName(")
//   - `public` properties with an explicit `{ get` accessor on the same
//     source line as the declaration
//   - the standard XML doc elements `<summary>`, `<param name="...">`,
//     `<returns>` and `<remarks>`; unrecognized elements inside the comment
//     are ignored rather than rejected
//
// NOT recognized -- absent from the generated page rather than misreported:
//   - `/** ... */` block doc comments (only `///` line comments are read)
//   - fields, events, indexers, operators, and explicit interface
//     implementations
//   - expression-bodied members (`public int X => 1;`) and auto-properties
//     whose accessor is not literally `{ get` on the declaration's own line
//     (e.g. `{ get; set; }` split across lines, or `=>`-bodied properties)
//   - generic type/method declarations whose signature wraps past the
//     declaration's own source line
//   - a `///` comment whose content is not well-formed XML: the parser falls
//     back to the raw comment text (still shown on the page, but not broken
//     into Summary/Parameters/Returns/Remarks) rather than dropping it
type NativeExtractor struct{}

// NewNativeExtractor creates a C# documentation extractor that reads `///`
// XML doc comments directly from source text. It has no external dependency:
// Validate always succeeds and Extract never starts a subprocess.
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
	return extractors.LanguageCSharp
}

// csharpExcludedDirs mirrors the build output directories the tool-based
// CSharpExtractor already skips, so the native parser never reads compiler
// output or restored packages as if they were the documented project's own
// source.
var csharpExcludedDirs = map[string]bool{
	"bin": true, "obj": true, "node_modules": true, ".git": true,
	".taskmaster": true,
}

// Extract generates documentation from C# source code.
func (n *NativeExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageCSharp {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageCSharp, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	files, err := findCSharpSourceFiles(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find C# source files: %w", err)
	}

	result := &extractors.ExtractResult{
		Language: extractors.LanguageCSharp,
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

		items, parseErr := scanCSharpFile(srcPath)
		if parseErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parse %s: %w", srcPath, parseErr))
			continue
		}

		if len(items) == 0 {
			// No public member this parser recognizes: a page with nothing on
			// it would be an empty page masquerading as documentation.
			continue
		}

		relPath, relErr := filepath.Rel(req.SourceDir, srcPath)
		if relErr != nil {
			relPath = filepath.Base(srcPath)
		}
		outputPath := filepath.Join(req.OutputDir, outputBaseName(relPath)+".md")

		markdown := renderCSharpMarkdown(relPath, items)
		if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write %s: %w", outputPath, err))
			continue
		}

		if err := extractors.ConfirmOutputFile("csharp doc-comment parser", outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		lines, _ := countCSharpLines(srcPath)
		result.Files = append(result.Files, outputPath)
		result.Stats.DocsGenerated++
		result.Stats.LinesProcessed += lines
	}

	return result, nil
}

// csharpDoc is a parsed `///` XML doc comment.
type csharpDoc struct {
	// Raw is the doc comment's original text, kept even when it parses
	// cleanly as XML, so a fallback rendering never has to re-derive it.
	Raw string
	// Parsed is true when Raw parsed as well-formed XML; when false, Summary,
	// Params and Returns/Remarks are all empty and only Raw is meaningful.
	Parsed  bool
	Summary string
	Params  []csharpParam
	Returns string
	Remarks string
}

type csharpParam struct {
	Name string
	Text string
}

// csharpXMLDoc is the encoding/xml target for one wrapped `///` comment body.
// Unknown elements are simply absent from the struct, so they are ignored
// rather than rejected (encoding/xml drops unmapped elements by default).
type csharpXMLDoc struct {
	Summary string `xml:"summary"`
	Returns string `xml:"returns"`
	Remarks string `xml:"remarks"`
	Params  []struct {
		Name string `xml:"name,attr"`
		Text string `xml:",chardata"`
	} `xml:"param"`
}

// parseCSharpDoc wraps raw `///` text (already stripped of its "///" markers)
// in a synthetic root element and parses it as XML, the same shape the C#
// compiler itself expects a doc comment's body to be. A doc comment that is
// not well-formed XML (a bare "Fixture." with no tags at all is common, and
// is not itself invalid -- it is just not wrapped in <summary>) still counts
// as extracted: it is kept as Raw and rendered verbatim, never silently
// dropped.
func parseCSharpDoc(raw string) csharpDoc {
	doc := csharpDoc{Raw: strings.TrimSpace(raw)}
	if doc.Raw == "" {
		return doc
	}

	var parsed csharpXMLDoc
	wrapped := "<doc>" + raw + "</doc>"
	if err := xml.Unmarshal([]byte(wrapped), &parsed); err != nil {
		return doc
	}

	doc.Parsed = true
	doc.Summary = strings.TrimSpace(parsed.Summary)
	doc.Returns = strings.TrimSpace(parsed.Returns)
	doc.Remarks = strings.TrimSpace(parsed.Remarks)
	for _, p := range parsed.Params {
		doc.Params = append(doc.Params, csharpParam{
			Name: p.Name,
			Text: strings.TrimSpace(p.Text),
		})
	}
	return doc
}

// csharpItem is one documented public member.
type csharpItem struct {
	// Kind is "class", "interface", "struct", "enum", "record", "constructor"
	// or "method"/"property".
	Kind      string
	Signature string
	Doc       csharpDoc
}

var (
	csharpTypePattern = regexp.MustCompile(
		`^\s*public\s+(?:(?:sealed|abstract|static|partial|readonly|unsafe)\s+)*(class|interface|struct|enum|record)\s+(\w+)`)

	// csharpMemberPattern matches a public method or constructor declaration.
	// Group 1 (the return type) is optional: a single bare token before "("
	// -- "public Widget(" with no second, whitespace-separated word -- has
	// nothing for the optional group to consume, so it is left empty and the
	// name alone lands in group 2. That absence, not name-matching against
	// the enclosing type, is what distinguishes a constructor from a method:
	// a constructor is the one declaration shape with no return type token
	// at all, so this needs no memory of the surrounding type's name.
	csharpMemberPattern = regexp.MustCompile(
		`^\s*public\s+(?:(?:static|virtual|override|abstract|sealed|async|unsafe|new|extern)\s+)*(?:([\w<>\[\],\.\?]+)\s+)?(\w+)\s*(?:<[^>]*>)?\s*\(`)

	csharpPropertyPattern = regexp.MustCompile(
		`^\s*public\s+(?:(?:static|virtual|override|abstract|sealed|readonly)\s+)*[\w<>\[\],\.\?]+\s+(\w+)\s*\{\s*get\b`)

	csharpAttributePattern = regexp.MustCompile(`^\s*\[`)
)

// scanCSharpFile reads one .cs file and extracts every `///`-documented
// public member this parser recognizes.
func scanCSharpFile(path string) ([]csharpItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []csharpItem
	var pendingDoc []string

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "///"):
			pendingDoc = append(pendingDoc, strings.TrimPrefix(trimmed, "///"))
			continue
		case trimmed == "":
			continue
		case csharpAttributePattern.MatchString(trimmed):
			continue
		}

		if m := csharpTypePattern.FindStringSubmatch(line); m != nil {
			items = append(items, csharpItem{
				Kind:      m[1],
				Signature: cleanCSharpSignature(line),
				Doc:       parseCSharpDoc(strings.Join(pendingDoc, "\n")),
			})
			pendingDoc = nil
			continue
		}

		if m := csharpPropertyPattern.FindStringSubmatch(line); m != nil {
			items = append(items, csharpItem{
				Kind:      "property",
				Signature: cleanCSharpSignature(line),
				Doc:       parseCSharpDoc(strings.Join(pendingDoc, "\n")),
			})
			pendingDoc = nil
			continue
		}

		if m := csharpMemberPattern.FindStringSubmatch(line); m != nil {
			kind := "method"
			if m[1] == "" {
				kind = "constructor"
			}
			items = append(items, csharpItem{
				Kind:      kind,
				Signature: cleanCSharpSignature(line),
				Doc:       parseCSharpDoc(strings.Join(pendingDoc, "\n")),
			})
			pendingDoc = nil
			continue
		}

		pendingDoc = nil
	}

	return items, nil
}

// cleanCSharpSignature trims a source line down to a one-line signature: no
// trailing "{", no leading/trailing whitespace.
func cleanCSharpSignature(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimSuffix(s, "{")
	s = strings.TrimRight(s, " \t")
	return s
}

// findCSharpSourceFiles walks rootDir for .cs files, skipping compiler output
// and restored-package directories.
func findCSharpSourceFiles(rootDir string) ([]string, error) {
	var files []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if csharpExcludedDirs[name] || (strings.HasPrefix(name, ".") && path != rootDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".cs") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// renderCSharpMarkdown turns one file's scanned items into Markdown.
func renderCSharpMarkdown(relPath string, items []csharpItem) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", filepath.ToSlash(relPath))

	for _, item := range items {
		fmt.Fprintf(&b, "### %s %s\n\n", item.Kind, item.Signature)
		writeCSharpDoc(&b, item.Doc)
		fmt.Fprintf(&b, "```csharp\n%s\n```\n\n", item.Signature)
	}

	return b.String()
}

func writeCSharpDoc(b *strings.Builder, doc csharpDoc) {
	if doc.Raw == "" {
		return
	}

	if !doc.Parsed {
		fmt.Fprintf(b, "%s\n\n", doc.Raw)
		return
	}

	if doc.Summary != "" {
		fmt.Fprintf(b, "%s\n\n", doc.Summary)
	}
	for _, p := range doc.Params {
		fmt.Fprintf(b, "- `%s`: %s\n", p.Name, p.Text)
	}
	if len(doc.Params) > 0 {
		b.WriteString("\n")
	}
	if doc.Returns != "" {
		fmt.Fprintf(b, "Returns: %s\n\n", doc.Returns)
	}
	if doc.Remarks != "" {
		fmt.Fprintf(b, "%s\n\n", doc.Remarks)
	}
}

// outputBaseName flattens a source file's path relative to the source root
// into a single, injective output file name -- the same escape scheme
// internal/documentation/extractors/go.outputBaseName and this card's own
// rust.outputBaseName use (AUR-424/AUR-427): "_" is an escape introducer,
// "_u" is one literal underscore, "__" is one path separator.
func outputBaseName(relPath string) string {
	clean := strings.TrimSuffix(filepath.ToSlash(relPath), ".cs")
	segments := strings.Split(clean, "/")
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(segment, "_", "_u")
	}
	return strings.Join(segments, "__")
}

// countCSharpLines counts lines in a file, matching every other extractor in
// this repository's line-counting convention.
func countCSharpLines(path string) (int, error) {
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
