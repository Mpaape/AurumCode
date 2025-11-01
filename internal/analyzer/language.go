package analyzer

import (
	"path/filepath"
	"strings"
)

// LanguageDetector detects programming languages from file extensions
type LanguageDetector struct {
	extensionMap map[string]string
}

// NewLanguageDetector creates a new language detector
func NewLanguageDetector() *LanguageDetector {
	return &LanguageDetector{
		extensionMap: buildExtensionMap(),
	}
}

// DetectLanguage detects the programming language from a file path
func (d *LanguageDetector) DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == "" {
		// Check for extensionless files
		base := filepath.Base(filePath)
		if lang, ok := d.extensionMap[base]; ok {
			return lang
		}
		return "unknown"
	}

	// Remove the dot from extension
	ext = strings.TrimPrefix(ext, ".")

	if lang, ok := d.extensionMap[ext]; ok {
		return lang
	}

	return "unknown"
}

// buildExtensionMap creates the mapping of file extensions to languages
func buildExtensionMap() map[string]string {
	return map[string]string{
		// Go
		"go":   "go",
		"mod":  "go",
		"sum":  "go",

		// JavaScript/TypeScript
		"js":   "javascript",
		"jsx":  "javascript",
		"ts":   "typescript",
		"tsx":  "typescript",
		"mjs":  "javascript",
		"cjs":  "javascript",

		// Python
		"py":   "python",
		"pyw":  "python",
		"pyx":  "python",
		"pyi":  "python",

		// Java/Kotlin
		"java": "java",
		"kt":   "kotlin",
		"kts":  "kotlin",

		// C/C++
		"c":    "c",
		"h":    "c",
		"cpp":  "cpp",
		"cc":   "cpp",
		"cxx":  "cpp",
		"hpp":  "cpp",
		"hxx":  "cpp",
		"hh":   "cpp",

		// C#
		"cs":   "csharp",
		"csx":  "csharp",

		// Rust
		"rs":   "rust",

		// Ruby
		"rb":   "ruby",
		"erb":  "ruby",

		// PHP
		"php":  "php",
		"phtml": "php",

		// Swift
		"swift": "swift",

		// Objective-C
		"m":    "objective-c",
		"mm":   "objective-c",

		// Shell
		"sh":   "shell",
		"bash": "shell",
		"zsh":  "shell",

		// Web
		"html": "html",
		"htm":  "html",
		"css":  "css",
		"scss": "scss",
		"sass": "sass",
		"less": "less",

		// Config/Data
		"json": "json",
		"yaml": "yaml",
		"yml":  "yaml",
		"toml": "toml",
		"xml":  "xml",
		"ini":  "ini",

		// Markdown/Documentation
		"md":   "markdown",
		"mdx":  "markdown",
		"rst":  "restructuredtext",

		// SQL
		"sql":  "sql",

		// Docker
		"dockerfile": "docker",
		"Dockerfile": "docker",

		// Makefiles
		"makefile": "make",
		"Makefile": "make",
		"mk":       "make",

		// Others
		"graphql": "graphql",
		"proto":   "protobuf",
		"thrift":  "thrift",
	}
}

// IsTestFile checks if a file is a test file based on naming conventions
func (d *LanguageDetector) IsTestFile(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))

	// Go test files
	if strings.HasSuffix(base, "_test.go") {
		return true
	}

	// JavaScript/TypeScript test files
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}

	// Python test files
	if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
		return true
	}

	// Java test files (convention)
	if strings.HasSuffix(base, "test.java") || strings.HasSuffix(base, "tests.java") {
		return true
	}

	// Check directory structure
	lowerPath := strings.ToLower(filePath)
	testDirs := []string{"/test/", "/tests/", "/__tests__/", "/spec/", "/specs/"}
	for _, dir := range testDirs {
		if strings.Contains(lowerPath, dir) {
			return true
		}
	}

	return false
}

// IsConfigFile checks if a file is a configuration file
func (d *LanguageDetector) IsConfigFile(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))

	configFiles := []string{
		"package.json",
		"tsconfig.json",
		"go.mod",
		"go.sum",
		"cargo.toml",
		"requirements.txt",
		"pyproject.toml",
		"setup.py",
		"pom.xml",
		"build.gradle",
		"dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"makefile",
		".gitignore",
		".dockerignore",
		".env",
		".env.example",
	}

	for _, cf := range configFiles {
		if base == cf {
			return true
		}
	}

	// Check extensions
	ext := strings.ToLower(filepath.Ext(filePath))
	configExts := []string{".json", ".yaml", ".yml", ".toml", ".ini", ".env", ".config"}
	for _, ce := range configExts {
		if ext == ce {
			return true
		}
	}

	return false
}

// GetLanguageCategory returns the category of a language
func (d *LanguageDetector) GetLanguageCategory(language string) string {
	switch language {
	case "go", "python", "java", "kotlin", "c", "cpp", "csharp", "rust", "ruby", "php", "swift", "objective-c":
		return "backend"
	case "javascript", "typescript", "html", "css", "scss", "sass", "less":
		return "frontend"
	case "sql":
		return "database"
	case "shell", "make", "docker":
		return "infrastructure"
	case "json", "yaml", "toml", "xml", "ini":
		return "config"
	case "markdown", "restructuredtext":
		return "documentation"
	default:
		return "other"
	}
}
