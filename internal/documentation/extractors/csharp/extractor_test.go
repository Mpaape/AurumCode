package csharp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewCSharpExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

	if extractor == nil {
		t.Fatal("NewCSharpExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageCSharp {
		t.Errorf("expected language %s, got %s", extractors.LanguageCSharp, extractor.Language())
	}
}

func TestCSharpExtractor_Validate(t *testing.T) {
	tests := []struct {
		name           string
		dotnetOutput   string
		dotnetErr      error
		xmldocmdOutput string
		xmldocmdErr    error
		wantError      bool
		errorContains  string
	}{
		{
			name:           "both tools installed",
			dotnetOutput:   "8.0.100",
			dotnetErr:      nil,
			xmldocmdOutput: "xmldocmd 2.7.2",
			xmldocmdErr:    nil,
			wantError:      false,
		},
		{
			name:          "dotnet not found",
			dotnetOutput:  "",
			dotnetErr:     errors.New("command not found"),
			wantError:     true,
			errorContains: "dotnet not found",
		},
		{
			name:           "xmldocmd not found",
			dotnetOutput:   "8.0.100",
			dotnetErr:      nil,
			xmldocmdOutput: "",
			xmldocmdErr:    errors.New("command not found"),
			wantError:      true,
			errorContains:  "xmldocmd not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()

			if tt.dotnetErr != nil {
				runner.WithError("dotnet --version", tt.dotnetErr)
			} else {
				runner.WithOutput("dotnet --version", tt.dotnetOutput)
			}

			if tt.xmldocmdErr != nil {
				runner.WithError("xmldocmd --version", tt.xmldocmdErr)
			} else {
				runner.WithOutput("xmldocmd --version", tt.xmldocmdOutput)
			}

			extractor := NewCSharpExtractor(runner)
			err := extractor.Validate(context.Background())

			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantError && err != nil && tt.errorContains != "" {
				if !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
			}
		})
	}
}

func TestCSharpExtractor_Extract_InvalidLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

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

func TestCSharpExtractor_Extract_InvalidSourceDir(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
		SourceDir: "/nonexistent/directory",
		OutputDir: t.TempDir(),
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid source directory")
	}
}

func TestCSharpExtractor_Extract_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "docs")

	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
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

func TestCSharpExtractor_findCSharpProjects(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	testDirs := []struct {
		path       string
		createFile bool
		fileName   string
	}{
		{"Project1", true, "Project1.csproj"},
		{"Project2", true, "Project2.csproj"},
		{"Nested/SubProject", true, "SubProject.csproj"},
		{"bin/Debug", true, "BuildOutput.csproj"},    // Should be excluded
		{"obj/Release", true, "Temp.csproj"},         // Should be excluded
		{".git/hooks", true, "Hook.csproj"},          // Should be excluded
		{"node_modules/lib", true, "Library.csproj"}, // Should be excluded
		{".taskmaster", true, "Task.csproj"},         // Should be excluded
	}

	for _, td := range testDirs {
		dir := filepath.Join(tmpDir, td.path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", td.path, err)
		}

		if td.createFile {
			file := filepath.Join(dir, td.fileName)
			content := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>`
			if err := os.WriteFile(file, []byte(content), 0644); err != nil {
				t.Fatalf("failed to create file in %s: %v", td.path, err)
			}
		}
	}

	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

	projects, err := extractor.findCSharpProjects(tmpDir)
	if err != nil {
		t.Fatalf("findCSharpProjects failed: %v", err)
	}

	// Should find Project1, Project2, SubProject
	// Should NOT find projects in bin, obj, .git, node_modules, .taskmaster
	expectedCount := 3
	if len(projects) != expectedCount {
		t.Errorf("expected %d projects, got %d", expectedCount, len(projects))
		for _, proj := range projects {
			t.Logf("Found project: %s", proj)
		}
	}

	// Verify excluded directories are not included
	for _, proj := range projects {
		if contains(proj, "bin") || contains(proj, "obj") ||
			contains(proj, ".git") || contains(proj, "node_modules") ||
			contains(proj, ".taskmaster") {
			t.Errorf("excluded directory found in projects: %s", proj)
		}
	}
}

func TestCSharpExtractor_getProjectName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{
			path:     "/path/to/MyProject.csproj",
			expected: "MyProject",
		},
		{
			path:     "C:\\Projects\\WebApp.csproj",
			expected: "WebApp",
		},
		{
			path:     "./Local/API.csproj",
			expected: "API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			runner := site.NewMockRunner()
			extractor := NewCSharpExtractor(runner)

			result := extractor.getProjectName(tt.path)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCSharpExtractor_findXmlDocFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various output structures
	testCases := []struct {
		name        string
		setupPath   string
		projectName string
		shouldFind  bool
	}{
		{
			name:        "Debug net8.0",
			setupPath:   "bin/Debug/net8.0",
			projectName: "TestProject",
			shouldFind:  true,
		},
		{
			name:        "Release net7.0",
			setupPath:   "bin/Release/net7.0",
			projectName: "TestProject",
			shouldFind:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create directory structure
			fullPath := filepath.Join(tmpDir, tc.setupPath)
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				t.Fatalf("failed to create directory: %v", err)
			}

			// Create XML file
			xmlFile := filepath.Join(fullPath, tc.projectName+".xml")
			if err := os.WriteFile(xmlFile, []byte("<doc></doc>"), 0644); err != nil {
				t.Fatalf("failed to create XML file: %v", err)
			}

			runner := site.NewMockRunner()
			extractor := NewCSharpExtractor(runner)

			result := extractor.findXmlDocFile(tmpDir, tc.projectName)

			if tc.shouldFind && result == "" {
				t.Error("expected to find XML file but got empty string")
			}
			if !tc.shouldFind && result != "" {
				t.Errorf("expected not to find XML file but got: %s", result)
			}
			if tc.shouldFind && result != "" && !contains(result, ".xml") {
				t.Errorf("expected result to contain .xml, got: %s", result)
			}

			// Cleanup for next test
			os.RemoveAll(fullPath)
		})
	}
}

func TestCSharpExtractor_countLines(t *testing.T) {
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
			extractor := NewCSharpExtractor(runner)

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

func TestCSharpExtractor_Language(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

	if extractor.Language() != extractors.LanguageCSharp {
		t.Errorf("expected language %s, got %s", extractors.LanguageCSharp, extractor.Language())
	}
}

func TestCSharpExtractor_Extract_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test projects
	for i := 0; i < 5; i++ {
		projectDir := filepath.Join(tmpDir, "Project"+string(rune('A'+i)))
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatalf("failed to create project directory: %v", err)
		}

		csprojFile := filepath.Join(projectDir, "Project.csproj")
		content := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>`
		if err := os.WriteFile(csprojFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create csproj file: %v", err)
		}
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := site.NewMockRunner()
	extractor := NewCSharpExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
		SourceDir: tmpDir,
		OutputDir: filepath.Join(tmpDir, "docs"),
	}

	_, err := extractor.Extract(ctx, req)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
