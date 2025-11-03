package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// PythonExtractor extracts documentation from Python source code using pydoc-markdown
type PythonExtractor struct {
	runner site.CommandRunner
}

// NewPythonExtractor creates a new Python documentation extractor
func NewPythonExtractor(runner site.CommandRunner) *PythonExtractor {
	return &PythonExtractor{
		runner: runner,
	}
}

// Extract generates documentation from Python source code
func (p *PythonExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	// Validate request
	if req.Language != extractors.LanguagePython {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguagePython, req.Language)
	}

	// Validate source directory
	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find all Python modules
	modules, err := p.findPythonModules(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find Python modules: %w", err)
	}

	if len(modules) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguagePython,
			Files:    []string{},
			Stats: extractors.ExtractionStats{
				FilesProcessed: 0,
				DocsGenerated:  0,
				LinesProcessed: 0,
			},
		}, nil
	}

	// Extract documentation for each module
	result := &extractors.ExtractResult{
		Language: extractors.LanguagePython,
		Files:    []string{},
		Errors:   []error{},
	}

	for _, module := range modules {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Generate output path
		moduleName := p.getModuleName(module, req.SourceDir)
		outputName := strings.ReplaceAll(moduleName, ".", "_")
		if outputName == "" {
			outputName = "module"
		}
		outputPath := filepath.Join(req.OutputDir, outputName+".md")

		// Extract documentation for this module
		err = p.extractModule(ctx, module, outputPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("module %s: %w", moduleName, err))
			continue
		}

		// Count lines in generated file
		lines, _ := p.countLines(outputPath)

		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
		result.Stats.LinesProcessed += lines
	}

	return result, nil
}

// Validate checks if pydoc-markdown is available
func (p *PythonExtractor) Validate(ctx context.Context) error {
	// Try to get pydoc-markdown version
	_, err := p.runner.Run(ctx, "pydoc-markdown", []string{"--version"}, ".", nil)
	if err != nil {
		return fmt.Errorf("pydoc-markdown not found: please install with 'pip install pydoc-markdown'")
	}
	return nil
}

// Language returns the language this extractor handles
func (p *PythonExtractor) Language() extractors.Language {
	return extractors.LanguagePython
}

// findPythonModules finds all Python modules in the source directory
func (p *PythonExtractor) findPythonModules(rootDir string) ([]string, error) {
	modules := []string{}
	visited := make(map[string]bool)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip non-Python files
		if info.IsDir() {
			// Skip common excluded directories
			dirName := filepath.Base(path)
			if dirName == "venv" || dirName == ".venv" || dirName == "env" ||
				dirName == "__pycache__" || dirName == ".git" ||
				dirName == "node_modules" || dirName == ".taskmaster" ||
				strings.HasPrefix(dirName, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for Python files (not test files)
		if strings.HasSuffix(path, ".py") && !strings.HasSuffix(path, "_test.py") &&
			!strings.Contains(path, "test_") && !visited[path] {
			modules = append(modules, path)
			visited[path] = true
		}

		return nil
	})

	return modules, err
}

// getModuleName converts file path to Python module name
func (p *PythonExtractor) getModuleName(modulePath, rootDir string) string {
	// Get relative path
	relPath, err := filepath.Rel(rootDir, modulePath)
	if err != nil {
		relPath = filepath.Base(modulePath)
	}

	// Remove .py extension
	moduleName := strings.TrimSuffix(relPath, ".py")

	// Replace path separators with dots
	moduleName = strings.ReplaceAll(moduleName, string(filepath.Separator), ".")

	// Remove __init__ from module name
	moduleName = strings.ReplaceAll(moduleName, ".__init__", "")

	return moduleName
}

// extractModule extracts documentation for a single Python module
func (p *PythonExtractor) extractModule(ctx context.Context, modulePath, outputPath string) error {
	// pydoc-markdown requires the module to be importable, which is complex
	// For simplicity, we'll read the Python file directly and extract docstrings
	// In a real implementation, this could use sphinx, pdoc, or pydoc-markdown properly

	// Read the Python file
	content, err := os.ReadFile(modulePath)
	if err != nil {
		return fmt.Errorf("failed to read module: %w", err)
	}

	// Extract docstrings (basic implementation)
	doc := p.extractDocstrings(string(content))

	// Write to output file
	if err := os.WriteFile(outputPath, []byte(doc), 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// extractDocstrings extracts docstrings from Python code (simplified)
func (p *PythonExtractor) extractDocstrings(code string) string {
	lines := strings.Split(code, "\n")
	var doc strings.Builder

	inDocstring := false
	docstringDelimiter := ""
	currentIndent := 0

	doc.WriteString("# Python Module Documentation\n\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for triple-quoted strings
		if !inDocstring {
			if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, "'''") {
				inDocstring = true
				docstringDelimiter = trimmed[:3]
				currentIndent = len(line) - len(strings.TrimLeft(line, " \t"))

				// Check if docstring ends on same line
				if strings.HasSuffix(trimmed, docstringDelimiter) && len(trimmed) > 6 {
					// Single-line docstring
					content := strings.TrimSuffix(strings.TrimPrefix(trimmed, docstringDelimiter), docstringDelimiter)
					doc.WriteString(content)
					doc.WriteString("\n\n")
					inDocstring = false
				} else {
					// Multi-line docstring start
					content := strings.TrimPrefix(trimmed, docstringDelimiter)
					if content != "" {
						doc.WriteString(content)
						doc.WriteString("\n")
					}
				}
				continue
			}
		} else {
			// Inside docstring
			if strings.Contains(trimmed, docstringDelimiter) {
				// End of docstring
				content := strings.TrimSuffix(trimmed, docstringDelimiter)
				if content != "" {
					doc.WriteString(content)
					doc.WriteString("\n")
				}
				doc.WriteString("\n")
				inDocstring = false
				continue
			}

			// Docstring content
			doc.WriteString(trimmed)
			doc.WriteString("\n")
		}
	}

	result := doc.String()
	if strings.TrimSpace(result) == "# Python Module Documentation" {
		return "# Python Module Documentation\n\nNo documentation found.\n"
	}

	return result
}

// countLines counts lines in a file
func (p *PythonExtractor) countLines(path string) (int, error) {
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
