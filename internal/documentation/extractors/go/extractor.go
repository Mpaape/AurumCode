package goextractor

import (
	"context"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

// commandRunner is a minimal, structural stand-in for
// internal/documentation/site.CommandRunner's single method. This package
// deliberately does not import the "site" package at all: standard-library
// extraction (below) has no external tool to run, so there is nothing to hand
// a runner to. The type only exists so NewGoExtractor keeps accepting the same
// argument cmd/regenerate-docs/main.go already passes it (a site.CommandRunner
// value). Go interface satisfaction is structural, so that value assigns here
// without either package needing to import the other's type by name.
type commandRunner interface {
	Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error)
}

// GoExtractor extracts documentation from Go source code using only the Go
// standard library: go/parser reads the source, go/doc associates package,
// type and function doc comments with the declarations they document exactly
// the way "go doc" does, and go/printer renders each declaration's signature.
// There is no external binary, no subprocess and no network access, so this
// extractor behaves identically on a developer machine and inside a
// network-denied, tool-less sandbox.
//
// Earlier this extractor shelled out to the third-party gomarkdoc binary. That
// binary is not installed in the offline acceptance sandbox: the resulting
// exec.ErrNotFound was reported as extractors.ToolUnavailableError, which the
// pipeline treats as "skip this language" rather than a failure. A Go project
// therefore documented with zero pages and exit 0, indistinguishable from a
// clean run that had nothing to document.
type GoExtractor struct {
	// runner is retained only so the constructor signature below does not
	// have to change at its call site; the standard-library implementation
	// never calls it.
	runner          commandRunner
	incrementalMode bool
}

// NewGoExtractor creates a new Go documentation extractor.
func NewGoExtractor(runner commandRunner) *GoExtractor {
	return &GoExtractor{
		runner:          runner,
		incrementalMode: false,
	}
}

// WithIncrementalMode enables or disables incremental generation. It is off by
// default: with it on, a consumer that commits its generated docs directory gets an
// empty run, because every package looks up to date.
func (g *GoExtractor) WithIncrementalMode(enabled bool) *GoExtractor {
	g.incrementalMode = enabled
	return g
}

// Extract generates documentation from Go source code
func (g *GoExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	// Validate request
	if req.Language != extractors.LanguageGo {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageGo, req.Language)
	}

	// Validate source directory
	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find all Go packages
	packages, err := g.findGoPackages(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find Go packages: %w", err)
	}

	if len(packages) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguageGo,
			Files:    []string{},
			Stats: extractors.ExtractionStats{
				FilesProcessed: 0,
				DocsGenerated:  0,
				LinesProcessed: 0,
			},
		}, nil
	}

	// Extract documentation for each package
	result := &extractors.ExtractResult{
		Language: extractors.LanguageGo,
		Files:    []string{},
		Errors:   []error{},
	}

	for _, pkg := range packages {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Generate output path
		relPath, err := filepath.Rel(req.SourceDir, pkg)
		if err != nil {
			relPath = filepath.Base(pkg)
		}

		// Clean up the relative path for output filename
		outputName := strings.ReplaceAll(relPath, string(filepath.Separator), "_")
		if outputName == "." || outputName == "" {
			outputName = "root"
		}
		outputPath := filepath.Join(req.OutputDir, outputName+".md")

		// Check if we should skip this package (incremental mode)
		if g.incrementalMode && g.shouldSkipPackage(pkg, outputPath) {
			result.Stats.FilesProcessed++
			continue
		}

		// Extract documentation for this package
		err = g.extractPackage(pkg, outputPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("package %s: %w", pkg, err))
			continue
		}

		// Count lines in generated file
		lines, _ := g.countLines(outputPath)

		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
		result.Stats.LinesProcessed += lines
	}

	return result, nil
}

// Validate reports whether this extractor's dependencies are available. The
// standard-library implementation has none: go/parser, go/doc and go/printer
// are compiled into this binary, so there is nothing on the host to look up
// and nothing that can be "not found" the way an external tool could be. This
// always returning nil is itself the fix for AUR-424: the previous version
// looked up the gomarkdoc binary here, and a missing binary made the pipeline
// skip Go silently.
func (g *GoExtractor) Validate(ctx context.Context) error {
	return nil
}

// Language returns the language this extractor handles
func (g *GoExtractor) Language() extractors.Language {
	return extractors.LanguageGo
}

// findGoPackages finds all Go packages in the source directory
func (g *GoExtractor) findGoPackages(rootDir string) ([]string, error) {
	packages := []string{}
	visited := make(map[string]bool)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip non-directories
		if !info.IsDir() {
			return nil
		}

		// Skip common excluded directories
		dirName := filepath.Base(path)
		if dirName == "vendor" || dirName == "node_modules" || dirName == ".git" ||
			dirName == "testdata" || dirName == ".taskmaster" || strings.HasPrefix(dirName, ".") {
			return filepath.SkipDir
		}

		// Check if directory contains Go files
		hasGoFiles, err := g.hasGoFiles(path)
		if err != nil {
			return nil // Skip errors
		}

		if hasGoFiles && !visited[path] {
			packages = append(packages, path)
			visited[path] = true
		}

		return nil
	})

	return packages, err
}

// hasGoFiles checks if a directory contains any .go files (excluding tests)
func (g *GoExtractor) hasGoFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check for .go files but exclude test files
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true, nil
		}
	}

	return false, nil
}

// shouldSkipPackage determines if a package should be skipped in incremental mode
func (g *GoExtractor) shouldSkipPackage(pkgPath, outputPath string) bool {
	// Check if output file exists
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		return false // Output doesn't exist, must generate
	}

	// Check if any source file is newer than output
	entries, err := os.ReadDir(pkgPath)
	if err != nil {
		return false // Error reading directory, regenerate to be safe
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// If source file is newer than output, must regenerate
		if info.ModTime().After(outputInfo.ModTime()) {
			return false
		}
	}

	// All source files are older than output, can skip
	return true
}

// nonTestGoFile is the parser.ParseDir filter: every .go file except _test.go
// ones. Test files document nothing a consumer of the package can call.
func nonTestGoFile(info os.FileInfo) bool {
	name := info.Name()
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// selectPackage picks the package to document out of everything go/parser
// found in a directory. A well-formed Go directory holds exactly one
// non-"_test" package; parser.ParseDir legitimately returns a second entry
// named "<pkg>_test" for files that declare an external test package, and
// that entry documents nothing a caller of the real package can use, so it is
// always skipped. Ties are broken by name for a deterministic result.
func selectPackage(pkgs map[string]*ast.Package, pkgPath string) (*ast.Package, string, error) {
	names := make([]string, 0, len(pkgs))
	for name := range pkgs {
		names = append(names, name)
	}
	sort.Strings(names)

	var best *ast.Package
	var bestName string
	for _, name := range names {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		pkg := pkgs[name]
		if best == nil || len(pkg.Files) > len(best.Files) {
			best = pkg
			bestName = name
		}
	}

	if best == nil {
		return nil, "", fmt.Errorf("no documentable Go package found in %s", pkgPath)
	}

	return best, bestName, nil
}

// extractPackage parses every non-test .go file in pkgPath, builds a go/doc
// package view of it, renders that view as Markdown, and writes it to
// outputPath. No external process is started and no network call is made.
func (g *GoExtractor) extractPackage(pkgPath, outputPath string) error {
	fset := token.NewFileSet()

	astPkgs, err := parser.ParseDir(fset, pkgPath, nonTestGoFile, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", pkgPath, err)
	}

	astPkg, pkgName, err := selectPackage(astPkgs, pkgPath)
	if err != nil {
		return err
	}

	// Mode 0 documents exported declarations only, matching the conventional
	// "go doc" surface and the exported-API focus a rendered documentation
	// page is for.
	docPkg := doc.New(astPkg, pkgPath, 0)

	markdown := renderMarkdown(pkgName, docPkg, fset)

	if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	// A file on disk is not automatically real content: confirm it the same
	// way every other extractor in this package does.
	return extractors.ConfirmOutputFile("go/doc", outputPath)
}

// renderMarkdown turns a parsed go/doc.Package into Markdown: the package doc
// comment, then one section per exported constant/variable/type/function,
// each carrying its real declaration and its real doc comment. This is a
// small, deterministic renderer rather than a port of gomarkdoc's template
// language: the contract this card promises is "functions, types and comments
// show up", not byte-for-byte gomarkdoc output.
func renderMarkdown(pkgName string, docPkg *doc.Package, fset *token.FileSet) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Package %s\n\n", pkgName)
	if pkgDoc := strings.TrimSpace(docPkg.Doc); pkgDoc != "" {
		fmt.Fprintf(&b, "%s\n\n", pkgDoc)
	}

	writeValues(&b, "Constants", docPkg.Consts, fset)
	writeValues(&b, "Variables", docPkg.Vars, fset)

	if len(docPkg.Types) > 0 {
		b.WriteString("## Types\n\n")
		for _, t := range docPkg.Types {
			writeType(&b, t, fset)
		}
	}

	if len(docPkg.Funcs) > 0 {
		b.WriteString("## Functions\n\n")
		for _, f := range docPkg.Funcs {
			writeFunc(&b, f, fset)
		}
	}

	return b.String()
}

func writeValues(b *strings.Builder, heading string, values []*doc.Value, fset *token.FileSet) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", heading)
	for _, v := range values {
		fmt.Fprintf(b, "### %s\n\n", strings.Join(v.Names, ", "))
		if vDoc := strings.TrimSpace(v.Doc); vDoc != "" {
			fmt.Fprintf(b, "%s\n\n", vDoc)
		}
		fmt.Fprintf(b, "```go\n%s\n```\n\n", genDeclSignature(v.Decl, fset))
	}
}

func writeType(b *strings.Builder, t *doc.Type, fset *token.FileSet) {
	fmt.Fprintf(b, "### type %s\n\n", t.Name)
	if tDoc := strings.TrimSpace(t.Doc); tDoc != "" {
		fmt.Fprintf(b, "%s\n\n", tDoc)
	}
	fmt.Fprintf(b, "```go\n%s\n```\n\n", genDeclSignature(t.Decl, fset))

	for _, f := range t.Funcs {
		writeFunc(b, f, fset)
	}
	for _, m := range t.Methods {
		writeFunc(b, m, fset)
	}
}

func writeFunc(b *strings.Builder, f *doc.Func, fset *token.FileSet) {
	fmt.Fprintf(b, "#### func %s\n\n", f.Name)
	if fDoc := strings.TrimSpace(f.Doc); fDoc != "" {
		fmt.Fprintf(b, "%s\n\n", fDoc)
	}
	fmt.Fprintf(b, "```go\n%s\n```\n\n", funcSignature(f.Decl, fset))
}

// genDeclSignature renders a const/var/type declaration without repeating its
// doc comment (already printed as prose above the code block).
func genDeclSignature(decl *ast.GenDecl, fset *token.FileSet) string {
	if decl == nil {
		return ""
	}
	sigOnly := *decl
	sigOnly.Doc = nil
	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, &sigOnly); err != nil {
		return ""
	}
	return buf.String()
}

// funcSignature renders a function or method's signature line, omitting both
// its body and its doc comment: the body is implementation, not
// documentation, and the doc comment is already printed as prose above the
// code block. Go's printer supports a FuncDecl with a nil Body (that is how
// it renders an assembly-only declaration), so this prints exactly
// "func Name(params) results" with no trailing braces.
func funcSignature(decl *ast.FuncDecl, fset *token.FileSet) string {
	if decl == nil {
		return ""
	}
	sigOnly := *decl
	sigOnly.Body = nil
	sigOnly.Doc = nil
	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, &sigOnly); err != nil {
		return decl.Name.Name
	}
	return buf.String()
}

// countLines counts lines in a file
func (g *GoExtractor) countLines(path string) (int, error) {
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
