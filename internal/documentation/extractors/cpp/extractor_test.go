package cpp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewCPPExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewCPPExtractor(runner)

	if extractor == nil {
		t.Fatal("NewCPPExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageCPP {
		t.Errorf("expected language %s, got %s", extractors.LanguageCPP, extractor.Language())
	}
}

func TestCPPExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{"doxygen installed", "1.9.1", nil, false},
		{"doxygen not found", "", errors.New("not found"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("doxygen --version", tt.err)
			} else {
				runner.WithOutput("doxygen --version", tt.output)
			}

			extractor := NewCPPExtractor(runner)
			err := extractor.Validate(context.Background())

			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCPPExtractor_Extract_InvalidLanguage(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewCPPExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: t.TempDir(),
		OutputDir: t.TempDir(),
	}

	_, err := extractor.Extract(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid language")
	}
}

func TestCPPExtractor_findCPPFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []string{"main.cpp", "utils.h", "test.cc", "lib.cxx"}
	for _, file := range testFiles {
		path := filepath.Join(tmpDir, file)
		if err := os.WriteFile(path, []byte("// C++ code"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	runner := site.NewMockRunner()
	extractor := NewCPPExtractor(runner)

	files, err := extractor.findCPPFiles(tmpDir)
	if err != nil {
		t.Fatalf("findCPPFiles failed: %v", err)
	}

	if len(files) != len(testFiles) {
		t.Errorf("expected %d files, got %d", len(testFiles), len(files))
	}
}
