package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:       "Test API",
			Version:     "1.0.0",
			Description: "A test API",
		},
		Servers: []Server{
			{
				URL:         "https://api.example.com",
				Description: "Production",
			},
		},
		Paths: map[string]Path{
			"/users": {
				Get: &Operation{
					Tags:    []string{"users"},
					Summary: "List users",
				},
			},
		},
		Tags: []Tag{
			{
				Name:        "users",
				Description: "User operations",
			},
		},
	}

	generator := NewGenerator()
	content := generator.Generate(spec)

	// Check header
	if !strings.Contains(content, "# Test API") {
		t.Error("Missing title")
	}

	if !strings.Contains(content, "**Version:** 1.0.0") {
		t.Error("Missing version")
	}

	// Check description
	if !strings.Contains(content, "A test API") {
		t.Error("Missing description")
	}

	// Check servers
	if !strings.Contains(content, "## Servers") {
		t.Error("Missing servers section")
	}

	if !strings.Contains(content, "https://api.example.com") {
		t.Error("Missing server URL")
	}

	// Check tags
	if !strings.Contains(content, "## Tags") {
		t.Error("Missing tags section")
	}

	if !strings.Contains(content, "**users**") {
		t.Error("Missing tag name")
	}

	// Check endpoints
	if !strings.Contains(content, "## Endpoints") {
		t.Error("Missing endpoints section")
	}

	if !strings.Contains(content, "`GET /users`") {
		t.Error("Missing endpoint")
	}

	if !strings.Contains(content, "List users") {
		t.Error("Missing endpoint summary")
	}
}

func TestGenerateWithContact(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:   "Test API",
			Version: "1.0.0",
			Contact: Contact{
				Name:  "API Support",
				Email: "support@example.com",
				URL:   "https://example.com/support",
			},
		},
		Paths: map[string]Path{},
	}

	generator := NewGenerator()
	content := generator.Generate(spec)

	if !strings.Contains(content, "## Contact") {
		t.Error("Missing contact section")
	}

	if !strings.Contains(content, "API Support") {
		t.Error("Missing contact name")
	}

	if !strings.Contains(content, "support@example.com") {
		t.Error("Missing contact email")
	}
}

func TestGenerateDeprecated(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]Path{
			"/legacy": {
				Get: &Operation{
					Tags:       []string{"legacy"},
					Summary:    "Legacy endpoint",
					Deprecated: true,
				},
			},
		},
	}

	// With deprecated included
	generator := NewGenerator().WithDeprecated(true)
	content := generator.Generate(spec)

	if !strings.Contains(content, "⚠️ DEPRECATED") {
		t.Error("Missing deprecated badge")
	}

	if !strings.Contains(content, "Legacy endpoint") {
		t.Error("Missing deprecated endpoint")
	}

	// Without deprecated
	generator = NewGenerator().WithDeprecated(false)
	content = generator.Generate(spec)

	if strings.Contains(content, "Legacy endpoint") {
		t.Error("Deprecated endpoint should be excluded")
	}
}

func TestGenerateWithoutServers(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Servers: []Server{
			{URL: "https://api.example.com"},
		},
		Paths: map[string]Path{},
	}

	generator := NewGenerator().WithServers(false)
	content := generator.Generate(spec)

	if strings.Contains(content, "## Servers") {
		t.Error("Servers section should be excluded")
	}
}

func TestGenerateToFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "docs", "API.md")

	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]Path{},
	}

	generator := NewGenerator()
	err := generator.GenerateToFile(spec, outputPath)
	if err != nil {
		t.Fatalf("GenerateToFile failed: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "# Test API") {
		t.Error("Output file missing expected content")
	}
}

func TestGenerateMultipleMethods(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]Path{
			"/users": {
				Get: &Operation{
					Tags:    []string{"users"},
					Summary: "List users",
				},
				Post: &Operation{
					Tags:    []string{"users"},
					Summary: "Create user",
				},
				Delete: &Operation{
					Tags:    []string{"users"},
					Summary: "Delete user",
				},
			},
		},
	}

	generator := NewGenerator()
	content := generator.Generate(spec)

	// Should have all methods
	methods := []string{"GET", "POST", "DELETE"}
	for _, method := range methods {
		expected := fmt.Sprintf("`%s /users`", method)
		if !strings.Contains(content, expected) {
			t.Errorf("Missing method: %s", method)
		}
	}
}

func TestGenerateAPIIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create OpenAPI spec
	specPath := filepath.Join(tmpDir, "openapi.yaml")
	specContent := `openapi: 3.0.0
info:
  title: Integration Test API
  version: 1.0.0
paths:
  /health:
    get:
      summary: Health check
`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run GenerateAPI
	err := GenerateAPI(tmpDir)
	if err != nil {
		t.Fatalf("GenerateAPI failed: %v", err)
	}

	// Check output file
	outputPath := filepath.Join(tmpDir, "docs", "API.md")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	if !strings.Contains(string(content), "Integration Test API") {
		t.Error("Output missing API title")
	}

	if !strings.Contains(string(content), "Health check") {
		t.Error("Output missing endpoint")
	}
}

func TestGenerateAPINoSpec(t *testing.T) {
	tmpDir := t.TempDir()

	// Run GenerateAPI without any spec
	err := GenerateAPI(tmpDir)
	// Should not error - just return nil when no spec found
	if err != nil {
		t.Errorf("GenerateAPI should not error when no spec found: %v", err)
	}
}
