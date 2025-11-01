package unit

import "aurumcode/pkg/types"

// Language represents supported test languages
type Language string

const (
	LanguageGo         Language = "go"
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
)

// TargetSymbol represents a code symbol to generate tests for
type TargetSymbol struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"` // "function", "class", "method"
	File       string   `json:"file"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	Signature  string   `json:"signature,omitempty"`
	Language   Language `json:"language"`
}

// TestCase represents a generated test case
type TestCase struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Code        string `json:"code"`
}

// GeneratedTest represents a complete test file
type GeneratedTest struct {
	TargetFile string     `json:"target_file"`
	TestFile   string     `json:"test_file"`
	Language   Language   `json:"language"`
	TestCases  []TestCase `json:"test_cases"`
	Hash       string     `json:"hash"` // Hash of target file for idempotency
}

// Generator generates tests for a specific language
type Generator interface {
	// Language returns the language this generator supports
	Language() Language

	// ExtractTargets extracts testable symbols from diff
	ExtractTargets(diff *types.Diff) []TargetSymbol

	// GenerateTests generates test cases for targets
	GenerateTests(targets []TargetSymbol, useLLM bool) ([]GeneratedTest, error)

	// GetTestFilePath returns the path for a test file given source file
	GetTestFilePath(sourceFile string) string
}

// Config configures test generation
type Config struct {
	EnableLLM      bool
	MaxTargets     int
	SkipExisting   bool
	GenerateStubs  bool
}
