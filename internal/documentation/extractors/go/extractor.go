package goextractor

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
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

// documentedGOOS and documentedGOARCH pin the build target this extractor
// documents. They are deliberately constants rather than runtime.GOOS and
// runtime.GOARCH: AC-001 requires that repeating the run over the same input
// produces the same output, and a host-derived target would make the page for
// any package holding platform-specific files depend on the machine that
// generated it. A reader on arm64 Darwin and a reader on amd64 Linux must be
// looking at the same documented API surface.
const (
	documentedGOOS   = "linux"
	documentedGOARCH = "amd64"
)

// documentBuildContext returns the pinned go/build context used to decide
// which files of a directory belong to the documented package.
//
// Without this filter, a directory holding mutually exclusive platform files
// (say impl_linux.go and impl_windows.go, or two files guarded by opposing
// //go:build lines) has every one of them parsed into a single package, and
// the rendered page then advertises symbols that no single build of the
// package ever contains -- an API surface that does not exist. Selecting one
// fixed target instead means the page describes a real, buildable
// configuration.
//
// CgoEnabled is off and BuildTags is empty for the same reason the target is
// pinned: both otherwise come from the host environment (CGO_ENABLED, -tags),
// so leaving them at their inherited values would let the machine, not the
// source, decide what the documentation says.
func documentBuildContext() build.Context {
	buildContext := build.Default
	buildContext.GOOS = documentedGOOS
	buildContext.GOARCH = documentedGOARCH
	buildContext.CgoEnabled = false
	buildContext.BuildTags = nil
	buildContext.UseAllFiles = false
	return buildContext
}

// matchedGoFiles returns the names of the .go files in dir that both belong to
// the documented build target and carry documentable (non-test) source, sorted
// by name so every later step sees one fixed order.
func matchedGoFiles(buildContext *build.Context, dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	matched := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		// MatchFile applies both halves of Go's own file-selection rule: the
		// //go:build (and legacy // +build) constraints inside the file, and
		// the _GOOS/_GOARCH suffix convention in its name.
		ok, matchErr := buildContext.MatchFile(dir, name)
		if matchErr != nil {
			return nil, fmt.Errorf("build constraints for %s: %w", filepath.Join(dir, name), matchErr)
		}
		if ok {
			matched = append(matched, name)
		}
	}

	sort.Strings(matched)
	return matched, nil
}

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

		// Flatten the package's relative path into one output file name.
		outputPath := filepath.Join(req.OutputDir, outputBaseName(relPath)+".md")

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

// rootOutputBaseName is the file name given to the package that sits at the
// source root, which has no path segments of its own to name it after.
const rootOutputBaseName = "root"

// outputBaseName flattens a package's path relative to the source root into a
// single output file name, injectively: two different packages can never be
// given the same name, so one page can never silently overwrite another.
//
// The previous encoding replaced every separator with "_" and stopped there,
// which is not injective: the directories "a/b" and "a_b" both became
// "a_b.md", and whichever package was written second destroyed the first
// package's page with no error and no warning anywhere in the result.
//
// The encoding below reserves "_" as an escape introducer, so every "_" in
// the output is the first byte of a two-byte sequence:
//
//	"_u" is one literal underscore inside a path segment
//	"__" is one path separator
//
// Because "u" and "_" differ, that mapping is decodable left to right, hence
// injective. Names with neither separator nor underscore -- the overwhelmingly
// common case, e.g. "ledger" -- pass through untouched.
//
// The root package is the one path with no segments, so it needs a name of
// its own. "root" would collide with a real top-level directory called
// "root", so the encoder bumps that one real path by appending "__": an empty
// trailing segment, which filepath.Rel never produces, so nothing else can
// encode to it.
func outputBaseName(relPath string) string {
	if relPath == "." || relPath == "" {
		return rootOutputBaseName
	}

	segments := strings.Split(filepath.ToSlash(relPath), "/")
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(segment, "_", "_u")
	}

	encoded := strings.Join(segments, "__")
	if encoded == rootOutputBaseName {
		encoded += "__"
	}
	return encoded
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

// hasGoFiles reports whether a directory holds documentable Go source: at
// least one non-test .go file that belongs to the documented build target. A
// directory whose only Go files are excluded by their build constraints (a
// Windows-only helper directory documented on the pinned Linux target, say)
// contributes no package for this target and is therefore not one, which is
// what keeps extractPackage from having to fail on it later.
func (g *GoExtractor) hasGoFiles(dir string) (bool, error) {
	buildContext := documentBuildContext()
	matched, err := matchedGoFiles(&buildContext, dir)
	if err != nil {
		return false, err
	}

	return len(matched) > 0, nil
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

// selectPackage picks the package to document out of the files parsed from a
// directory, grouped by the package clause each file declares. A well-formed
// Go directory holds exactly one non-"_test" package; an entry named
// "<pkg>_test" documents nothing a caller of the real package can use, so it
// is always skipped. Files are visited in sorted name order, so the result
// does not depend on map iteration.
func selectPackage(pkgs map[string][]*ast.File, pkgPath string) ([]*ast.File, string, error) {
	names := make([]string, 0, len(pkgs))
	for name := range pkgs {
		names = append(names, name)
	}
	sort.Strings(names)

	var best []*ast.File
	var bestName string
	for _, name := range names {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		files := pkgs[name]
		if best == nil || len(files) > len(best) {
			best = files
			bestName = name
		}
	}

	if best == nil {
		return nil, "", fmt.Errorf("no documentable Go package found in %s", pkgPath)
	}

	return best, bestName, nil
}

// extractPackage parses the non-test .go files in pkgPath that belong to the
// documented build target, builds a go/doc package view of them, renders that
// view as Markdown, and writes it to outputPath. No external process is
// started and no network call is made.
//
// Files are selected through documentBuildContext rather than handed wholesale
// to the parser: see that function for why a page assembled from every file on
// disk describes a package that cannot be built.
func (g *GoExtractor) extractPackage(pkgPath, outputPath string) error {
	buildContext := documentBuildContext()
	names, err := matchedGoFiles(&buildContext, pkgPath)
	if err != nil {
		return fmt.Errorf("select files in %s: %w", pkgPath, err)
	}
	if len(names) == 0 {
		return fmt.Errorf("no Go file in %s builds for %s/%s",
			pkgPath, buildContext.GOOS, buildContext.GOARCH)
	}

	fset := token.NewFileSet()
	byPackage := make(map[string][]*ast.File)
	for _, name := range names {
		path := filepath.Join(pkgPath, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		pkgName := file.Name.Name
		byPackage[pkgName] = append(byPackage[pkgName], file)
	}

	files, pkgName, err := selectPackage(byPackage, pkgPath)
	if err != nil {
		return err
	}

	// The default mode documents exported declarations only, matching the
	// conventional "go doc" surface and the exported-API focus a rendered
	// documentation page is for.
	docPkg, err := doc.NewFromFiles(fset, files, pkgPath)
	if err != nil {
		return fmt.Errorf("document %s: %w", pkgPath, err)
	}

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
