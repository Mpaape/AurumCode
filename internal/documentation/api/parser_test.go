package api

import (
	"os"
	"path/filepath"
	"testing"
)

const validYAMLSpec = `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
  description: A test API specification
  contact:
    name: API Support
    email: support@example.com
servers:
  - url: https://api.example.com
    description: Production server
paths:
  /users:
    get:
      tags:
        - users
      summary: List users
      description: Get a list of all users
      operationId: listUsers
  /users/{id}:
    get:
      tags:
        - users
      summary: Get user
      description: Get a single user by ID
tags:
  - name: users
    description: User management endpoints
`

const validJSONSpec = `{
  "openapi": "3.0.0",
  "info": {
    "title": "Test API",
    "version": "1.0.0"
  },
  "paths": {
    "/health": {
      "get": {
        "summary": "Health check"
      }
    }
  }
}`

const invalidSpec = `openapi: 3.0.0
# Missing required info fields
paths: {}`

func TestParseYAML(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	if err := os.WriteFile(specPath, []byte(validYAMLSpec), 0644); err != nil {
		t.Fatal(err)
	}

	location := &SpecLocation{
		Path:   specPath,
		Format: "yaml",
	}

	parser := NewParser()
	spec, err := parser.Parse(location)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify basic fields
	if spec.Info.Title != "Test API" {
		t.Errorf("Title = %v, want Test API", spec.Info.Title)
	}

	if spec.Info.Version != "1.0.0" {
		t.Errorf("Version = %v, want 1.0.0", spec.Info.Version)
	}

	if spec.OpenAPI != "3.0.0" {
		t.Errorf("OpenAPI version = %v, want 3.0.0", spec.OpenAPI)
	}

	// Verify paths
	if len(spec.Paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(spec.Paths))
	}

	// Verify servers
	if len(spec.Servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(spec.Servers))
	}

	if spec.Servers[0].URL != "https://api.example.com" {
		t.Errorf("Server URL = %v", spec.Servers[0].URL)
	}

	// Verify tags
	if len(spec.Tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(spec.Tags))
	}
}

func TestParseJSON(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.json")

	if err := os.WriteFile(specPath, []byte(validJSONSpec), 0644); err != nil {
		t.Fatal(err)
	}

	location := &SpecLocation{
		Path:   specPath,
		Format: "json",
	}

	parser := NewParser()
	spec, err := parser.Parse(location)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if spec.Info.Title != "Test API" {
		t.Errorf("Title = %v, want Test API", spec.Info.Title)
	}
}

func TestParseInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	if err := os.WriteFile(specPath, []byte(invalidSpec), 0644); err != nil {
		t.Fatal(err)
	}

	location := &SpecLocation{
		Path:   specPath,
		Format: "yaml",
	}

	parser := NewParser()
	_, err := parser.Parse(location)
	if err == nil {
		t.Error("Expected validation error for invalid spec")
	}
}

func TestGroupEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	if err := os.WriteFile(specPath, []byte(validYAMLSpec), 0644); err != nil {
		t.Fatal(err)
	}

	location := &SpecLocation{
		Path:   specPath,
		Format: "yaml",
	}

	parser := NewParser()
	spec, err := parser.Parse(location)
	if err != nil {
		t.Fatal(err)
	}

	groups := parser.GroupEndpoints(spec)

	// Should have one group (users)
	if len(groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(groups))
	}

	// Check group
	group := groups[0]
	if group.Tag != "users" {
		t.Errorf("Tag = %v, want users", group.Tag)
	}

	// Should have 2 endpoints (GET /users and GET /users/{id})
	if len(group.Endpoints) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(group.Endpoints))
	}

	// Check endpoints
	for _, endpoint := range group.Endpoints {
		if endpoint.Method != "GET" {
			t.Errorf("Method = %v, want GET", endpoint.Method)
		}

		if endpoint.Path != "/users" && endpoint.Path != "/users/{id}" {
			t.Errorf("Unexpected path: %v", endpoint.Path)
		}
	}
}

func TestExtractSummary(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	if err := os.WriteFile(specPath, []byte(validYAMLSpec), 0644); err != nil {
		t.Fatal(err)
	}

	location := &SpecLocation{
		Path:   specPath,
		Format: "yaml",
	}

	parser := NewParser()
	spec, err := parser.Parse(location)
	if err != nil {
		t.Fatal(err)
	}

	summary := parser.ExtractSummary(spec)

	if summary["title"] != "Test API" {
		t.Errorf("Title = %v", summary["title"])
	}

	if summary["version"] != "1.0.0" {
		t.Errorf("Version = %v", summary["version"])
	}

	if summary["endpoint_count"] != 2 {
		t.Errorf("Endpoint count = %v, want 2", summary["endpoint_count"])
	}

	if summary["path_count"] != 2 {
		t.Errorf("Path count = %v, want 2", summary["path_count"])
	}

	if summary["tag_count"] != 1 {
		t.Errorf("Tag count = %v, want 1", summary["tag_count"])
	}
}

func TestValidateUnsupportedVersion(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "2.0",
		Info: Info{
			Title:   "Test",
			Version: "1.0.0",
		},
	}

	parser := NewParser()
	err := parser.validate(spec)
	if err == nil {
		t.Error("Expected error for unsupported OpenAPI version")
	}
}

func TestValidateMissingFields(t *testing.T) {
	tests := []struct {
		name string
		spec OpenAPISpec
	}{
		{
			name: "missing title",
			spec: OpenAPISpec{
				OpenAPI: "3.0.0",
				Info: Info{
					Version: "1.0.0",
				},
			},
		},
		{
			name: "missing version",
			spec: OpenAPISpec{
				OpenAPI: "3.0.0",
				Info: Info{
					Title: "Test",
				},
			},
		},
		{
			name: "missing openapi",
			spec: OpenAPISpec{
				Info: Info{
					Title:   "Test",
					Version: "1.0.0",
				},
			},
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.validate(&tt.spec)
			if err == nil {
				t.Error("Expected validation error")
			}
		})
	}
}
