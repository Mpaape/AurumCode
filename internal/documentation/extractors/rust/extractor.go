package rust

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// RustExtractor extracts documentation from Rust source code using cargo doc
type RustExtractor struct {
	runner site.CommandRunner
}

// NewRustExtractor creates a new Rust documentation extractor
func NewRustExtractor(runner site.CommandRunner) *RustExtractor {
	return &RustExtractor{
		runner: runner,
	}
}

// Extract generates documentation from Rust source code
func (r *RustExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageRust {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageRust, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Find Cargo.toml
	cargoPath := filepath.Join(req.SourceDir, "Cargo.toml")
	if _, err := os.Stat(cargoPath); err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageRust,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{fmt.Errorf("no Cargo.toml found")},
		}, nil
	}

	// Run cargo doc
	args := []string{"doc", "--no-deps"}
	_, err := r.runner.Run(ctx, "cargo", args, req.SourceDir, nil)
	if err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageRust,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{fmt.Errorf("cargo doc failed: %w", err)},
		}, nil
	}

	// Find generated HTML docs
	docDir := filepath.Join(req.SourceDir, "target", "doc")
	files, _ := r.countHTMLFiles(docDir)

	result := &extractors.ExtractResult{
		Language: extractors.LanguageRust,
		Files:    files,
		Stats: extractors.ExtractionStats{
			FilesProcessed: 1,
			DocsGenerated:  len(files),
		},
	}

	return result, nil
}

// Validate checks if Rust and Cargo are available
func (r *RustExtractor) Validate(ctx context.Context) error {
	_, err := r.runner.Run(ctx, "cargo", []string{"--version"}, ".", nil)
	if err != nil {
		return fmt.Errorf("cargo not found: please install from https://rustup.rs")
	}
	return nil
}

// Language returns the language this extractor handles
func (r *RustExtractor) Language() extractors.Language {
	return extractors.LanguageRust
}

func (r *RustExtractor) countHTMLFiles(dir string) ([]string, error) {
	files := []string{}
	if _, err := os.Stat(dir); err != nil {
		return files, nil
	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
			files = append(files, path)
		}
		return nil
	})
	return files, nil
}
