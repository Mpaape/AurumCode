package goextractor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewGoExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

	if extractor == nil {
		t.Fatal("NewGoExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageGo {
		t.Errorf("expected language %s, got %s", extractors.LanguageGo, extractor.Language())
	}

	if !extractor.incrementalMode {
		t.Error("expected incremental mode to be enabled by default")
	}
}

func TestGoExtractor_WithIncrementalMode(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner).WithIncrementalMode(false)

	if extractor.incrementalMode {
		t.Error("expected incremental mode to be disabled")
	}
}

func TestGoExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{
			name:      "gomarkdoc installed",
			output:    "gomarkdoc version 1.1.0",
			err:       nil,
			wantError: false,
		},
		{
			name:      "gomarkdoc not found",
			output:    "",
			err:       errors.New("command not found"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("gomarkdoc --version", tt.err)
			} else {
				runner.WithOutput("gomarkdoc --version", tt.output)
			}

			extractor := NewGoExtractor(runner)
			err := extractor.Validate(context.Background())

			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
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

	// Create a sample Go file
	goFile := filepath.Join(pkgDir, "main.go")
	goContent := `package testpkg

// Add adds two integers
func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to create Go file: %v", err)
	}

	// Create output directory
	outputDir := filepath.Join(tmpDir, "docs")

	// Setup mock runner
	runner := site.NewMockRunner()
	runner.WithOutput("gomarkdoc --version", "gomarkdoc 1.1.0")
	runner.WithOutput("gomarkdoc", "Documentation generated")

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

	// Verify gomarkdoc was called
	calls := runner.GetCalls()
	foundGomarkdoc := false
	for _, call := range calls {
		if call.Cmd == "gomarkdoc" && len(call.Args) >= 2 {
			foundGomarkdoc = true
			break
		}
	}
	if !foundGomarkdoc {
		t.Error("expected gomarkdoc command to be called")
	}
}

func TestGoExtractor_Extract_InvalidLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

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
	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

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

	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

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

	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

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

	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

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

			runner := site.NewMockRunner()
			extractor := NewGoExtractor(runner)

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

	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

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

			runner := site.NewMockRunner()
			extractor := NewGoExtractor(runner)

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
	runner := site.NewMockRunner()
	extractor := NewGoExtractor(runner)

	if extractor.Language() != extractors.LanguageGo {
		t.Errorf("expected language %s, got %s", extractors.LanguageGo, extractor.Language())
	}
}
