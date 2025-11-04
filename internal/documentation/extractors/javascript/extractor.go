package javascript

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// ProjectType represents JavaScript or TypeScript project
type ProjectType string

const (
	ProjectTypeJavaScript ProjectType = "javascript"
	ProjectTypeTypeScript ProjectType = "typescript"
)

// JSExtractor extracts documentation from JavaScript/TypeScript using TypeDoc
type JSExtractor struct {
	runner site.CommandRunner
}

// NewJSExtractor creates a new JavaScript/TypeScript documentation extractor
func NewJSExtractor(runner site.CommandRunner) *JSExtractor {
	return &JSExtractor{
		runner: runner,
	}
}

// Extract generates documentation from JavaScript/TypeScript source code
func (j *JSExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	// Validate request
	if req.Language != extractors.LanguageJavaScript && req.Language != extractors.LanguageTypeScript {
		return nil, fmt.Errorf("invalid language: expected JavaScript or TypeScript, got %s", req.Language)
	}

	// Validate source directory
	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Detect project type
	projectType, err := j.detectProjectType(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to detect project type: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find entry point (src directory or tsconfig.json)
	entryPoint, err := j.findEntryPoint(req.SourceDir, projectType)
	if err != nil {
		return nil, fmt.Errorf("failed to find entry point: %w", err)
	}

	// Extract documentation
	result := &extractors.ExtractResult{
		Language: req.Language,
		Files:    []string{},
		Errors:   []error{},
	}

	err = j.extractDocs(ctx, entryPoint, req.OutputDir, projectType)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("TypeDoc extraction failed: %w", err)
	}

	// Count generated files
	files, err := j.countGeneratedFiles(req.OutputDir)
	if err == nil {
		result.Files = files
		result.Stats.DocsGenerated = len(files)
		result.Stats.FilesProcessed = 1 // One project processed

		// Count total lines
		for _, file := range files {
			lines, _ := j.countLines(file)
			result.Stats.LinesProcessed += lines
		}
	}

	return result, nil
}

// Validate checks if TypeDoc is available
func (j *JSExtractor) Validate(ctx context.Context) error {
	// Try to get TypeDoc version
	_, err := j.runner.Run(ctx, "typedoc", []string{"--version"}, ".", nil)
	if err != nil {
		return fmt.Errorf("typedoc not found: please install with 'npm install -g typedoc typedoc-plugin-markdown'")
	}
	return nil
}

// Language returns the language this extractor handles
func (j *JSExtractor) Language() extractors.Language {
	return extractors.LanguageJavaScript
}

// detectProjectType detects if project is TypeScript or JavaScript
func (j *JSExtractor) detectProjectType(rootDir string) (ProjectType, error) {
	// Check for tsconfig.json (TypeScript)
	tsconfigPath := filepath.Join(rootDir, "tsconfig.json")
	if _, err := os.Stat(tsconfigPath); err == nil {
		return ProjectTypeTypeScript, nil
	}

	// Check for package.json (JavaScript)
	packagePath := filepath.Join(rootDir, "package.json")
	if _, err := os.Stat(packagePath); err == nil {
		return ProjectTypeJavaScript, nil
	}

	// Check for .ts files
	hasTS, err := j.hasTypeScriptFiles(rootDir)
	if err == nil && hasTS {
		return ProjectTypeTypeScript, nil
	}

	// Check for .js files
	hasJS, err := j.hasJavaScriptFiles(rootDir)
	if err == nil && hasJS {
		return ProjectTypeJavaScript, nil
	}

	return "", fmt.Errorf("could not detect JavaScript or TypeScript project")
}

// hasTypeScriptFiles checks if directory contains .ts or .tsx files
func (j *JSExtractor) hasTypeScriptFiles(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx") {
			found = true
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	return found, nil
}

// hasJavaScriptFiles checks if directory contains .js or .jsx files
func (j *JSExtractor) hasJavaScriptFiles(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".jsx") ||
			strings.HasSuffix(name, ".mjs") || strings.HasSuffix(name, ".cjs") {
			found = true
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	return found, nil
}

// findEntryPoint finds the entry point for TypeDoc
func (j *JSExtractor) findEntryPoint(rootDir string, projectType ProjectType) (string, error) {
	// For TypeScript, prefer tsconfig.json
	if projectType == ProjectTypeTypeScript {
		tsconfigPath := filepath.Join(rootDir, "tsconfig.json")
		if _, err := os.Stat(tsconfigPath); err == nil {
			return tsconfigPath, nil
		}
	}

	// Check for src directory
	srcDir := filepath.Join(rootDir, "src")
	if _, err := os.Stat(srcDir); err == nil {
		return srcDir, nil
	}

	// Check for lib directory
	libDir := filepath.Join(rootDir, "lib")
	if _, err := os.Stat(libDir); err == nil {
		return libDir, nil
	}

	// Use root directory as fallback
	return rootDir, nil
}

// extractDocs runs TypeDoc to extract documentation
func (j *JSExtractor) extractDocs(ctx context.Context, entryPoint, outputDir string, projectType ProjectType) error {
	args := []string{
		"--plugin", "typedoc-plugin-markdown",
		"--out", outputDir,
	}

	// Add entry point
	if filepath.Ext(entryPoint) == ".json" {
		// If entry point is tsconfig.json, use --tsconfig flag
		args = append(args, "--tsconfig", entryPoint)
	} else {
		// Otherwise, use entry point as source
		args = append(args, entryPoint)
	}

	// Additional flags for better output
	args = append(args, "--readme", "none") // Don't include README in output
	args = append(args, "--hideGenerator")  // Hide TypeDoc generator info

	_, err := j.runner.Run(ctx, "typedoc", args, ".", nil)
	if err != nil {
		return fmt.Errorf("typedoc failed: %w", err)
	}

	return nil
}

// countGeneratedFiles counts markdown files in output directory
func (j *JSExtractor) countGeneratedFiles(outputDir string) ([]string, error) {
	files := []string{}

	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// countLines counts lines in a file
func (j *JSExtractor) countLines(path string) (int, error) {
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
