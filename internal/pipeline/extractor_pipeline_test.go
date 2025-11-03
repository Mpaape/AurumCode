package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewExtractorPipeline(t *testing.T) {
	config := &ExtractorPipelineConfig{
		SourceDir: ".",
		OutputDir: "docs",
		DocsDir:   "docs",
	}

	runner := site.NewMockRunner()
	pipeline := NewExtractorPipeline(config, runner, nil)

	if pipeline == nil {
		t.Fatal("NewExtractorPipeline returned nil")
	}

	if pipeline.config != config {
		t.Error("Config not set correctly")
	}

	if pipeline.registry == nil {
		t.Error("Registry should be initialized")
	}

	if pipeline.normalizer == nil {
		t.Error("Normalizer should be initialized")
	}

	if pipeline.incrementalMgr == nil {
		t.Error("Incremental manager should be initialized")
	}
}

func TestDetectLanguageFromFile(t *testing.T) {
	tests := []struct {
		file string
		want extractors.Language
	}{
		{"main.go", extractors.LanguageGo},
		{"app.js", extractors.LanguageJavaScript},
		{"index.ts", extractors.LanguageTypeScript},
		{"script.py", extractors.LanguagePython},
		{"Program.cs", extractors.LanguageCSharp},
		{"Main.java", extractors.LanguageJava},
		{"code.cpp", extractors.LanguageCPP},
		{"lib.rs", extractors.LanguageRust},
		{"script.sh", extractors.LanguageBash},
		{"module.ps1", extractors.LanguagePowerShell},
		{"README.md", ""},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := detectLanguageFromFile(tt.file)
			if got != tt.want {
				t.Errorf("detectLanguageFromFile(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestShouldSkipPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/main.go", false},
		{"node_modules/pkg/index.js", true},
		{".git/config", true},
		{"vendor/lib/code.go", true},
		{"target/release/app", true},
		{"dist/bundle.js", true},
		{".taskmaster/tasks.json", true},
		{"internal/app/main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shouldSkipPath(tt.path)
			if got != tt.want {
				t.Errorf("shouldSkipPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGroupFilesByLanguage(t *testing.T) {
	config := &ExtractorPipelineConfig{
		SourceDir: ".",
		OutputDir: "docs",
	}

	runner := site.NewMockRunner()
	pipeline := NewExtractorPipeline(config, runner, nil)

	files := []string{
		"main.go",
		"util.go",
		"app.py",
		"script.js",
		"test.ts",
		"README.md",
	}

	grouped := pipeline.groupFilesByLanguage(files)

	// Should have 4 languages (Go, Python, JavaScript, TypeScript)
	if len(grouped) != 4 {
		t.Errorf("Expected 4 language groups, got %d", len(grouped))
	}

	// Check Go files
	goFiles := grouped[extractors.LanguageGo]
	if len(goFiles) != 2 {
		t.Errorf("Expected 2 Go files, got %d", len(goFiles))
	}

	// Check Python files
	pyFiles := grouped[extractors.LanguagePython]
	if len(pyFiles) != 1 {
		t.Errorf("Expected 1 Python file, got %d", len(pyFiles))
	}

	// README.md should not be in any group
	for lang, files := range grouped {
		for _, file := range files {
			if file == "README.md" {
				t.Errorf("README.md should not be in %s group", lang)
			}
		}
	}
}

func TestExtractorPipeline_DetermineFilesToProcess_FullMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test source files
	testFiles := map[string]string{
		"main.go":        "package main",
		"util.go":        "package util",
		"app.py":         "def main():",
		"node_modules/skip.js": "should be skipped",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	config := &ExtractorPipelineConfig{
		SourceDir:   tmpDir,
		OutputDir:   filepath.Join(tmpDir, "docs"),
		Incremental: false, // Full mode
	}

	runner := site.NewMockRunner()
	pipeline := NewExtractorPipeline(config, runner, nil)

	files, err := pipeline.determineFilesToProcess(context.Background())
	if err != nil {
		t.Fatalf("determineFilesToProcess failed: %v", err)
	}

	// Should find Go and Python files, but skip node_modules
	if len(files) == 0 {
		t.Error("Expected some files to be found")
	}

	// Check Go files were found
	goFiles := files[extractors.LanguageGo]
	if len(goFiles) < 2 {
		t.Errorf("Expected at least 2 Go files, got %d", len(goFiles))
	}

	// Check node_modules was skipped
	for _, fileList := range files {
		for _, file := range fileList {
			if filepath.Base(filepath.Dir(file)) == "node_modules" {
				t.Error("node_modules should be skipped")
			}
		}
	}
}

func TestExtractorPipeline_LanguageFilter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "app.py"), []byte("def main():"), 0644)

	config := &ExtractorPipelineConfig{
		SourceDir:   tmpDir,
		OutputDir:   filepath.Join(tmpDir, "docs"),
		Incremental: false,
		Languages:   []string{"go"}, // Only extract Go
	}

	runner := site.NewMockRunner()
	pipeline := NewExtractorPipeline(config, runner, nil)

	files, err := pipeline.determineFilesToProcess(context.Background())
	if err != nil {
		t.Fatalf("determineFilesToProcess failed: %v", err)
	}

	// Should only have Go files
	if len(files) != 1 {
		t.Errorf("Expected only 1 language group, got %d", len(files))
	}

	if _, ok := files[extractors.LanguageGo]; !ok {
		t.Error("Go language should be present")
	}

	if _, ok := files[extractors.LanguagePython]; ok {
		t.Error("Python language should be filtered out")
	}
}

func TestExtractorPipeline_Config(t *testing.T) {
	tests := []struct {
		name   string
		config *ExtractorPipelineConfig
	}{
		{
			name: "full mode with welcome",
			config: &ExtractorPipelineConfig{
				SourceDir:       "src",
				OutputDir:       "docs",
				DocsDir:         "docs",
				Incremental:     false,
				GenerateWelcome: true,
				ValidateJekyll:  false,
			},
		},
		{
			name: "incremental with validation",
			config: &ExtractorPipelineConfig{
				SourceDir:      ".",
				OutputDir:      "docs",
				DocsDir:        "docs",
				Incremental:    true,
				ValidateJekyll: true,
			},
		},
		{
			name: "language filtered",
			config: &ExtractorPipelineConfig{
				SourceDir: ".",
				OutputDir: "docs",
				Languages: []string{"go", "python"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			pipeline := NewExtractorPipeline(tt.config, runner, nil)

			if pipeline.config.SourceDir != tt.config.SourceDir {
				t.Error("SourceDir not set correctly")
			}

			if pipeline.config.Incremental != tt.config.Incremental {
				t.Error("Incremental not set correctly")
			}

			if pipeline.config.GenerateWelcome != tt.config.GenerateWelcome {
				t.Error("GenerateWelcome not set correctly")
			}
		})
	}
}
