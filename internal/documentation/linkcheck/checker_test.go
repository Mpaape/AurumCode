package linkcheck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testMarkdownWithLinks = `# Test Document

This is a test document with various links.

## Internal Links

- [README](README.md)
- [Documentation](docs/README.md)

## External Links

- [Google](https://www.google.com)
- [Example](https://example.com)

## Anchor Links

- [Top](#top)
- [Section](#test-document)

## Broken Links

- [Missing](missing.md)
- [Bad Anchor](#nonexistent)
`

func TestScanFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	if err := os.WriteFile(testFile, []byte(testMarkdownWithLinks), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner()
	links, err := scanner.ScanFile(testFile)
	if err != nil {
		t.Fatalf("ScanFile failed: %v", err)
	}

	if len(links) == 0 {
		t.Fatal("Expected to find links")
	}

	// Count by type
	var internal, external, anchor int
	for _, link := range links {
		switch link.Type {
		case LinkTypeInternal:
			internal++
		case LinkTypeExternal:
			external++
		case LinkTypeAnchor:
			anchor++
		}
	}

	if internal < 2 {
		t.Errorf("Expected at least 2 internal links, got %d", internal)
	}

	if external < 2 {
		t.Errorf("Expected at least 2 external links, got %d", external)
	}

	if anchor < 2 {
		t.Errorf("Expected at least 2 anchor links, got %d", anchor)
	}
}

func TestScanFileWithIgnorePatterns(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	if err := os.WriteFile(testFile, []byte(testMarkdownWithLinks), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner().WithIgnorePatterns([]string{"google.com", "example.com"})
	links, err := scanner.ScanFile(testFile)
	if err != nil {
		t.Fatalf("ScanFile failed: %v", err)
	}

	// Check that ignored links are not present
	for _, link := range links {
		if strings.Contains(link.URL, "google.com") || strings.Contains(link.URL, "example.com") {
			t.Errorf("Ignored link found: %s", link.URL)
		}
	}
}

func TestValidateInternal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	testFile := filepath.Join(tmpDir, "test.md")
	readmeFile := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readmeFile, []byte("readme"), 0644); err != nil {
		t.Fatal(err)
	}

	validator := NewValidator(tmpDir)

	// Test existing file
	link := Link{
		URL:        "README.md",
		Type:       LinkTypeInternal,
		SourceFile: testFile,
	}

	result := validator.validateInternal(link)
	if result.Status != LinkStatusOK {
		t.Errorf("Expected OK for existing file, got %v: %s", result.Status, result.Message)
	}

	// Test missing file
	missingLink := Link{
		URL:        "missing.md",
		Type:       LinkTypeInternal,
		SourceFile: testFile,
	}

	result = validator.validateInternal(missingLink)
	if result.Status != LinkStatusBroken {
		t.Errorf("Expected Broken for missing file, got %v", result.Status)
	}
}

func TestValidateAnchor(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	content := `# Test Document

## Section One

Some content here.

### Subsection

More content.
`

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	validator := NewValidator(tmpDir)

	tests := []struct {
		name     string
		anchor   string
		expected LinkStatus
	}{
		{"valid anchor", "#test-document", LinkStatusOK},
		{"valid section", "#section-one", LinkStatusOK},
		{"invalid anchor", "#nonexistent", LinkStatusBroken},
		{"empty anchor", "#", LinkStatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := Link{
				URL:        tt.anchor,
				Type:       LinkTypeAnchor,
				SourceFile: testFile,
			}

			result := validator.validateAnchor(link)
			if result.Status != tt.expected {
				t.Errorf("Expected %v, got %v: %s", tt.expected, result.Status, result.Message)
			}
		})
	}
}

func TestCheckDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create files with links
	file1 := filepath.Join(tmpDir, "README.md")
	file2 := filepath.Join(docsDir, "guide.md")

	content1 := `# README
[Guide](docs/guide.md)
[Missing](missing.md)
`

	content2 := `# Guide
[Back to README](../README.md)
`

	if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	// Check directory
	checker := NewChecker(tmpDir)
	report, err := checker.CheckDirectory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("CheckDirectory failed: %v", err)
	}

	if report.TotalLinks == 0 {
		t.Error("Expected to find links")
	}

	// Should have at least one broken link (missing.md)
	if report.BrokenLinks == 0 {
		t.Error("Expected to find broken links")
	}

	// Should have at least one OK link
	if report.OKLinks == 0 {
		t.Error("Expected to find OK links")
	}
}

func TestGenerateReport(t *testing.T) {
	checker := NewChecker(".")

	report := &Report{
		TotalLinks:   5,
		OKLinks:      3,
		BrokenLinks:  2,
		SkippedLinks: 0,
		Results: []LinkResult{
			{
				Link: Link{
					URL:        "missing.md",
					SourceFile: "test.md",
					LineNumber: 10,
				},
				Status:  LinkStatusBroken,
				Message: "file not found",
			},
		},
	}

	reportText := checker.GenerateReport(report)

	// Check report contains expected sections
	expectedSections := []string{
		"# Link Validation Report",
		"**Total Links:** 5",
		"**OK:** 3",
		"**Broken:** 2",
		"## Broken Links",
		"missing.md",
		"test.md:10",
	}

	for _, section := range expectedSections {
		if !strings.Contains(reportText, section) {
			t.Errorf("Report missing expected section: %s", section)
		}
	}
}

func TestWriteReport(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "reports", "links.md")

	checker := NewChecker(".")
	report := &Report{
		TotalLinks:  10,
		OKLinks:     8,
		BrokenLinks: 2,
	}

	err := checker.WriteReport(report, outputPath)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Report file was not created")
	}

	// Read and verify
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "Link Validation Report") {
		t.Error("Report missing header")
	}
}

func TestHasErrors(t *testing.T) {
	checker := NewChecker(".")

	tests := []struct {
		name     string
		report   *Report
		expected bool
	}{
		{
			name: "no errors",
			report: &Report{
				TotalLinks:  5,
				OKLinks:     5,
				BrokenLinks: 0,
			},
			expected: false,
		},
		{
			name: "has errors",
			report: &Report{
				TotalLinks:  5,
				OKLinks:     3,
				BrokenLinks: 2,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.HasErrors(tt.report)
			if got != tt.expected {
				t.Errorf("HasErrors() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestScanDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create structure
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create files
	file1 := filepath.Join(tmpDir, "README.md")
	file2 := filepath.Join(docsDir, "API.md")

	if err := os.WriteFile(file1, []byte("[Link](test.md)"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("[Link](../README.md)"), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner()
	links, err := scanner.ScanDirectory(tmpDir)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(links))
	}
}

func TestValidateExternalDisabled(t *testing.T) {
	validator := NewValidator(".").WithExternalCheck(false)

	link := Link{
		URL:  "https://example.com",
		Type: LinkTypeExternal,
	}

	result := validator.validateExternal(context.Background(), link)
	if result.Status != LinkStatusSkipped {
		t.Errorf("Expected skipped when external checking disabled, got %v", result.Status)
	}
}
