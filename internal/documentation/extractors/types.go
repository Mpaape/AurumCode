package extractors

import "context"

// Language represents a programming language
type Language string

// Supported languages
const (
	LanguageGo         Language = "go"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
	LanguageCSharp     Language = "csharp"
	LanguageCPP        Language = "cpp"
	LanguageRust       Language = "rust"
	LanguageBash       Language = "bash"
	LanguagePowerShell Language = "powershell"
	LanguageJava       Language = "java"
)

// AllLanguages returns all supported languages
func AllLanguages() []Language {
	return []Language{
		LanguageGo,
		LanguageJavaScript,
		LanguageTypeScript,
		LanguagePython,
		LanguageCSharp,
		LanguageCPP,
		LanguageRust,
		LanguageBash,
		LanguagePowerShell,
		LanguageJava,
	}
}

// IsValid checks if a language is supported
func (l Language) IsValid() bool {
	for _, lang := range AllLanguages() {
		if l == lang {
			return true
		}
	}
	return false
}

// String returns the string representation
func (l Language) String() string {
	return string(l)
}

// ExtractRequest defines parameters for documentation extraction
type ExtractRequest struct {
	// Language to extract documentation for
	Language Language

	// SourceDir is the root directory containing source files
	SourceDir string

	// OutputDir is where extracted markdown will be written
	OutputDir string

	// Options for extractor-specific configuration
	Options map[string]interface{}
}

// ExtractResult contains the result of documentation extraction
type ExtractResult struct {
	// Language that was processed
	Language Language

	// Files that were generated
	Files []string

	// Stats about the extraction
	Stats ExtractionStats

	// Errors encountered (non-fatal)
	Errors []error
}

// ExtractionStats contains statistics about the extraction
type ExtractionStats struct {
	// FilesProcessed is the number of source files processed
	FilesProcessed int

	// DocsGenerated is the number of markdown files generated
	DocsGenerated int

	// LinesProcessed is the total lines of source code processed
	LinesProcessed int

	// Duration in milliseconds
	Duration int64
}

// Extractor defines the interface for language-specific documentation extractors
type Extractor interface {
	// Extract generates documentation from source code
	Extract(ctx context.Context, req *ExtractRequest) (*ExtractResult, error)

	// Validate checks if the extractor's dependencies are available
	Validate(ctx context.Context) error

	// Language returns the language this extractor handles
	Language() Language
}
