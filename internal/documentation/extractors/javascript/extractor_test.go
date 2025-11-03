package javascript

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewJSExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewJSExtractor(runner)

	if extractor == nil {
		t.Fatal("NewJSExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageJavaScript {
		t.Errorf("expected language %s, got %s", extractors.LanguageJavaScript, extractor.Language())
	}
}

func TestJSExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{
			name:      "typedoc installed",
			output:    "TypeDoc 0.25.0",
			err:       nil,
			wantError: false,
		},
		{
			name:      "typedoc not found",
			output:    "",
			err:       errors.New("command not found"),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("typedoc --version", tt.err)
			} else {
				runner.WithOutput("typedoc --version", tt.output)
			}

			extractor := NewJSExtractor(runner)
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

func TestJSExtractor_detectProjectType(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected ProjectType
		wantErr  bool
	}{
		{
			name:     "TypeScript with tsconfig.json",
			files:    []string{"tsconfig.json", "src/index.ts"},
			expected: ProjectTypeTypeScript,
			wantErr:  false,
		},
		{
			name:     "JavaScript with package.json",
			files:    []string{"package.json", "src/index.js"},
			expected: ProjectTypeJavaScript,
			wantErr:  false,
		},
		{
			name:     "TypeScript without tsconfig",
			files:    []string{"src/index.ts", "src/utils.tsx"},
			expected: ProjectTypeTypeScript,
			wantErr:  false,
		},
		{
			name:     "JavaScript without package.json",
			files:    []string{"src/index.js", "src/utils.jsx"},
			expected: ProjectTypeJavaScript,
			wantErr:  false,
		},
		{
			name:     "no project files",
			files:    []string{"README.md", "LICENSE"},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create test files
			for _, file := range tt.files {
				fullPath := filepath.Join(tmpDir, file)
				dir := filepath.Dir(fullPath)

				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create directory %s: %v", dir, err)
				}

				var content []byte
				if filepath.Ext(file) == ".json" {
					content = []byte("{}")
				} else {
					content = []byte("content")
				}

				if err := os.WriteFile(fullPath, content, 0644); err != nil {
					t.Fatalf("failed to create file %s: %v", file, err)
				}
			}

			runner := site.NewMockRunner()
			extractor := NewJSExtractor(runner)

			result, err := extractor.detectProjectType(tmpDir)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("expected project type %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestJSExtractor_hasTypeScriptFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected bool
	}{
		{
			name:     "has .ts files",
			files:    []string{"index.ts", "utils.ts"},
			expected: true,
		},
		{
			name:     "has .tsx files",
			files:    []string{"App.tsx", "Component.tsx"},
			expected: true,
		},
		{
			name:     "mixed ts and tsx",
			files:    []string{"index.ts", "App.tsx"},
			expected: true,
		},
		{
			name:     "only js files",
			files:    []string{"index.js", "utils.js"},
			expected: false,
		},
		{
			name:     "no script files",
			files:    []string{"README.md", "config.json"},
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
			extractor := NewJSExtractor(runner)

			result, err := extractor.hasTypeScriptFiles(tmpDir)
			if err != nil {
				t.Fatalf("hasTypeScriptFiles failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestJSExtractor_hasJavaScriptFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected bool
	}{
		{
			name:     "has .js files",
			files:    []string{"index.js", "utils.js"},
			expected: true,
		},
		{
			name:     "has .jsx files",
			files:    []string{"App.jsx", "Component.jsx"},
			expected: true,
		},
		{
			name:     "has .mjs files",
			files:    []string{"module.mjs"},
			expected: true,
		},
		{
			name:     "has .cjs files",
			files:    []string{"common.cjs"},
			expected: true,
		},
		{
			name:     "only ts files",
			files:    []string{"index.ts", "utils.ts"},
			expected: false,
		},
		{
			name:     "no script files",
			files:    []string{"README.md", "config.json"},
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
			extractor := NewJSExtractor(runner)

			result, err := extractor.hasJavaScriptFiles(tmpDir)
			if err != nil {
				t.Fatalf("hasJavaScriptFiles failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestJSExtractor_findEntryPoint(t *testing.T) {
	tests := []struct {
		name        string
		projectType ProjectType
		dirs        []string
		files       []string
		expectedExt string // Extension to check (.json for tsconfig, empty for directory)
	}{
		{
			name:        "TypeScript with tsconfig.json",
			projectType: ProjectTypeTypeScript,
			files:       []string{"tsconfig.json"},
			expectedExt: ".json",
		},
		{
			name:        "TypeScript with src directory",
			projectType: ProjectTypeTypeScript,
			dirs:        []string{"src"},
			expectedExt: "",
		},
		{
			name:        "JavaScript with src directory",
			projectType: ProjectTypeJavaScript,
			dirs:        []string{"src"},
			expectedExt: "",
		},
		{
			name:        "Project with lib directory",
			projectType: ProjectTypeJavaScript,
			dirs:        []string{"lib"},
			expectedExt: "",
		},
		{
			name:        "Fallback to root",
			projectType: ProjectTypeJavaScript,
			files:       []string{"package.json"},
			expectedExt: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create directories
			for _, dir := range tt.dirs {
				dirPath := filepath.Join(tmpDir, dir)
				if err := os.MkdirAll(dirPath, 0755); err != nil {
					t.Fatalf("failed to create directory %s: %v", dir, err)
				}
			}

			// Create files
			for _, file := range tt.files {
				filePath := filepath.Join(tmpDir, file)
				if err := os.WriteFile(filePath, []byte("{}"), 0644); err != nil {
					t.Fatalf("failed to create file %s: %v", file, err)
				}
			}

			runner := site.NewMockRunner()
			extractor := NewJSExtractor(runner)

			entryPoint, err := extractor.findEntryPoint(tmpDir, tt.projectType)
			if err != nil {
				t.Fatalf("findEntryPoint failed: %v", err)
			}

			if tt.expectedExt != "" {
				// Check file extension
				if filepath.Ext(entryPoint) != tt.expectedExt {
					t.Errorf("expected extension %s, got %s", tt.expectedExt, filepath.Ext(entryPoint))
				}
			} else {
				// Should be a directory
				info, err := os.Stat(entryPoint)
				if err != nil {
					t.Fatalf("entry point doesn't exist: %v", err)
				}
				if !info.IsDir() && entryPoint != tmpDir {
					t.Error("expected entry point to be a directory or root")
				}
			}
		})
	}
}

func TestJSExtractor_Extract(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create TypeScript project structure
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src directory: %v", err)
	}

	// Create tsconfig.json
	tsconfigPath := filepath.Join(tmpDir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create tsconfig.json: %v", err)
	}

	// Create TypeScript file
	tsFile := filepath.Join(srcDir, "index.ts")
	if err := os.WriteFile(tsFile, []byte("export function hello() {}"), 0644); err != nil {
		t.Fatalf("failed to create TypeScript file: %v", err)
	}

	// Create output directory
	outputDir := filepath.Join(tmpDir, "docs")

	// Setup mock runner
	runner := site.NewMockRunner()
	runner.WithOutput("typedoc --version", "TypeDoc 0.25.0")
	runner.WithOutput("typedoc", "Documentation generated")

	// Simulate generated output files
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to create output directory: %v", err)
	}
	outputFile := filepath.Join(outputDir, "index.md")
	if err := os.WriteFile(outputFile, []byte("# Documentation\n"), 0644); err != nil {
		t.Fatalf("failed to create output file: %v", err)
	}

	extractor := NewJSExtractor(runner)

	// Create extract request
	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageTypeScript,
		SourceDir: tmpDir,
		OutputDir: outputDir,
	}

	// Run extraction
	result, err := extractor.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Verify result
	if result.Language != extractors.LanguageTypeScript {
		t.Errorf("expected language %s, got %s", extractors.LanguageTypeScript, result.Language)
	}

	if result.Stats.FilesProcessed == 0 {
		t.Error("expected at least 1 file to be processed")
	}

	// Verify typedoc was called
	calls := runner.GetCalls()
	foundTypedoc := false
	for _, call := range calls {
		if call.Cmd == "typedoc" && len(call.Args) > 0 {
			foundTypedoc = true
			break
		}
	}
	if !foundTypedoc {
		t.Error("expected typedoc command to be called")
	}
}

func TestJSExtractor_Extract_InvalidLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	runner := site.NewMockRunner()
	extractor := NewJSExtractor(runner)

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

func TestJSExtractor_Extract_InvalidSourceDir(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewJSExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageJavaScript,
		SourceDir: "/nonexistent/directory",
		OutputDir: t.TempDir(),
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid source directory")
	}
}

func TestJSExtractor_countLines(t *testing.T) {
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
			extractor := NewJSExtractor(runner)

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

func TestJSExtractor_Language(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewJSExtractor(runner)

	if extractor.Language() != extractors.LanguageJavaScript {
		t.Errorf("expected language %s, got %s", extractors.LanguageJavaScript, extractor.Language())
	}
}
