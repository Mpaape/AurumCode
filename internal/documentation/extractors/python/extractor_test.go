package python

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewPythonExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	if extractor == nil {
		t.Fatal("NewPythonExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguagePython {
		t.Errorf("expected language %s, got %s", extractors.LanguagePython, extractor.Language())
	}
}

func TestPythonExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{
			name:      "pydoc-markdown installed",
			output:    "pydoc-markdown version 4.0.0",
			err:       nil,
			wantError: false,
		},
		{
			name:      "pydoc-markdown not found",
			output:    "",
			err:       errors.New("command not found"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("pydoc-markdown --version", tt.err)
			} else {
				runner.WithOutput("pydoc-markdown --version", tt.output)
			}

			extractor := NewPythonExtractor(runner)
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

func TestPythonExtractor_Extract(t *testing.T) {
	// Create temporary test directory with Python source files
	tmpDir := t.TempDir()

	// Create test Python module
	moduleDir := filepath.Join(tmpDir, "mymodule")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("failed to create module directory: %v", err)
	}

	// Create a sample Python file with docstring
	pyFile := filepath.Join(moduleDir, "utils.py")
	pyContent := `"""
This module provides utility functions.
"""

def add(a, b):
    """Add two numbers together.

    Args:
        a: First number
        b: Second number

    Returns:
        Sum of a and b
    """
    return a + b
`
	if err := os.WriteFile(pyFile, []byte(pyContent), 0644); err != nil {
		t.Fatalf("failed to create Python file: %v", err)
	}

	// Create output directory
	outputDir := filepath.Join(tmpDir, "docs")

	// Setup mock runner
	runner := site.NewMockRunner()
	runner.WithOutput("pydoc-markdown --version", "pydoc-markdown 4.0.0")

	extractor := NewPythonExtractor(runner)

	// Create extract request
	req := &extractors.ExtractRequest{
		Language:  extractors.LanguagePython,
		SourceDir: tmpDir,
		OutputDir: outputDir,
	}

	// Run extraction
	result, err := extractor.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Verify result
	if result.Language != extractors.LanguagePython {
		t.Errorf("expected language %s, got %s", extractors.LanguagePython, result.Language)
	}

	if result.Stats.FilesProcessed == 0 {
		t.Error("expected at least 1 file to be processed")
	}

	if result.Stats.DocsGenerated == 0 {
		t.Error("expected at least 1 doc to be generated")
	}

	// Verify output file was created
	if len(result.Files) == 0 {
		t.Error("expected at least one output file")
	}

	// Check that output file contains docstring content
	if len(result.Files) > 0 {
		content, err := os.ReadFile(result.Files[0])
		if err != nil {
			t.Errorf("failed to read output file: %v", err)
		} else {
			contentStr := string(content)
			if !strings.Contains(contentStr, "utility functions") {
				t.Error("output file should contain docstring content")
			}
		}
	}
}

func TestPythonExtractor_Extract_InvalidLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageGo, // Wrong language
		SourceDir: tmpDir,
		OutputDir: tmpDir,
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid language")
	}
}

func TestPythonExtractor_Extract_InvalidSourceDir(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguagePython,
		SourceDir: "/nonexistent/directory",
		OutputDir: t.TempDir(),
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid source directory")
	}
}

func TestPythonExtractor_Extract_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "docs")

	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguagePython,
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

func TestPythonExtractor_findPythonModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	testDirs := []struct {
		path       string
		createFile bool
		fileName   string
	}{
		{"module1", true, "main.py"},
		{"module2", true, "utils.py"},
		{"module3/submodule", true, "helper.py"},
		{"venv/lib", true, "package.py"},           // Should be excluded
		{".venv/lib", true, "package.py"},          // Should be excluded
		{"__pycache__", true, "cache.py"},          // Should be excluded
		{".git/hooks", true, "hook.py"},            // Should be excluded
		{"tests", true, "test_utils.py"},           // Should be excluded (test file)
		{"module4", true, "app_test.py"},           // Should be excluded (test file)
	}

	for _, td := range testDirs {
		dir := filepath.Join(tmpDir, td.path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", td.path, err)
		}

		if td.createFile {
			file := filepath.Join(dir, td.fileName)
			if err := os.WriteFile(file, []byte("# Python module\n"), 0644); err != nil {
				t.Fatalf("failed to create file in %s: %v", td.path, err)
			}
		}
	}

	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	modules, err := extractor.findPythonModules(tmpDir)
	if err != nil {
		t.Fatalf("findPythonModules failed: %v", err)
	}

	// Should find module1, module2, module3/submodule
	// Should NOT find venv, .venv, __pycache__, .git, test files
	expectedCount := 3
	if len(modules) != expectedCount {
		t.Errorf("expected %d modules, got %d", expectedCount, len(modules))
		for _, mod := range modules {
			t.Logf("Found module: %s", mod)
		}
	}

	// Verify excluded directories are not included
	for _, mod := range modules {
		if strings.Contains(mod, "venv") || strings.Contains(mod, "__pycache__") ||
			strings.Contains(mod, ".git") || strings.Contains(mod, "test_") ||
			strings.HasSuffix(mod, "_test.py") {
			t.Errorf("excluded file found in modules: %s", mod)
		}
	}
}

func TestPythonExtractor_getModuleName(t *testing.T) {
	tests := []struct {
		name       string
		modulePath string
		rootDir    string
		expected   string
	}{
		{
			name:       "simple module",
			modulePath: "/project/mymodule.py",
			rootDir:    "/project",
			expected:   "mymodule",
		},
		{
			name:       "nested module",
			modulePath: "/project/pkg/subpkg/module.py",
			rootDir:    "/project",
			expected:   "pkg.subpkg.module",
		},
		{
			name:       "__init__ module",
			modulePath: "/project/pkg/__init__.py",
			rootDir:    "/project",
			expected:   "pkg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			extractor := NewPythonExtractor(runner)

			result := extractor.getModuleName(tt.modulePath, tt.rootDir)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestPythonExtractor_extractDocstrings(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		contains []string
	}{
		{
			name: "module docstring",
			code: `"""This is a module docstring."""

def function():
    pass
`,
			contains: []string{"This is a module docstring"},
		},
		{
			name: "function docstring",
			code: `def add(a, b):
    """Add two numbers.

    Args:
        a: First number
        b: Second number
    """
    return a + b
`,
			contains: []string{"Add two numbers", "Args:", "First number"},
		},
		{
			name: "single quotes docstring",
			code: `'''Single quotes docstring'''

def test():
    pass
`,
			contains: []string{"Single quotes docstring"},
		},
		{
			name:     "no docstrings",
			code:     `def function():\n    pass`,
			contains: []string{"No documentation found"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			extractor := NewPythonExtractor(runner)

			result := extractor.extractDocstrings(tt.code)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("expected result to contain %q, got:\n%s", expected, result)
				}
			}
		})
	}
}

func TestPythonExtractor_countLines(t *testing.T) {
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
			extractor := NewPythonExtractor(runner)

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

func TestPythonExtractor_Language(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	if extractor.Language() != extractors.LanguagePython {
		t.Errorf("expected language %s, got %s", extractors.LanguagePython, extractor.Language())
	}
}

func TestPythonExtractor_Extract_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test modules
	for i := 0; i < 10; i++ {
		moduleDir := filepath.Join(tmpDir, "module"+string(rune(i)))
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("failed to create module directory: %v", err)
		}

		pyFile := filepath.Join(moduleDir, "main.py")
		if err := os.WriteFile(pyFile, []byte("\"\"\"Module docstring\"\"\"\n"), 0644); err != nil {
			t.Fatalf("failed to create Python file: %v", err)
		}
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := site.NewMockRunner()
	extractor := NewPythonExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguagePython,
		SourceDir: tmpDir,
		OutputDir: filepath.Join(tmpDir, "docs"),
	}

	_, err := extractor.Extract(ctx, req)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}
