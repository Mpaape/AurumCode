package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tmpDir := t.TempDir()

	// Create OpenAPI spec
	specPath := filepath.Join(tmpDir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte("openapi: 3.0.0"), 0644); err != nil {
		t.Fatal(err)
	}

	detector := NewDetector()
	location, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if location == nil {
		t.Fatal("Expected location to be found")
	}

	if location.Format != "yaml" {
		t.Errorf("Format = %v, want yaml", location.Format)
	}

	if !filepath.IsAbs(location.Path) {
		t.Error("Expected absolute path")
	}
}

func TestDetectInApiDir(t *testing.T) {
	tmpDir := t.TempDir()
	apiDir := filepath.Join(tmpDir, "api")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create OpenAPI spec in api directory
	specPath := filepath.Join(apiDir, "openapi.json")
	if err := os.WriteFile(specPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	detector := NewDetector()
	location, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if location.Format != "json" {
		t.Errorf("Format = %v, want json", location.Format)
	}
}

func TestDetectNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	detector := NewDetector()
	_, err := detector.Detect(tmpDir)
	if err == nil {
		t.Error("Expected error when no spec found")
	}
}

func TestDetectAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple specs
	if err := os.WriteFile(filepath.Join(tmpDir, "openapi.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	apiDir := filepath.Join(tmpDir, "api")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "swagger.json"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	detector := NewDetector()
	specs, err := detector.DetectAll(tmpDir)
	if err != nil {
		t.Fatalf("DetectAll failed: %v", err)
	}

	if len(specs) != 2 {
		t.Errorf("Expected 2 specs, got %d", len(specs))
	}
}

func TestDetectAllSkipsNodeModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create spec in node_modules (should be ignored)
	nodeModules := filepath.Join(tmpDir, "node_modules", "somepackage")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModules, "openapi.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	detector := NewDetector()
	_, err := detector.DetectAll(tmpDir)
	if err == nil {
		t.Error("Should not find specs in node_modules")
	}
}

func TestIsOpenAPIFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"openapi.yaml", true},
		{"openapi.yml", true},
		{"openapi.json", true},
		{"swagger.yaml", true},
		{"swagger.json", true},
		{"notapi.yaml", false},
		{"openapi", false},
		{"readme.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOpenAPIFile(tt.name)
			if got != tt.want {
				t.Errorf("isOpenAPIFile(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
