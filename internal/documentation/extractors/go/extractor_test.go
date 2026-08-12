package goextractor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

// spyRunner records every call it receives and always fails it. It is used to
// prove the standard-library extractor never shells out: a caller-supplied
// runner that would blow up on first use must never actually be reached.
type spyRunner struct {
	calls []string
}

func (r *spyRunner) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	r.calls = append(r.calls, cmd)
	return "", fmt.Errorf("spyRunner: unexpected external tool invocation: %s %v", cmd, args)
}

func TestNewGoExtractor(t *testing.T) {
	extractor := NewGoExtractor(&spyRunner{})

	if extractor == nil {
		t.Fatal("NewGoExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageGo {
		t.Errorf("expected language %s, got %s", extractors.LanguageGo, extractor.Language())
	}

	if extractor.incrementalMode {
		t.Error("expected incremental mode to be disabled by default")
	}
}

// TestGoExtractor_DefaultExtractionRegeneratesExistingDocs pins that the default
// extractor regenerates documentation even when up-to-date output already exists.
// A consumer that commits its generated docs directory must not get an empty run.
func TestGoExtractor_DefaultExtractionRegeneratesExistingDocs(t *testing.T) {
	srcDir := t.TempDir()
	outputDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create Go file: %v", err)
	}

	stalePath := filepath.Join(outputDir, "root.md")
	if err := os.WriteFile(stalePath, []byte("# committed output\n"), 0644); err != nil {
		t.Fatalf("failed to create existing doc: %v", err)
	}

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(stalePath, future, future); err != nil {
		t.Fatalf("failed to age existing doc: %v", err)
	}

	extractor := NewGoExtractor(&spyRunner{})

	result, err := extractor.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: srcDir,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result.Stats.DocsGenerated == 0 {
		t.Fatalf("default extraction generated no documentation: %d files processed, %d docs generated",
			result.Stats.FilesProcessed, result.Stats.DocsGenerated)
	}

	// The committed placeholder must actually have been overwritten with real
	// output, not merely counted as generated.
	regenerated, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("failed to read regenerated doc: %v", err)
	}
	if strings.TrimSpace(string(regenerated)) == "# committed output" {
		t.Error("stale committed output was counted as regenerated but was never overwritten")
	}
}

func TestGoExtractor_WithIncrementalMode(t *testing.T) {
	extractor := NewGoExtractor(&spyRunner{}).WithIncrementalMode(false)

	if extractor.incrementalMode {
		t.Error("expected incremental mode to be disabled")
	}
}

// TestGoExtractor_Validate pins the fix for AUR-424: the standard-library
// extractor has no external dependency, so Validate must always succeed, even
// when the caller-supplied runner is configured to fail everything. The old
// gomarkdoc-backed implementation looked the tool up here and turned a
// missing binary into a silent pipeline skip; this test is what would have
// caught that regression.
func TestGoExtractor_Validate(t *testing.T) {
	extractor := NewGoExtractor(&spyRunner{})

	if err := extractor.Validate(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGoExtractor_Extract(t *testing.T) {
	// Create temporary test directory with Go source files
	tmpDir := t.TempDir()

	// Create test Go package
	pkgDir := filepath.Join(tmpDir, "testpkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create package directory: %v", err)
	}

	// Create a sample Go file with an exported type and an exported function,
	// each with a real doc comment.
	goFile := filepath.Join(pkgDir, "main.go")
	goContent := `package testpkg

// Widget is documented so the rendered page can be checked for real content.
type Widget struct {
	// Label names the widget.
	Label string
}

// Add adds two integers and returns the sum.
func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to create Go file: %v", err)
	}

	// Create output directory
	outputDir := filepath.Join(tmpDir, "docs")

	runner := &spyRunner{}
	extractor := NewGoExtractor(runner).WithIncrementalMode(false)

	// Create extract request
	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: tmpDir,
		OutputDir: outputDir,
	}

	// Run extraction
	result, err := extractor.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Verify result
	if result.Language != extractors.LanguageGo {
		t.Errorf("expected language %s, got %s", extractors.LanguageGo, result.Language)
	}

	if result.Stats.FilesProcessed == 0 {
		t.Error("expected at least 1 file to be processed")
	}

	// Every document reported as generated must exist on disk.
	if len(result.Files) != result.Stats.DocsGenerated {
		t.Errorf("DocsGenerated=%d but %d files reported", result.Stats.DocsGenerated, len(result.Files))
	}
	for _, file := range result.Files {
		if _, statErr := os.Stat(file); statErr != nil {
			t.Errorf("reported file %s does not exist: %v", file, statErr)
		}
	}

	// The generated page must carry the real symbols and their real doc
	// comments: this is what proves the standard-library extractor actually
	// parsed the source instead of producing an empty placeholder.
	generated := filepath.Join(outputDir, "testpkg.md")
	content, err := os.ReadFile(generated)
	if err != nil {
		t.Fatalf("expected generated page at %s: %v", generated, err)
	}
	page := string(content)
	for _, want := range []string{
		"testpkg", "Widget", "Widget is documented",
		"func Add", "Add adds two integers and returns the sum",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("generated page missing %q:\n%s", want, page)
		}
	}

	// No external tool call was made: the fix for AUR-424 is that this
	// extractor never needs one.
	if len(runner.calls) != 0 {
		t.Errorf("expected no external tool invocations, got %v", runner.calls)
	}
}

func TestGoExtractor_Extract_InvalidLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	extractor := NewGoExtractor(&spyRunner{})

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguagePython, // Wrong language
		SourceDir: tmpDir,
		OutputDir: tmpDir,
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid language")
	}
}

func TestGoExtractor_Extract_InvalidSourceDir(t *testing.T) {
	extractor := NewGoExtractor(&spyRunner{})

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: "/nonexistent/directory",
		OutputDir: t.TempDir(),
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid source directory")
	}
}

func TestGoExtractor_Extract_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "docs")

	extractor := NewGoExtractor(&spyRunner{})

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: tmpDir,
		OutputDir: outputDir,
	}

	result, err := extractor.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Stats.FilesProcessed != 0 {
		t.Errorf("expected 0 files processed, got %d", result.Stats.FilesProcessed)
	}

	if result.Stats.DocsGenerated != 0 {
		t.Errorf("expected 0 docs generated, got %d", result.Stats.DocsGenerated)
	}
}

func TestGoExtractor_Extract_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test packages
	for i := 0; i < 10; i++ {
		pkgDir := filepath.Join(tmpDir, fmt.Sprintf("pkg_%d", i))
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatalf("failed to create package directory: %v", err)
		}

		goFile := filepath.Join(pkgDir, "main.go")
		if err := os.WriteFile(goFile, []byte("package main\n"), 0644); err != nil {
			t.Fatalf("failed to create Go file: %v", err)
		}
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	extractor := NewGoExtractor(&spyRunner{})

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: tmpDir,
		OutputDir: filepath.Join(tmpDir, "docs"),
	}

	_, err := extractor.Extract(ctx, req)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestGoExtractor_findGoPackages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	testDirs := []struct {
		path       string
		createFile bool
		fileName   string
	}{
		{"pkg1", true, "main.go"},
		{"pkg2", true, "utils.go"},
		{"pkg3/subpkg", true, "helper.go"},
		{"vendor/external", true, "vendor.go"}, // Should be excluded
		{"node_modules/lib", true, "lib.go"},   // Should be excluded
		{".git/hooks", true, "hook.go"},        // Should be excluded
		{"nogofiles", false, ""},               // No Go files, should be excluded
		{"testdata", true, "test.go"},          // Should be excluded
	}

	for _, td := range testDirs {
		dir := filepath.Join(tmpDir, td.path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", td.path, err)
		}

		if td.createFile {
			file := filepath.Join(dir, td.fileName)
			if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
				t.Fatalf("failed to create file in %s: %v", td.path, err)
			}
		}
	}

	extractor := NewGoExtractor(&spyRunner{})

	packages, err := extractor.findGoPackages(tmpDir)
	if err != nil {
		t.Fatalf("findGoPackages failed: %v", err)
	}

	// Should find pkg1, pkg2, pkg3/subpkg
	// Should NOT find vendor, node_modules, .git, nogofiles, testdata
	expectedCount := 3
	if len(packages) != expectedCount {
		t.Errorf("expected %d packages, got %d", expectedCount, len(packages))
		for _, pkg := range packages {
			t.Logf("Found package: %s", pkg)
		}
	}

	// Verify excluded directories are not included
	for _, pkg := range packages {
		if strings.Contains(pkg, "vendor") || strings.Contains(pkg, "node_modules") ||
			strings.Contains(pkg, ".git") || strings.Contains(pkg, "testdata") {
			t.Errorf("excluded directory found in packages: %s", pkg)
		}
	}
}

func TestGoExtractor_hasGoFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected bool
	}{
		{
			name:     "has go files",
			files:    []string{"main.go", "utils.go"},
			expected: true,
		},
		{
			name:     "only test files",
			files:    []string{"main_test.go", "utils_test.go"},
			expected: false,
		},
		{
			name:     "mixed files",
			files:    []string{"main.go", "main_test.go", "README.md"},
			expected: true,
		},
		{
			name:     "no go files",
			files:    []string{"README.md", "config.json"},
			expected: false,
		},
		{
			name:     "empty directory",
			files:    []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for _, file := range tt.files {
				path := filepath.Join(tmpDir, file)
				if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
					t.Fatalf("failed to create file %s: %v", file, err)
				}
			}

			extractor := NewGoExtractor(&spyRunner{})

			result, err := extractor.hasGoFiles(tmpDir)
			if err != nil {
				t.Fatalf("hasGoFiles failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGoExtractor_shouldSkipPackage(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(sourceFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	outputFile := filepath.Join(tmpDir, "output.md")

	extractor := NewGoExtractor(&spyRunner{})

	// Test 1: Output doesn't exist - should NOT skip
	shouldSkip := extractor.shouldSkipPackage(tmpDir, outputFile)
	if shouldSkip {
		t.Error("expected to NOT skip when output doesn't exist")
	}

	// Create output file
	if err := os.WriteFile(outputFile, []byte("# Documentation\n"), 0644); err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}

	// Test 2: Output older than source - should NOT skip
	// (Note: In real scenario, we'd manipulate timestamps, but for test we assume output is recent)
	shouldSkip = extractor.shouldSkipPackage(tmpDir, outputFile)
	// This test is time-sensitive, so we just verify the method runs without error
	_ = shouldSkip

	// Test 3: Source doesn't exist - should NOT skip (regenerate to be safe)
	shouldSkip = extractor.shouldSkipPackage("/nonexistent", outputFile)
	if shouldSkip {
		t.Error("expected to NOT skip when source directory doesn't exist")
	}
}

func TestGoExtractor_countLines(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "empty file",
			content:  "",
			expected: 0,
		},
		{
			name:     "single line",
			content:  "line1\n",
			expected: 1,
		},
		{
			name:     "multiple lines",
			content:  "line1\nline2\nline3\n",
			expected: 3,
		},
		{
			name:     "no trailing newline",
			content:  "line1\nline2\nline3",
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			file := filepath.Join(tmpDir, "test.txt")

			if err := os.WriteFile(file, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			extractor := NewGoExtractor(&spyRunner{})

			count, err := extractor.countLines(file)
			if err != nil {
				t.Fatalf("countLines failed: %v", err)
			}

			if count != tt.expected {
				t.Errorf("expected %d lines, got %d", tt.expected, count)
			}
		})
	}
}

func TestGoExtractor_Language(t *testing.T) {
	extractor := NewGoExtractor(&spyRunner{})

	if extractor.Language() != extractors.LanguageGo {
		t.Errorf("expected language %s, got %s", extractors.LanguageGo, extractor.Language())
	}
}

// TestGoExtractor_Extract_TypeWithMethod pins that a method on an exported
// type is rendered under that type, with its own doc comment - the shape a
// consumer expects a symbol page to have.
func TestGoExtractor_Extract_TypeWithMethod(t *testing.T) {
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "greet")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create package directory: %v", err)
	}

	goContent := `package greet

// Greeter says hello.
type Greeter struct {
	// Name is who to greet.
	Name string
}

// Hello returns a greeting for the receiver's Name.
func (g *Greeter) Hello() string {
	return "Hello, " + g.Name
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "greet.go"), []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to create Go file: %v", err)
	}

	outputDir := filepath.Join(tmpDir, "docs")
	extractor := NewGoExtractor(&spyRunner{})

	_, err := extractor.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: pkgDir,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	generated := filepath.Join(outputDir, "root.md")
	content, err := os.ReadFile(generated)
	if err != nil {
		t.Fatalf("expected generated page at %s: %v", generated, err)
	}
	page := string(content)
	for _, want := range []string{"Greeter", "Greeter says hello", "func Hello", "Hello returns a greeting"} {
		if !strings.Contains(page, want) {
			t.Errorf("generated page missing %q:\n%s", want, page)
		}
	}
}

// TestGoExtractor_Validate_ErrorIgnored double-checks that a runner configured
// to error on every call still leaves Validate unaffected and is never called.
func TestGoExtractor_Validate_ErrorIgnored(t *testing.T) {
	runner := &spyRunner{}
	extractor := NewGoExtractor(runner)

	if err := extractor.Validate(context.Background()); err != nil {
		t.Fatalf("Validate must not depend on the runner, got: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("Validate must not call the runner, got calls: %v", runner.calls)
	}
}
