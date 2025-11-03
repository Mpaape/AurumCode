package extractors

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewDetector(t *testing.T) {
	detector := NewDetector()

	if detector == nil {
		t.Fatal("NewDetector returned nil")
	}

	// Verify excluded directories are set
	expectedExcluded := []string{"vendor", "node_modules", ".git", "bin", "obj", "_site", ".taskmaster"}
	for _, dir := range expectedExcluded {
		if !detector.excludedDirs[dir] {
			t.Errorf("expected %s to be in excluded directories", dir)
		}
	}

	// Verify extension mappings
	if detector.extensions[".go"] != LanguageGo {
		t.Error("expected .go to map to LanguageGo")
	}
	if detector.extensions[".js"] != LanguageJavaScript {
		t.Error("expected .js to map to LanguageJavaScript")
	}
	if detector.extensions[".py"] != LanguagePython {
		t.Error("expected .py to map to LanguagePython")
	}
}

func TestDetector_WithExcludedDirs(t *testing.T) {
	detector := NewDetector().WithExcludedDirs("custom1", "custom2")

	if !detector.excludedDirs["custom1"] {
		t.Error("expected custom1 to be excluded")
	}
	if !detector.excludedDirs["custom2"] {
		t.Error("expected custom2 to be excluded")
	}
}

func TestDetector_WithExtensions(t *testing.T) {
	detector := NewDetector().WithExtensions(map[string]Language{
		".custom": LanguageGo,
	})

	if detector.extensions[".custom"] != LanguageGo {
		t.Error("expected .custom to map to LanguageGo")
	}
}

func TestDetector_Detect(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"main.go":              "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
		"utils.go":             "package main\n\nfunc helper() {}\n",
		"app.js":               "console.log('hello');\n",
		"script.py":            "print('hello')\n",
		"component.tsx":        "export default function Component() {}\n",
		"README.md":            "# README\n",
		"subdir/nested.go":     "package subdir\n",
		"subdir/nested.py":     "def foo(): pass\n",
		"vendor/external.go":   "package vendor\n", // Should be excluded
		"node_modules/lib.js":  "module.exports = {};\n", // Should be excluded
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)

		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Run detection
	detector := NewDetector()
	result, err := detector.Detect(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Verify results
	if result == nil {
		t.Fatal("result is nil")
	}

	// Should detect Go, JavaScript, Python, TypeScript
	// Should NOT count files in vendor/ or node_modules/
	expectedLanguages := map[Language]int{
		LanguageGo:         3, // main.go, utils.go, subdir/nested.go
		LanguageJavaScript: 1, // app.js
		LanguagePython:     2, // script.py, subdir/nested.py
		LanguageTypeScript: 1, // component.tsx
	}

	if len(result.Languages) != len(expectedLanguages) {
		t.Errorf("expected %d languages, got %d", len(expectedLanguages), len(result.Languages))
	}

	for lang, expectedCount := range expectedLanguages {
		stats, ok := result.Languages[lang]
		if !ok {
			t.Errorf("expected language %s to be detected", lang)
			continue
		}

		if stats.FileCount != expectedCount {
			t.Errorf("expected %d files for %s, got %d", expectedCount, lang, stats.FileCount)
		}

		if len(stats.Files) != expectedCount {
			t.Errorf("expected %d file paths for %s, got %d", expectedCount, lang, len(stats.Files))
		}

		if stats.LineCount == 0 {
			t.Errorf("expected non-zero line count for %s", lang)
		}
	}

	// Verify total counts
	expectedTotalFiles := 7 // Excluding vendor and node_modules
	if result.TotalFiles != expectedTotalFiles {
		t.Errorf("expected %d total files, got %d", expectedTotalFiles, result.TotalFiles)
	}

	if result.TotalLines == 0 {
		t.Error("expected non-zero total lines")
	}
}

func TestDetector_Detect_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	detector := NewDetector()
	result, err := detector.Detect(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if len(result.Languages) != 0 {
		t.Errorf("expected 0 languages, got %d", len(result.Languages))
	}

	if result.TotalFiles != 0 {
		t.Errorf("expected 0 files, got %d", result.TotalFiles)
	}
}

func TestDetector_Detect_InvalidDirectory(t *testing.T) {
	detector := NewDetector()
	_, err := detector.Detect(context.Background(), "/nonexistent/directory/path")
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

func TestDetector_Detect_FileNotDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	detector := NewDetector()
	_, err := detector.Detect(context.Background(), filePath)
	if err == nil {
		t.Error("expected error when path is a file not directory")
	}
}

func TestDetector_Detect_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many files to increase detection time
	for i := 0; i < 100; i++ {
		path := filepath.Join(tmpDir, "file"+string(rune(i))+".go")
		if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait to ensure context is cancelled
	time.Sleep(10 * time.Millisecond)

	detector := NewDetector()
	_, err := detector.Detect(ctx, tmpDir)

	// Should return context error
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestDetector_Detect_ExcludedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files in excluded directories
	excludedDirs := []string{"vendor", "node_modules", ".git", "bin", "obj", "_site", ".taskmaster"}

	for _, dir := range excludedDirs {
		dirPath := filepath.Join(tmpDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}

		filePath := filepath.Join(dirPath, "file.go")
		if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
			t.Fatalf("failed to create file in %s: %v", dir, err)
		}
	}

	// Create one file in root (should be detected)
	rootFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(rootFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to create root file: %v", err)
	}

	detector := NewDetector()
	result, err := detector.Detect(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	// Should only detect 1 file (the root file)
	if result.TotalFiles != 1 {
		t.Errorf("expected 1 file, got %d (excluded directories were not skipped)", result.TotalFiles)
	}
}

func TestDetector_countLines(t *testing.T) {
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
			name:     "single line with newline",
			content:  "line1\n",
			expected: 1,
		},
		{
			name:     "single line without newline",
			content:  "line1",
			expected: 1,
		},
		{
			name:     "multiple lines with newline",
			content:  "line1\nline2\nline3\n",
			expected: 3,
		},
		{
			name:     "multiple lines without final newline",
			content:  "line1\nline2\nline3",
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "test.txt")

			if err := os.WriteFile(filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			detector := NewDetector()
			count, err := detector.countLines(filePath)
			if err != nil {
				t.Fatalf("countLines failed: %v", err)
			}

			if count != tt.expected {
				t.Errorf("expected %d lines, got %d", tt.expected, count)
			}
		})
	}
}

func TestDetectionResult_GetLanguages(t *testing.T) {
	result := &DetectionResult{
		Languages: map[Language]*LanguageStats{
			LanguageGo:     {Language: LanguageGo},
			LanguagePython: {Language: LanguagePython},
		},
	}

	langs := result.GetLanguages()
	if len(langs) != 2 {
		t.Errorf("expected 2 languages, got %d", len(langs))
	}

	// Verify both languages are present
	hasGo := false
	hasPython := false
	for _, lang := range langs {
		if lang == LanguageGo {
			hasGo = true
		}
		if lang == LanguagePython {
			hasPython = true
		}
	}

	if !hasGo || !hasPython {
		t.Error("expected both Go and Python in language list")
	}
}

func TestDetectionResult_HasLanguage(t *testing.T) {
	result := &DetectionResult{
		Languages: map[Language]*LanguageStats{
			LanguageGo: {Language: LanguageGo},
		},
	}

	if !result.HasLanguage(LanguageGo) {
		t.Error("expected HasLanguage to return true for Go")
	}

	if result.HasLanguage(LanguagePython) {
		t.Error("expected HasLanguage to return false for Python")
	}
}

func TestDetectionResult_GetStats(t *testing.T) {
	expectedStats := &LanguageStats{
		Language:  LanguageGo,
		FileCount: 5,
		LineCount: 100,
	}

	result := &DetectionResult{
		Languages: map[Language]*LanguageStats{
			LanguageGo: expectedStats,
		},
	}

	stats, ok := result.GetStats(LanguageGo)
	if !ok {
		t.Error("expected GetStats to return true for Go")
	}
	if stats != expectedStats {
		t.Error("expected same stats instance")
	}

	_, ok = result.GetStats(LanguagePython)
	if ok {
		t.Error("expected GetStats to return false for Python")
	}
}
