package unit

import (
	"aurumcode/pkg/types"
	"path/filepath"
	"regexp"
	"strings"
)

// SymbolExtractor extracts testable symbols from diffs
type SymbolExtractor struct{}

// NewSymbolExtractor creates a new symbol extractor
func NewSymbolExtractor() *SymbolExtractor {
	return &SymbolExtractor{}
}

// Extract extracts symbols from a diff
func (e *SymbolExtractor) Extract(diff *types.Diff) []TargetSymbol {
	var symbols []TargetSymbol

	for _, file := range diff.Files {
		// Skip test files
		if isTestFile(file.Path) {
			continue
		}

		// Determine language
		lang := detectLanguage(file.Path)
		if lang == "" {
			continue
		}

		// Extract symbols based on language
		fileSymbols := e.extractFromFile(file, lang)
		symbols = append(symbols, fileSymbols...)
	}

	return symbols
}

// extractFromFile extracts symbols from a specific file
func (e *SymbolExtractor) extractFromFile(file types.DiffFile, lang Language) []TargetSymbol {
	var symbols []TargetSymbol

	// Combine hunks into full added content
	var addedLines []string
	currentLine := 0

	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				addedLines = append(addedLines, line[1:])
				currentLine++
			}
		}
	}

	content := strings.Join(addedLines, "\n")

	// Extract based on language
	switch lang {
	case LanguageGo:
		symbols = append(symbols, e.extractGoSymbols(file.Path, content)...)
	case LanguagePython:
		symbols = append(symbols, e.extractPythonSymbols(file.Path, content)...)
	case LanguageJavaScript, LanguageTypeScript:
		symbols = append(symbols, e.extractJSSymbols(file.Path, content, lang)...)
	}

	return symbols
}

// extractGoSymbols extracts Go functions
func (e *SymbolExtractor) extractGoSymbols(filePath string, content string) []TargetSymbol {
	var symbols []TargetSymbol

	// Match Go functions: func Name(...) ...
	funcRegex := regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?([A-Z]\w+)\s*\([^)]*\)`)
	matches := funcRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			symbols = append(symbols, TargetSymbol{
				Name:     match[1],
				Type:     "function",
				File:     filePath,
				Language: LanguageGo,
			})
		}
	}

	return symbols
}

// extractPythonSymbols extracts Python functions and classes
func (e *SymbolExtractor) extractPythonSymbols(filePath string, content string) []TargetSymbol {
	var symbols []TargetSymbol

	// Match Python functions: def name(...):
	funcRegex := regexp.MustCompile(`def\s+(\w+)\s*\([^)]*\):`)
	matches := funcRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			name := match[1]
			// Skip private functions (starting with _)
			if !strings.HasPrefix(name, "_") {
				symbols = append(symbols, TargetSymbol{
					Name:     name,
					Type:     "function",
					File:     filePath,
					Language: LanguagePython,
				})
			}
		}
	}

	// Match Python classes: class Name:
	classRegex := regexp.MustCompile(`class\s+(\w+)(?:\([^)]*\))?:`)
	classMatches := classRegex.FindAllStringSubmatch(content, -1)

	for _, match := range classMatches {
		if len(match) >= 2 {
			symbols = append(symbols, TargetSymbol{
				Name:     match[1],
				Type:     "class",
				File:     filePath,
				Language: LanguagePython,
			})
		}
	}

	return symbols
}

// extractJSSymbols extracts JavaScript/TypeScript functions and classes
func (e *SymbolExtractor) extractJSSymbols(filePath string, content string, lang Language) []TargetSymbol {
	var symbols []TargetSymbol

	// Match function declarations: function name(...) and export function name(...)
	funcRegex := regexp.MustCompile(`(?:export\s+)?function\s+(\w+)\s*\([^)]*\)`)
	matches := funcRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			symbols = append(symbols, TargetSymbol{
				Name:     match[1],
				Type:     "function",
				File:     filePath,
				Language: lang,
			})
		}
	}

	// Match arrow functions: const name = (...) =>
	arrowRegex := regexp.MustCompile(`(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`)
	arrowMatches := arrowRegex.FindAllStringSubmatch(content, -1)

	for _, match := range arrowMatches {
		if len(match) >= 2 {
			symbols = append(symbols, TargetSymbol{
				Name:     match[1],
				Type:     "function",
				File:     filePath,
				Language: lang,
			})
		}
	}

	// Match classes: class Name or export class Name
	classRegex := regexp.MustCompile(`(?:export\s+)?class\s+(\w+)`)
	classMatches := classRegex.FindAllStringSubmatch(content, -1)

	for _, match := range classMatches {
		if len(match) >= 2 {
			symbols = append(symbols, TargetSymbol{
				Name:     match[1],
				Type:     "class",
				File:     filePath,
				Language: lang,
			})
		}
	}

	return symbols
}

// isTestFile checks if a file is a test file
func isTestFile(path string) bool {
	base := filepath.Base(path)

	// Go test files
	if strings.HasSuffix(base, "_test.go") {
		return true
	}

	// Python test files
	if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
		return true
	}

	// JavaScript/TypeScript test files
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}

	// Test directories
	dir := filepath.Dir(path)
	if strings.Contains(dir, "/__tests__/") || strings.Contains(dir, "/tests/") {
		return true
	}

	return false
}

// detectLanguage detects language from file extension
func detectLanguage(path string) Language {
	ext := filepath.Ext(path)

	switch ext {
	case ".go":
		return LanguageGo
	case ".py":
		return LanguagePython
	case ".js", ".jsx":
		return LanguageJavaScript
	case ".ts", ".tsx":
		return LanguageTypeScript
	default:
		return ""
	}
}
