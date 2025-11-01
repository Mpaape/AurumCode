package unit

import (
	"aurumcode/pkg/types"
	"fmt"
	"path/filepath"
	"strings"
)

// PythonGenerator generates Python tests
type PythonGenerator struct {
	extractor *SymbolExtractor
}

// NewPythonGenerator creates a new Python test generator
func NewPythonGenerator() *PythonGenerator {
	return &PythonGenerator{
		extractor: NewSymbolExtractor(),
	}
}

// Language returns the language
func (p *PythonGenerator) Language() Language {
	return LanguagePython
}

// ExtractTargets extracts Python symbols from diff
func (p *PythonGenerator) ExtractTargets(diff *types.Diff) []TargetSymbol {
	allSymbols := p.extractor.Extract(diff)

	var pythonSymbols []TargetSymbol
	for _, symbol := range allSymbols {
		if symbol.Language == LanguagePython {
			pythonSymbols = append(pythonSymbols, symbol)
		}
	}

	return pythonSymbols
}

// GenerateTests generates Python tests
func (p *PythonGenerator) GenerateTests(targets []TargetSymbol, useLLM bool) ([]GeneratedTest, error) {
	fileTargets := make(map[string][]TargetSymbol)
	for _, target := range targets {
		fileTargets[target.File] = append(fileTargets[target.File], target)
	}

	var results []GeneratedTest

	for file, symbols := range fileTargets {
		testFile := p.GetTestFilePath(file)
		hash := computeHash(file)

		var testCases []TestCase
		for _, symbol := range symbols {
			if useLLM {
				testCases = append(testCases, p.generateLLMTest(symbol))
			} else {
				testCases = append(testCases, p.generateStubTest(symbol))
			}
		}

		results = append(results, GeneratedTest{
			TargetFile: file,
			TestFile:   testFile,
			Language:   LanguagePython,
			TestCases:  testCases,
			Hash:       hash,
		})
	}

	return results, nil
}

// GetTestFilePath returns the test file path
func (p *PythonGenerator) GetTestFilePath(sourceFile string) string {
	dir := filepath.Dir(sourceFile)
	base := filepath.Base(sourceFile)

	// Remove .py extension
	base = strings.TrimSuffix(base, ".py")

	// tests/test_module.py
	testsDir := filepath.Join(dir, "tests")
	return filepath.Join(testsDir, "test_"+base+".py")
}

// generateStubTest generates a basic stub test
func (p *PythonGenerator) generateStubTest(symbol TargetSymbol) TestCase {
	testName := "test_" + strings.ToLower(symbol.Name)

	if symbol.Type == "class" {
		code := fmt.Sprintf(`class Test%s:
    def %s(self):
        """Test %s"""
        pytest.skip("Test not yet implemented")`, symbol.Name, testName, symbol.Name)

		return TestCase{
			Name:        "Test" + symbol.Name,
			Description: fmt.Sprintf("Test class for %s", symbol.Name),
			Code:        code,
		}
	}

	// Function test
	code := fmt.Sprintf(`def %s():
    """Test %s function"""
    pytest.skip("Test not yet implemented")`, testName, symbol.Name)

	return TestCase{
		Name:        testName,
		Description: fmt.Sprintf("Test for %s", symbol.Name),
		Code:        code,
	}
}

// generateLLMTest generates an LLM-powered test
func (p *PythonGenerator) generateLLMTest(symbol TargetSymbol) TestCase {
	testName := "test_" + strings.ToLower(symbol.Name)

	code := fmt.Sprintf(`@pytest.mark.parametrize("input_val,expected", [
    (None, None),  # TODO: Add test cases
])
def %s(input_val, expected):
    """Test %s with various inputs"""
    # TODO: Call %s and verify
    pass`, testName, symbol.Name, symbol.Name)

	return TestCase{
		Name:        testName,
		Description: fmt.Sprintf("Parametrized test for %s", symbol.Name),
		Code:        code,
	}
}
