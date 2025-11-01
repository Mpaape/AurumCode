package unit

import (
	"aurumcode/pkg/types"
	"fmt"
	"path/filepath"
	"strings"
)

// JSGenerator generates JavaScript/TypeScript tests
type JSGenerator struct {
	extractor *SymbolExtractor
	language  Language
}

// NewJSGenerator creates a new JS/TS test generator
func NewJSGenerator(lang Language) *JSGenerator {
	return &JSGenerator{
		extractor: NewSymbolExtractor(),
		language:  lang,
	}
}

// Language returns the language
func (j *JSGenerator) Language() Language {
	return j.language
}

// ExtractTargets extracts JS/TS symbols from diff
func (j *JSGenerator) ExtractTargets(diff *types.Diff) []TargetSymbol {
	allSymbols := j.extractor.Extract(diff)

	var jsSymbols []TargetSymbol
	for _, symbol := range allSymbols {
		if symbol.Language == j.language {
			jsSymbols = append(jsSymbols, symbol)
		}
	}

	return jsSymbols
}

// GenerateTests generates JS/TS tests
func (j *JSGenerator) GenerateTests(targets []TargetSymbol, useLLM bool) ([]GeneratedTest, error) {
	fileTargets := make(map[string][]TargetSymbol)
	for _, target := range targets {
		fileTargets[target.File] = append(fileTargets[target.File], target)
	}

	var results []GeneratedTest

	for file, symbols := range fileTargets {
		testFile := j.GetTestFilePath(file)
		hash := computeHash(file)

		var testCases []TestCase
		for _, symbol := range symbols {
			if useLLM {
				testCases = append(testCases, j.generateLLMTest(symbol))
			} else {
				testCases = append(testCases, j.generateStubTest(symbol))
			}
		}

		results = append(results, GeneratedTest{
			TargetFile: file,
			TestFile:   testFile,
			Language:   j.language,
			TestCases:  testCases,
			Hash:       hash,
		})
	}

	return results, nil
}

// GetTestFilePath returns the test file path
func (j *JSGenerator) GetTestFilePath(sourceFile string) string {
	ext := filepath.Ext(sourceFile)
	base := strings.TrimSuffix(sourceFile, ext)

	// Place in __tests__ directory
	dir := filepath.Dir(base)
	filename := filepath.Base(base)

	return filepath.Join(dir, "__tests__", filename+".test"+ext)
}

// generateStubTest generates a basic stub test
func (j *JSGenerator) generateStubTest(symbol TargetSymbol) TestCase {
	testName := fmt.Sprintf("'%s'", symbol.Name)

	code := fmt.Sprintf(`describe(%s, () => {
  it('should work', () => {
    // TODO: Implement test
    expect(true).toBe(true);
  });
});`, testName)

	return TestCase{
		Name:        symbol.Name,
		Description: fmt.Sprintf("Test for %s", symbol.Name),
		Code:        code,
	}
}

// generateLLMTest generates an LLM-powered test
func (j *JSGenerator) generateLLMTest(symbol TargetSymbol) TestCase {
	testName := fmt.Sprintf("'%s'", symbol.Name)

	code := fmt.Sprintf(`describe(%s, () => {
  it.each([
    // TODO: Add test cases
    { input: null, expected: null },
  ])('should handle %%p', ({ input, expected }) => {
    // TODO: Call %s and verify
    expect(true).toBe(true);
  });
});`, testName, symbol.Name)

	return TestCase{
		Name:        symbol.Name,
		Description: fmt.Sprintf("Parameterized test for %s", symbol.Name),
		Code:        code,
	}
}
