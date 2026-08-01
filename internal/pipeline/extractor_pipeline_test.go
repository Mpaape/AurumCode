package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
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
		"main.go":              "package main",
		"util.go":              "package util",
		"app.py":               "def main():",
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

// stubExtractor is a configurable Extractor used to drive Run's outcome paths.
type stubExtractor struct {
	lang           extractors.Language
	validateErr    error
	extractErr     error
	filesProcessed int
	docsGenerated  int
	resultErrors   []error
}

func (s *stubExtractor) Language() extractors.Language { return s.lang }

func (s *stubExtractor) Validate(ctx context.Context) error { return s.validateErr }

func (s *stubExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if s.extractErr != nil {
		return nil, s.extractErr
	}

	return &extractors.ExtractResult{
		Language: s.lang,
		Stats: extractors.ExtractionStats{
			FilesProcessed: s.filesProcessed,
			DocsGenerated:  s.docsGenerated,
		},
		Errors: s.resultErrors,
	}, nil
}

func newRunPipeline(t *testing.T, sourceDir string, exts ...extractors.Extractor) *ExtractorPipeline {
	t.Helper()

	config := &ExtractorPipelineConfig{
		SourceDir: sourceDir,
		OutputDir: filepath.Join(sourceDir, "docs"),
		DocsDir:   filepath.Join(sourceDir, "docs"),
	}

	pipeline := NewExtractorPipeline(config, site.NewMockRunner(), nil)
	for _, ext := range exts {
		if err := pipeline.RegisterExtractor(ext); err != nil {
			t.Fatalf("RegisterExtractor failed: %v", err)
		}
	}

	return pipeline
}

func writeSourceFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

func TestExtractorPipeline_Run_AllExtractorsFail(t *testing.T) {
	tmpDir := t.TempDir()
	writeSourceFile(t, tmpDir, "main.go", "package main")

	pipeline := newRunPipeline(t, tmpDir, &stubExtractor{
		lang:       extractors.LanguageGo,
		extractErr: errors.New("godoc exploded"),
	})

	err := pipeline.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil when every extractor failed and no documentation was generated")
	}

	var extractErr *ExtractionError
	if !errors.As(err, &extractErr) {
		t.Fatalf("Run error = %v, want *ExtractionError", err)
	}

	if extractErr.Partial {
		t.Error("total extraction failure reported as partial")
	}

	if extractErr.DocsGenerated != 0 {
		t.Errorf("DocsGenerated = %d, want 0", extractErr.DocsGenerated)
	}

	if len(extractErr.Errors) == 0 {
		t.Error("ExtractionError should carry the underlying extractor errors")
	}
}

func TestExtractorPipeline_Run_NoDocsWithoutExtractorErrors(t *testing.T) {
	tmpDir := t.TempDir()
	writeSourceFile(t, tmpDir, "main.go", "package main")

	pipeline := newRunPipeline(t, tmpDir, &stubExtractor{
		lang:           extractors.LanguageGo,
		filesProcessed: 1,
		docsGenerated:  0,
	})

	err := pipeline.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil when files were processed but no documentation was generated")
	}

	var extractErr *ExtractionError
	if !errors.As(err, &extractErr) {
		t.Fatalf("Run error = %v, want *ExtractionError", err)
	}

	if extractErr.Partial {
		t.Error("empty documentation reported as partial success")
	}
}

func TestExtractorPipeline_Run_PartialExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	writeSourceFile(t, tmpDir, "main.go", "package main")
	writeSourceFile(t, tmpDir, "app.py", "def main():")

	pipeline := newRunPipeline(t, tmpDir,
		&stubExtractor{
			lang:           extractors.LanguageGo,
			filesProcessed: 1,
			docsGenerated:  3,
		},
		&stubExtractor{
			lang:        extractors.LanguagePython,
			validateErr: errors.New("pdoc not installed"),
		},
	)

	err := pipeline.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil for a partial extraction")
	}

	var extractErr *ExtractionError
	if !errors.As(err, &extractErr) {
		t.Fatalf("Run error = %v, want *ExtractionError", err)
	}

	if !extractErr.Partial {
		t.Error("partial extraction not flagged as partial")
	}

	if extractErr.DocsGenerated != 3 {
		t.Errorf("DocsGenerated = %d, want 3", extractErr.DocsGenerated)
	}
}

func TestExtractorPipeline_Run_FullSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	writeSourceFile(t, tmpDir, "main.go", "package main")

	pipeline := newRunPipeline(t, tmpDir, &stubExtractor{
		lang:           extractors.LanguageGo,
		filesProcessed: 1,
		docsGenerated:  2,
	})

	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error for a fully successful extraction: %v", err)
	}
}

func TestExtractorPipeline_Run_NoFilesToProcess(t *testing.T) {
	tmpDir := t.TempDir()
	writeSourceFile(t, tmpDir, "README.md", "# docs")

	pipeline := newRunPipeline(t, tmpDir)

	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error when there was nothing to process: %v", err)
	}
}

// TestExtractorPipeline_Run_LogsSourceFileCount pins the "Found %d files" log to the
// number of source files. The map is keyed by language, so logging len(filesToProcess)
// reports the language count instead - 2 rather than 3 for the fixture below.
func TestExtractorPipeline_Run_LogsSourceFileCount(t *testing.T) {
	tmpDir := t.TempDir()
	writeSourceFile(t, tmpDir, "main.go", "package main")
	writeSourceFile(t, tmpDir, "util.go", "package util")
	writeSourceFile(t, tmpDir, "app.py", "def main():")

	pipeline := newRunPipeline(t, tmpDir,
		&stubExtractor{
			lang:           extractors.LanguageGo,
			filesProcessed: 2,
			docsGenerated:  2,
		},
		&stubExtractor{
			lang:           extractors.LanguagePython,
			filesProcessed: 1,
			docsGenerated:  1,
		},
	)

	var logBuf bytes.Buffer
	originalWriter, originalFlags := log.Writer(), log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	}()

	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error for a fully successful extraction: %v", err)
	}

	if !strings.Contains(logBuf.String(), "Found 3 files to process") {
		t.Errorf("expected the log to report 3 source files across 2 languages, got:\n%s", logBuf.String())
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
