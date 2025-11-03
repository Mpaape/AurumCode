package goextractor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// GoExtractor extracts documentation from Go source code using gomarkdoc
type GoExtractor struct {
	runner          site.CommandRunner
	incrementalMode bool
}

// NewGoExtractor creates a new Go documentation extractor
func NewGoExtractor(runner site.CommandRunner) *GoExtractor {
	return &GoExtractor{
		runner:          runner,
		incrementalMode: true,
	}
}

// WithIncrementalMode enables or disables incremental generation
func (g *GoExtractor) WithIncrementalMode(enabled bool) *GoExtractor {
	g.incrementalMode = enabled
	return g
}

// Extract generates documentation from Go source code
func (g *GoExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	// Validate request
	if req.Language != extractors.LanguageGo {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageGo, req.Language)
	}

	// Validate source directory
	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find all Go packages
	packages, err := g.findGoPackages(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find Go packages: %w", err)
	}

	if len(packages) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguageGo,
			Files:    []string{},
			Stats: extractors.ExtractionStats{
				FilesProcessed: 0,
				DocsGenerated:  0,
				LinesProcessed: 0,
			},
		}, nil
	}

	// Extract documentation for each package
	result := &extractors.ExtractResult{
		Language: extractors.LanguageGo,
		Files:    []string{},
		Errors:   []error{},
	}

	for _, pkg := range packages {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Generate output path
		relPath, err := filepath.Rel(req.SourceDir, pkg)
		if err != nil {
			relPath = filepath.Base(pkg)
		}

		// Clean up the relative path for output filename
		outputName := strings.ReplaceAll(relPath, string(filepath.Separator), "_")
		if outputName == "." || outputName == "" {
			outputName = "root"
		}
		outputPath := filepath.Join(req.OutputDir, outputName+".md")

		// Check if we should skip this package (incremental mode)
		if g.incrementalMode && g.shouldSkipPackage(pkg, outputPath) {
			result.Stats.FilesProcessed++
			continue
		}

		// Extract documentation for this package
		err = g.extractPackage(ctx, pkg, outputPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("package %s: %w", pkg, err))
			continue
		}

		// Count lines in generated file
		lines, _ := g.countLines(outputPath)

		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
		result.Stats.LinesProcessed += lines
	}

	return result, nil
}

// Validate checks if gomarkdoc is available
func (g *GoExtractor) Validate(ctx context.Context) error {
	// Try to get gomarkdoc version
	_, err := g.runner.Run(ctx, "gomarkdoc", []string{"--version"}, ".", nil)
	if err != nil {
		return fmt.Errorf("gomarkdoc not found: please install with 'go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest'")
	}
	return nil
}

// Language returns the language this extractor handles
func (g *GoExtractor) Language() extractors.Language {
	return extractors.LanguageGo
}

// findGoPackages finds all Go packages in the source directory
func (g *GoExtractor) findGoPackages(rootDir string) ([]string, error) {
	packages := []string{}
	visited := make(map[string]bool)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip non-directories
		if !info.IsDir() {
			return nil
		}

		// Skip common excluded directories
		dirName := filepath.Base(path)
		if dirName == "vendor" || dirName == "node_modules" || dirName == ".git" ||
			dirName == "testdata" || dirName == ".taskmaster" || strings.HasPrefix(dirName, ".") {
			return filepath.SkipDir
		}

		// Check if directory contains Go files
		hasGoFiles, err := g.hasGoFiles(path)
		if err != nil {
			return nil // Skip errors
		}

		if hasGoFiles && !visited[path] {
			packages = append(packages, path)
			visited[path] = true
		}

		return nil
	})

	return packages, err
}

// hasGoFiles checks if a directory contains any .go files (excluding tests)
func (g *GoExtractor) hasGoFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check for .go files but exclude test files
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true, nil
		}
	}

	return false, nil
}

// shouldSkipPackage determines if a package should be skipped in incremental mode
func (g *GoExtractor) shouldSkipPackage(pkgPath, outputPath string) bool {
	// Check if output file exists
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		return false // Output doesn't exist, must generate
	}

	// Check if any source file is newer than output
	entries, err := os.ReadDir(pkgPath)
	if err != nil {
		return false // Error reading directory, regenerate to be safe
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// If source file is newer than output, must regenerate
		if info.ModTime().After(outputInfo.ModTime()) {
			return false
		}
	}

	// All source files are older than output, can skip
	return true
}

// extractPackage extracts documentation for a single package
func (g *GoExtractor) extractPackage(ctx context.Context, pkgPath, outputPath string) error {
	// Run gomarkdoc
	args := []string{
		"-o", outputPath,
		pkgPath,
	}

	_, err := g.runner.Run(ctx, "gomarkdoc", args, ".", nil)
	if err != nil {
		return fmt.Errorf("gomarkdoc failed: %w", err)
	}

	return nil
}

// countLines counts lines in a file
func (g *GoExtractor) countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}

	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}

	return count, nil
}
