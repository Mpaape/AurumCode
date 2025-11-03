package normalizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFrontMatter(t *testing.T) {
	tests := []struct {
		name     string
		opts     FrontMatterOptions
		wantVals map[string]interface{}
	}{
		{
			name: "basic file",
			opts: FrontMatterOptions{
				FilePath: "_api/example.md",
				Language: "go",
				Section:  "_api",
			},
			wantVals: map[string]interface{}{
				"layout": "default",
				"parent": "API Reference",
			},
		},
		{
			name: "index file",
			opts: FrontMatterOptions{
				FilePath: "_stack/index.md",
				Section:  "_stack",
				IsIndex:  true,
			},
			wantVals: map[string]interface{}{
				"has_children": true,
				"parent":       "Technology Stack",
			},
		},
		{
			name: "custom title",
			opts: FrontMatterOptions{
				FilePath:    "_tutorials/getting_started.md",
				CustomTitle: "Getting Started Guide",
			},
			wantVals: map[string]interface{}{
				"title": "Getting Started Guide",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := GenerateFrontMatter(tt.opts)

			if fm == nil {
				t.Fatal("GenerateFrontMatter returned nil")
			}

			if layout, ok := tt.wantVals["layout"]; ok && fm.Layout != layout {
				t.Errorf("Layout = %q, want %q", fm.Layout, layout)
			}

			if parent, ok := tt.wantVals["parent"]; ok && fm.Parent != parent {
				t.Errorf("Parent = %q, want %q", fm.Parent, parent)
			}

			if title, ok := tt.wantVals["title"]; ok && fm.Title != title {
				t.Errorf("Title = %q, want %q", fm.Title, title)
			}

			if hasChildren, ok := tt.wantVals["has_children"]; ok && fm.HasChildren != hasChildren {
				t.Errorf("HasChildren = %v, want %v", fm.HasChildren, hasChildren)
			}
		})
	}
}

func TestFrontMatter_ToYAML(t *testing.T) {
	fm := &FrontMatter{
		Title:  "Test Page",
		Layout: "default",
		Parent: "API Reference",
	}

	yaml, err := fm.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}

	if !strings.HasPrefix(yaml, "---\n") {
		t.Error("YAML should start with ---")
	}

	if !strings.Contains(yaml, "---\n\n") {
		t.Error("YAML should end with ---\\n\\n")
	}

	if !strings.Contains(yaml, "title: Test Page") {
		t.Error("YAML should contain title")
	}

	if !strings.Contains(yaml, "layout: default") {
		t.Error("YAML should contain layout")
	}
}

func TestParseFrontMatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFM      bool
		wantTitle   string
		wantContent string
	}{
		{
			name: "with front matter",
			content: `---
title: Test Page
layout: default
---

# Content here`,
			wantFM:      true,
			wantTitle:   "Test Page",
			wantContent: "# Content here",
		},
		{
			name:        "without front matter",
			content:     "# Just content\n\nNo front matter here",
			wantFM:      false,
			wantContent: "# Just content\n\nNo front matter here",
		},
		{
			name: "with front matter and extra content",
			content: `---
title: Another Page
parent: API Reference
---

## Section 1

Content goes here.`,
			wantFM:      true,
			wantTitle:   "Another Page",
			wantContent: "## Section 1\n\nContent goes here.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, content, err := ParseFrontMatter(tt.content)
			if err != nil {
				t.Fatalf("ParseFrontMatter failed: %v", err)
			}

			if tt.wantFM {
				if fm == nil {
					t.Fatal("Expected front matter but got nil")
				}
				if fm.Title != tt.wantTitle {
					t.Errorf("Title = %q, want %q", fm.Title, tt.wantTitle)
				}
			} else {
				if fm != nil {
					t.Errorf("Expected no front matter but got %+v", fm)
				}
			}

			if content != tt.wantContent {
				t.Errorf("Content = %q, want %q", content, tt.wantContent)
			}
		})
	}
}

func TestMergeFrontMatter(t *testing.T) {
	existing := &FrontMatter{
		Title:    "Existing Title",
		Layout:   "custom",
		NavOrder: 5,
	}

	new := &FrontMatter{
		Title:       "New Title",
		Layout:      "default",
		Parent:      "API Reference",
		HasChildren: true,
	}

	merged := MergeFrontMatter(existing, new)

	// Existing values should be preferred
	if merged.Title != "Existing Title" {
		t.Errorf("Title = %q, want %q", merged.Title, "Existing Title")
	}

	if merged.Layout != "custom" {
		t.Errorf("Layout = %q, want %q", merged.Layout, "custom")
	}

	if merged.NavOrder != 5 {
		t.Errorf("NavOrder = %d, want %d", merged.NavOrder, 5)
	}

	// New values should be used when existing is empty
	if merged.Parent != "API Reference" {
		t.Errorf("Parent = %q, want %q", merged.Parent, "API Reference")
	}

	if !merged.HasChildren {
		t.Error("HasChildren should be true")
	}
}

func TestNormalizer_NormalizeFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name            string
		initialContent  string
		expectedContain []string
	}{
		{
			name:           "file without front matter",
			initialContent: "# Test Document\n\nThis is content.",
			expectedContain: []string{
				"---",
				"title:",
				"layout: default",
				"# Test Document",
			},
		},
		{
			name: "file with existing front matter",
			initialContent: `---
title: Original Title
---

# Content`,
			expectedContain: []string{
				"---",
				"title: Original Title",
				"# Content",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			testFile := filepath.Join(tmpDir, "test.md")
			if err := os.WriteFile(testFile, []byte(tt.initialContent), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Normalize file
			normalizer := NewNormalizer(tmpDir)
			if err := normalizer.NormalizeFile(testFile); err != nil {
				t.Fatalf("NormalizeFile failed: %v", err)
			}

			// Read result
			result, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatalf("Failed to read result: %v", err)
			}

			resultStr := string(result)

			// Check expected content
			for _, expected := range tt.expectedContain {
				if !strings.Contains(resultStr, expected) {
					t.Errorf("Result should contain %q\nGot:\n%s", expected, resultStr)
				}
			}
		})
	}
}

func TestNormalizer_NormalizeDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directory structure
	files := map[string]string{
		"index.md":           "# Index",
		"page1.md":           "# Page 1",
		"subdir/page2.md":    "# Page 2",
		"_api/api_doc.md":    "# API Doc",
		"_site/ignore.md":    "# Should be ignored",
		"README.txt":         "Not markdown",
		"_stack/index.md":    "# Stack",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}

	// Normalize directory
	normalizer := NewNormalizer(tmpDir)
	processed, errors := normalizer.NormalizeDir(tmpDir)

	if len(errors) > 0 {
		t.Errorf("NormalizeDir returned errors: %v", errors)
	}

	// Should process markdown files but skip _site and .txt files
	expectedMin := 5 // index.md, page1.md, subdir/page2.md, _api/api_doc.md, _stack/index.md
	if processed < expectedMin {
		t.Errorf("Processed %d files, want at least %d", processed, expectedMin)
	}

	// Verify index.md was normalized
	indexPath := filepath.Join(tmpDir, "index.md")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("Failed to read index.md: %v", err)
	}

	if !strings.HasPrefix(string(content), "---\n") {
		t.Error("index.md should have front matter")
	}

	// Verify _site/ignore.md was NOT processed
	sitePath := filepath.Join(tmpDir, "_site", "ignore.md")
	siteContent, err := os.ReadFile(sitePath)
	if err != nil {
		t.Fatalf("Failed to read _site/ignore.md: %v", err)
	}

	if strings.HasPrefix(string(siteContent), "---\n") {
		t.Error("_site/ignore.md should NOT have been normalized")
	}
}

func TestDetectSection(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"_api/file.md", "_api"},
		{"_stack/docs/file.md", "_stack"},
		{"regular/path.md", ""},
		{"_tutorials/intro/getting_started.md", "_tutorials"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectSection(tt.path)
			if got != tt.want {
				t.Errorf("detectSection(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"_api/go/package.md", "go"},
		{"docs/python/module.md", "python"},
		{"javascript/api.md", "javascript"},
		{"src/typescript/types.md", "typescript"},
		{"cpp/class.md", "cpp"},
		{"rust/crate.md", "rust"},
		{"generic/file.md", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGenerateTitle(t *testing.T) {
	tests := []struct {
		path     string
		language string
		want     string
	}{
		{"api_reference.md", "go", "Api Reference - Go"},
		{"getting_started.md", "", "Getting Started"},
		{"my-cool-file.md", "python", "My Cool File - Python"},
		{"_api/index.md", "", "Api"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := generateTitle(tt.path, tt.language)
			if got != tt.want {
				t.Errorf("generateTitle(%q, %q) = %q, want %q", tt.path, tt.language, got, tt.want)
			}
		})
	}
}

func TestSectionToParent(t *testing.T) {
	tests := []struct {
		section string
		want    string
	}{
		{"_api", "API Reference"},
		{"_stack", "Technology Stack"},
		{"_architecture", "Architecture"},
		{"_tutorials", "Tutorials"},
		{"_roadmap", "Roadmap"},
		{"_custom", "Custom Documentation"},
		{"_unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.section, func(t *testing.T) {
			got := sectionToParent(tt.section)
			if got != tt.want {
				t.Errorf("sectionToParent(%q) = %q, want %q", tt.section, got, tt.want)
			}
		})
	}
}
