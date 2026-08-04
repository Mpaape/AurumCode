package cpp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// CPPExtractor extracts documentation from C/C++ source code using Doxygen
type CPPExtractor struct {
	runner site.CommandRunner
}

// NewCPPExtractor creates a new C/C++ documentation extractor
func NewCPPExtractor(runner site.CommandRunner) *CPPExtractor {
	return &CPPExtractor{
		runner: runner,
	}
}

// Extract generates documentation from C/C++ source code
func (c *CPPExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageCPP {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageCPP, req.Language)
	}

	// Both paths are interpolated into the Doxyfile, so they are validated
	// before anything is created on disk. A request carrying a directive
	// injection is refused without leaving an output directory behind.
	if err := validateDoxyfileValue("source directory", req.SourceDir); err != nil {
		return nil, err
	}
	if err := validateDoxyfileValue("output directory", req.OutputDir); err != nil {
		return nil, err
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find C/C++ files
	files, err := c.findCPPFiles(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find C/C++ files: %w", err)
	}

	if len(files) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguageCPP,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
		}, nil
	}

	// Create the Doxyfile in a private directory owned by this run, so
	// concurrent extractions cannot share or delete each other's configuration.
	doxyfilePath, cleanup, err := c.newDoxyfile(req.SourceDir, req.OutputDir)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Run Doxygen
	_, err = c.runner.Run(ctx, "doxygen", []string{doxyfilePath}, ".", nil)
	if err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageCPP,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{fmt.Errorf("doxygen failed: %w", err)},
		}, nil
	}

	// Count generated files. Doxygen exiting 0 proves nothing; only the XML it
	// actually left on disk counts as documentation.
	genFiles, _ := c.countGeneratedFiles(req.OutputDir)
	if err := extractors.ConfirmOutputFiles("doxygen", filepath.Join(req.OutputDir, "xml"), genFiles); err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageCPP,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{err},
		}, nil
	}

	result := &extractors.ExtractResult{
		Language: extractors.LanguageCPP,
		Files:    genFiles,
		Stats: extractors.ExtractionStats{
			FilesProcessed: len(files),
			DocsGenerated:  len(genFiles),
		},
	}

	return result, nil
}

// Validate checks if Doxygen is available.
//
// Only a genuine lookup failure means doxygen is unavailable; an installed
// doxygen that exits non-zero is an extraction error, not a skip.
func (c *CPPExtractor) Validate(ctx context.Context) error {
	_, err := c.runner.Run(ctx, "doxygen", []string{"--version"}, ".", nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return extractors.NewToolUnavailableError("doxygen", "please install from https://www.doxygen.nl", err)
		}
		return fmt.Errorf("doxygen is installed but failed: %w", err)
	}
	return nil
}

// Language returns the language this extractor handles
func (c *CPPExtractor) Language() extractors.Language {
	return extractors.LanguageCPP
}

func (c *CPPExtractor) findCPPFiles(rootDir string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".cpp" || ext == ".cc" || ext == ".cxx" || ext == ".c" ||
			ext == ".h" || ext == ".hpp" || ext == ".hxx" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (c *CPPExtractor) countGeneratedFiles(outputDir string) ([]string, error) {
	files := []string{}
	xmlDir := filepath.Join(outputDir, "xml")
	if _, err := os.Stat(xmlDir); err != nil {
		return files, nil
	}
	filepath.Walk(xmlDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".xml") {
			files = append(files, path)
		}
		return nil
	})
	return files, nil
}
