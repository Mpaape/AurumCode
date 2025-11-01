package api

import (
	"fmt"
	"os"
	"path/filepath"
)

// APITest represents a generated API test
type APITest struct {
	Name        string   `json:"name"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// Language represents the test language
type Language string

const (
	LanguageGo     Language = "go"
	LanguagePython Language = "python"
	LanguageJS     Language = "javascript"
)

// writeFile is a helper to write files
func writeFile(dir, filename, content string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
