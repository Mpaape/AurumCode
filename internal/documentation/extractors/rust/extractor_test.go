package rust

import (
	"context"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewRustExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewRustExtractor(runner)

	if extractor == nil {
		t.Fatal("NewRustExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageRust {
		t.Errorf("expected language %s, got %s", extractors.LanguageRust, extractor.Language())
	}
}

func TestRustExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{"cargo installed", "cargo 1.70.0", nil, false},
		{"cargo not found", "", errors.New("not found"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("cargo --version", tt.err)
			} else {
				runner.WithOutput("cargo --version", tt.output)
			}

			extractor := NewRustExtractor(runner)
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

func TestRustExtractor_Extract_NoCargo(t *testing.T) {
	tmpDir := t.TempDir()
	runner := site.NewMockRunner()
	extractor := NewRustExtractor(runner)

	req := &extractors.ExtractRequest{
		Language:  extractors.LanguageRust,
		SourceDir: tmpDir,
		OutputDir: tmpDir,
	}

	result, err := extractor.Extract(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Errors) == 0 {
		t.Error("expected error for missing Cargo.toml")
	}
}
