package api

import (
	"fmt"
	"os"
	"path/filepath"
)

// SpecLocation represents the location of an OpenAPI spec
type SpecLocation struct {
	Path   string
	Format string // "yaml" or "json"
}

// Detector finds OpenAPI specifications in a repository
type Detector struct{}

// NewDetector creates a new OpenAPI detector
func NewDetector() *Detector {
	return &Detector{}
}

// Detect searches for OpenAPI specs in common locations
func (d *Detector) Detect(repoPath string) (*SpecLocation, error) {
	// Common locations to check
	locations := []string{
		"openapi.yaml",
		"openapi.yml",
		"openapi.json",
		"api/openapi.yaml",
		"api/openapi.yml",
		"api/openapi.json",
		"docs/openapi.yaml",
		"docs/openapi.yml",
		"docs/openapi.json",
		"spec/openapi.yaml",
		"spec/openapi.yml",
		"spec/openapi.json",
		"swagger.yaml",
		"swagger.yml",
		"swagger.json",
		"api/swagger.yaml",
		"api/swagger.yml",
		"api/swagger.json",
	}

	for _, loc := range locations {
		fullPath := filepath.Join(repoPath, loc)
		if _, err := os.Stat(fullPath); err == nil {
			// File exists
			format := "yaml"
			if filepath.Ext(fullPath) == ".json" {
				format = "json"
			}

			return &SpecLocation{
				Path:   fullPath,
				Format: format,
			}, nil
		}
	}

	return nil, fmt.Errorf("no OpenAPI specification found")
}

// DetectAll finds all OpenAPI specs in a repository
func (d *Detector) DetectAll(repoPath string) ([]SpecLocation, error) {
	var specs []SpecLocation

	// Walk the repository looking for openapi/swagger files
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip common directories
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if filename matches OpenAPI patterns
		name := info.Name()
		if isOpenAPIFile(name) {
			format := "yaml"
			if filepath.Ext(name) == ".json" {
				format = "json"
			}

			specs = append(specs, SpecLocation{
				Path:   path,
				Format: format,
			})
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("no OpenAPI specifications found")
	}

	return specs, nil
}

// isOpenAPIFile checks if a filename matches OpenAPI patterns
func isOpenAPIFile(name string) bool {
	patterns := []string{
		"openapi.yaml",
		"openapi.yml",
		"openapi.json",
		"swagger.yaml",
		"swagger.yml",
		"swagger.json",
	}

	for _, pattern := range patterns {
		if name == pattern {
			return true
		}
	}

	return false
}
