package normalizer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Normalizer handles adding Jekyll front matter to markdown files
type Normalizer struct {
	docsRoot string // Root directory of docs
}

// NewNormalizer creates a new markdown normalizer
func NewNormalizer(docsRoot string) *Normalizer {
	return &Normalizer{
		docsRoot: docsRoot,
	}
}

// NormalizeFile adds or updates front matter in a single markdown file
func (n *Normalizer) NormalizeFile(filePath string) error {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse existing front matter
	existingFM, bodyContent, err := ParseFrontMatter(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse front matter: %w", err)
	}

	// Generate new front matter based on file context
	opts := n.buildOptions(filePath)
	newFM := GenerateFrontMatter(opts)

	// Merge with existing front matter
	var finalFM *FrontMatter
	if existingFM != nil {
		finalFM = MergeFrontMatter(existingFM, newFM)
	} else {
		finalFM = newFM
	}

	// Convert to YAML
	fmYAML, err := finalFM.ToYAML()
	if err != nil {
		return fmt.Errorf("failed to generate YAML: %w", err)
	}

	// Combine front matter and body
	normalized := fmYAML + bodyContent

	// Write back to file
	if err := os.WriteFile(filePath, []byte(normalized), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// NormalizeDir recursively processes all markdown files in a directory
func (n *Normalizer) NormalizeDir(dirPath string) (int, []error) {
	var processed int
	var errors []error

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errors = append(errors, fmt.Errorf("error accessing %s: %w", path, err))
			return nil // Continue walking
		}

		// Skip directories and non-markdown files
		if info.IsDir() || !isMarkdownFile(path) {
			return nil
		}

		// Skip Jekyll internal directories
		if shouldSkip(path) {
			return nil
		}

		// Normalize the file
		if err := n.NormalizeFile(path); err != nil {
			errors = append(errors, fmt.Errorf("failed to normalize %s: %w", path, err))
		} else {
			processed++
		}

		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Errorf("directory walk failed: %w", err))
	}

	return processed, errors
}

// buildOptions creates FrontMatterOptions from file path
func (n *Normalizer) buildOptions(filePath string) FrontMatterOptions {
	opts := FrontMatterOptions{
		IsIndex: filepath.Base(filePath) == "index.md",
	}

	// Calculate relative path from docs root
	if n.docsRoot != "" {
		relPath, err := filepath.Rel(n.docsRoot, filePath)
		if err == nil {
			opts.FilePath = filepath.ToSlash(relPath)
		}
	} else {
		opts.FilePath = filepath.ToSlash(filePath)
	}

	// Detect section from path
	opts.Section = detectSection(opts.FilePath)

	// Detect language from path or filename
	opts.Language = detectLanguage(opts.FilePath)

	return opts
}

// detectSection identifies the documentation section from path
func detectSection(path string) string {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "_") {
			return part
		}
	}
	return ""
}

// detectLanguage identifies the documented language from the path. Only
// markdown files reach this package, so the language is carried by a
// directory or by a name like "build.sh.md", never by the file extension.
func detectLanguage(path string) string {
	lowerPath := strings.ToLower(path)

	tokens := strings.FieldsFunc(lowerPath, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r))
	})

	patterns := map[string]string{
		"go":         "go",
		"golang":     "go",
		"python":     "python",
		"py":         "python",
		"javascript": "javascript",
		"js":         "javascript",
		"node":       "javascript",
		"typescript": "typescript",
		"ts":         "typescript",
		"java":       "java",
		"csharp":     "csharp",
		"cs":         "csharp",
		"dotnet":     "csharp",
		"cpp":        "cpp",
		"cxx":        "cpp",
		"rust":       "rust",
		"rs":         "rust",
		"ruby":       "ruby",
		"rb":         "ruby",
		"php":        "php",
		"bash":       "bash",
		"shell":      "bash",
		"sh":         "bash",
		"powershell": "powershell",
		"ps1":        "powershell",
	}

	for _, token := range tokens {
		if lang, ok := patterns[token]; ok {
			return lang
		}
	}

	return ""
}

// isMarkdownFile checks if file has markdown extension
func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

// shouldSkip reports whether a path lies inside a generated or vendored
// directory. Matching is per path segment: a substring match also skipped
// documents whose own name merely contained one of these words.
func shouldSkip(path string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		switch segment {
		case "_site", ".sass-cache", ".jekyll-cache", "node_modules", ".git", "vendor":
			return true
		}
	}

	return false
}
