package unit

import (
	"aurumcode/pkg/types"
	"crypto/md5"
	"fmt"
	"path/filepath"
	"strings"
)

// GoGenerator generates Go tests
type GoGenerator struct {
	extractor *SymbolExtractor
}

// NewGoGenerator creates a new Go test generator
func NewGoGenerator() *GoGenerator {
	return &GoGenerator{
		extractor: NewSymbolExtractor(),
	}
}

// Language returns the language
func (g *GoGenerator) Language() Language {
	return LanguageGo
}

// ExtractTargets extracts Go symbols from diff
func (g *GoGenerator) ExtractTargets(diff *types.Diff) []TargetSymbol {
	allSymbols := g.extractor.Extract(diff)

	// Filter for Go symbols
	var goSymbols []TargetSymbol
	for _, symbol := range allSymbols {
		if symbol.Language == LanguageGo {
			goSymbols = append(goSymbols, symbol)
		}
	}

	return goSymbols
}

// GenerateTests generates Go tests
func (g *GoGenerator) GenerateTests(targets []TargetSymbol, useLLM bool) ([]GeneratedTest, error) {
	// Group targets by file
	fileTargets := make(map[string][]TargetSymbol)
	for _, target := range targets {
		fileTargets[target.File] = append(fileTargets[target.File], target)
	}

	var results []GeneratedTest

	for file, symbols := range fileTargets {
		testFile := g.GetTestFilePath(file)
		hash := computeHash(file)

		var testCases []TestCase
		for _, symbol := range symbols {
			if useLLM {
				// LLM-generated test (placeholder - would call LLM)
				testCases = append(testCases, g.generateLLMTest(symbol))
			} else {
				// Generate stub
				testCases = append(testCases, g.generateStubTest(symbol))
			}
		}

		results = append(results, GeneratedTest{
			TargetFile: file,
			TestFile:   testFile,
			Language:   LanguageGo,
			TestCases:  testCases,
			Hash:       hash,
		})
	}

	return results, nil
}

// GetTestFilePath returns the test file path for a source file
func (g *GoGenerator) GetTestFilePath(sourceFile string) string {
	ext := filepath.Ext(sourceFile)
	base := strings.TrimSuffix(sourceFile, ext)
	return base + "_test.go"
}

// generateStubTest generates a basic stub test
func (g *GoGenerator) generateStubTest(symbol TargetSymbol) TestCase {
	testName := "Test" + symbol.Name
	code := fmt.Sprintf(`func %s(t *testing.T) {
	// TODO: Implement test for %s
	t.Skip("Test not yet implemented")
}`, testName, symbol.Name)

	return TestCase{
		Name:        testName,
		Description: fmt.Sprintf("Test for %s", symbol.Name),
		Code:        code,
	}
}

// generateLLMTest generates an LLM-powered test (placeholder)
func (g *GoGenerator) generateLLMTest(symbol TargetSymbol) TestCase {
	// In real implementation, this would call the LLM
	// For now, return a table-driven test template
	testName := "Test" + symbol.Name
	code := fmt.Sprintf(`func %s(t *testing.T) {
	tests := []struct {
		name string
		want interface{}
	}{
		{
			name: "basic case",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: Call %s and verify
		})
	}
}`, testName, symbol.Name)

	return TestCase{
		Name:        testName,
		Description: fmt.Sprintf("Table-driven test for %s", symbol.Name),
		Code:        code,
	}
}

// computeHash computes MD5 hash of a file path (simplified)
func computeHash(path string) string {
	hash := md5.Sum([]byte(path))
	return fmt.Sprintf("%x", hash)
}
