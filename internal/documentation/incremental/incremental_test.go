package incremental

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewCache(t *testing.T) {
	cache := NewCache()

	if cache == nil {
		t.Fatal("NewCache returned nil")
	}

	if cache.Mappings == nil {
		t.Error("Mappings should be initialized")
	}

	if cache.LanguageMappings == nil {
		t.Error("LanguageMappings should be initialized")
	}
}

func TestCache_AddMapping(t *testing.T) {
	cache := NewCache()

	cache.AddMapping("src/main.go", "docs/main.md", "docs/api.md")

	docFiles := cache.GetDocFiles("src/main.go")
	if len(docFiles) != 2 {
		t.Errorf("Expected 2 doc files, got %d", len(docFiles))
	}

	if docFiles[0] != "docs/main.md" {
		t.Errorf("First doc file = %q, want %q", docFiles[0], "docs/main.md")
	}
}

func TestCache_GetAffectedDocs(t *testing.T) {
	cache := NewCache()

	cache.AddMapping("src/file1.go", "docs/file1.md")
	cache.AddMapping("src/file2.go", "docs/file2.md", "docs/shared.md")
	cache.AddMapping("src/file3.go", "docs/file3.md")

	changed := []string{"src/file1.go", "src/file2.go"}
	affected := cache.GetAffectedDocs(changed)

	// Should have file1.md, file2.md, and shared.md (deduplicated)
	if len(affected) < 2 || len(affected) > 3 {
		t.Errorf("Expected 2-3 affected docs, got %d: %v", len(affected), affected)
	}

	// Verify file3.md is NOT in affected
	for _, doc := range affected {
		if doc == "docs/file3.md" {
			t.Error("file3.md should not be affected")
		}
	}
}

func TestCache_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	// Create and populate cache
	cache1 := NewCache()
	cache1.AddMapping("src/main.go", "docs/main.md")
	cache1.UpdateCommit("abc123")

	// Save
	if err := cache1.Save(cachePath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("Cache file not created: %v", err)
	}

	// Load into new cache
	cache2 := NewCache()
	if err := cache2.Load(cachePath); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded data
	if cache2.LastCommit != "abc123" {
		t.Errorf("LastCommit = %q, want %q", cache2.LastCommit, "abc123")
	}

	if len(cache2.Mappings) != 1 {
		t.Errorf("Expected 1 mapping, got %d", len(cache2.Mappings))
	}
}

func TestLoadFromFile_NonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "nonexistent.json")

	cache, err := LoadFromFile(cachePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed for nonexistent file: %v", err)
	}

	if cache == nil {
		t.Fatal("Expected non-nil cache")
	}

	if !cache.IsEmpty() {
		t.Error("Cache should be empty for nonexistent file")
	}
}

func TestCache_IsEmpty(t *testing.T) {
	cache := NewCache()

	if !cache.IsEmpty() {
		t.Error("New cache should be empty")
	}

	cache.AddMapping("file.go", "doc.md")

	if cache.IsEmpty() {
		t.Error("Cache with mappings should not be empty")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache()
	cache.AddMapping("file.go", "doc.md")
	cache.UpdateCommit("abc123")

	cache.Clear()

	if !cache.IsEmpty() {
		t.Error("Cleared cache should be empty")
	}

	if cache.LastCommit != "" {
		t.Error("LastCommit should be empty after clear")
	}
}

func TestChangeDetector_FilterByLanguage(t *testing.T) {
	runner := site.NewMockRunner()
	detector := NewChangeDetector(runner, ".")

	files := []string{
		"src/main.go",
		"src/util.py",
		"src/app.js",
		"README.md",
		"test.go",
	}

	// Filter for Go files
	filtered := detector.FilterByLanguage(files, []string{".go", "go"})

	if len(filtered) != 2 {
		t.Errorf("Expected 2 Go files, got %d", len(filtered))
	}

	// Verify both .go files are present
	goFiles := 0
	for _, f := range filtered {
		if filepath.Ext(f) == ".go" {
			goFiles++
		}
	}

	if goFiles != 2 {
		t.Errorf("Expected 2 .go files, got %d", goFiles)
	}
}

func TestChangeDetector_DetectChanges(t *testing.T) {
	runner := site.NewMockRunner()
	detector := NewChangeDetector(runner, ".")

	// Mock git diff output
	runner.WithOutput("git diff --name-only abc123...def456", "file1.go\nfile2.go\nfile3.py")

	files, err := detector.DetectChanges(context.Background(), "abc123", "def456")
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}

	expectedFiles := map[string]bool{
		"file1.go": true,
		"file2.go": true,
		"file3.py": true,
	}

	for _, file := range files {
		if !expectedFiles[file] {
			t.Errorf("Unexpected file: %s", file)
		}
	}
}

func TestChangeDetector_GetCurrentCommit(t *testing.T) {
	runner := site.NewMockRunner()
	detector := NewChangeDetector(runner, ".")

	runner.WithOutput("git rev-parse HEAD", "abc123def456")

	commit, err := detector.GetCurrentCommit(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentCommit failed: %v", err)
	}

	if commit != "abc123def456" {
		t.Errorf("Commit = %q, want %q", commit, "abc123def456")
	}
}

func TestChangeDetector_IsGitRepository(t *testing.T) {
	runner := site.NewMockRunner()
	detector := NewChangeDetector(runner, ".")

	// Mock success
	runner.WithOutput("git rev-parse --git-dir", ".git")

	if !detector.IsGitRepository(context.Background()) {
		t.Error("Should detect git repository")
	}
}

func TestManager_NewManager(t *testing.T) {
	runner := site.NewMockRunner()
	manager := NewManager(runner, ".")

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.detector == nil {
		t.Error("Detector should be initialized")
	}

	if manager.cache == nil {
		t.Error("Cache should be initialized")
	}

	if manager.cachePath != defaultCachePath {
		t.Errorf("Cache path = %q, want %q", manager.cachePath, defaultCachePath)
	}
}

func TestManager_RegisterDocumentation(t *testing.T) {
	runner := site.NewMockRunner()
	manager := NewManager(runner, ".")

	manager.RegisterDocumentation("src/main.go", "docs/main.md", "docs/api.md")

	docFiles := manager.cache.GetDocFiles("src/main.go")
	if len(docFiles) != 2 {
		t.Errorf("Expected 2 doc files, got %d", len(docFiles))
	}
}

func TestManager_IsFirstRun(t *testing.T) {
	runner := site.NewMockRunner()
	manager := NewManager(runner, ".")

	if !manager.IsFirstRun() {
		t.Error("New manager should be first run")
	}

	manager.cache.UpdateCommit("abc123")

	if manager.IsFirstRun() {
		t.Error("Manager with commit should not be first run")
	}
}

func TestManager_SaveAndLoadCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	runner := site.NewMockRunner()
	manager := NewManagerWithCache(runner, ".", cachePath)

	// Add some data
	manager.RegisterDocumentation("src/file.go", "docs/file.md")

	// Save
	if err := manager.SaveCache(); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	// Create new manager and load
	manager2 := NewManagerWithCache(runner, ".", cachePath)
	if err := manager2.LoadCache(); err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}

	// Verify data
	docFiles := manager2.cache.GetDocFiles("src/file.go")
	if len(docFiles) != 1 {
		t.Errorf("Expected 1 doc file after load, got %d", len(docFiles))
	}
}

func TestCache_AddLanguageMapping(t *testing.T) {
	cache := NewCache()

	cache.AddLanguageMapping("go", "main.go", "util.go")
	cache.AddLanguageMapping("python", "app.py")

	goFiles := cache.GetSourcesByLanguage("go")
	if len(goFiles) != 2 {
		t.Errorf("Expected 2 Go files, got %d", len(goFiles))
	}

	pyFiles := cache.GetSourcesByLanguage("python")
	if len(pyFiles) != 1 {
		t.Errorf("Expected 1 Python file, got %d", len(pyFiles))
	}
}

func TestCache_HasChanges(t *testing.T) {
	cache := NewCache()

	cache.UpdateCommit("abc123")

	if cache.HasChanges("abc123") {
		t.Error("Cache should not have changes for same commit")
	}

	if !cache.HasChanges("def456") {
		t.Error("Cache should have changes for different commit")
	}
}

func TestCache_UpdateCommit(t *testing.T) {
	cache := NewCache()
	before := cache.LastUpdate

	time.Sleep(10 * time.Millisecond)

	cache.UpdateCommit("abc123")

	if cache.LastCommit != "abc123" {
		t.Errorf("LastCommit = %q, want %q", cache.LastCommit, "abc123")
	}

	if !cache.LastUpdate.After(before) {
		t.Error("LastUpdate should be updated")
	}
}
