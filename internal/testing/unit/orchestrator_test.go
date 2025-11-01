package unit

import (
	"aurumcode/pkg/types"
	"testing"
)

func TestOrchestrator(t *testing.T) {
	config := &Config{
		EnableLLM:     false,
		MaxTargets:    10,
		SkipExisting:  true,
		GenerateStubs: true,
	}

	orch := NewOrchestrator(config)

	// Create test diff with Go file
	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "internal/example/service.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+package example",
							"+",
							"+func ProcessData(input string) string {",
							"+	return input",
							"+}",
						},
					},
				},
			},
		},
	}

	// Generate tests
	results, err := orch.GenerateTests(diff)
	if err != nil {
		t.Fatalf("GenerateTests failed: %v", err)
	}

	// Should have Go tests
	goTests, ok := results[LanguageGo]
	if !ok {
		t.Fatal("Expected Go tests")
	}

	if len(goTests) == 0 {
		t.Error("Expected at least one generated test")
	}

	// Verify test structure
	test := goTests[0]
	if test.Language != LanguageGo {
		t.Errorf("Language = %v, want %v", test.Language, LanguageGo)
	}

	if len(test.TestCases) == 0 {
		t.Error("Expected test cases")
	}
}

func TestOrchestratorMultipleLanguages(t *testing.T) {
	orch := NewOrchestrator(nil)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "service.go",
				Hunks: []types.DiffHunk{
					{Lines: []string{"+func GoFunc() {}"}},
				},
			},
			{
				Path: "service.py",
				Hunks: []types.DiffHunk{
					{Lines: []string{"+def python_func():"}},
				},
			},
			{
				Path: "service.ts",
				Hunks: []types.DiffHunk{
					{Lines: []string{"+function tsFunc() {}"}},
				},
			},
		},
	}

	results, err := orch.GenerateTests(diff)
	if err != nil {
		t.Fatalf("GenerateTests failed: %v", err)
	}

	// Should have tests for all languages
	expectedLangs := []Language{LanguageGo, LanguagePython, LanguageTypeScript}
	for _, lang := range expectedLangs {
		if _, ok := results[lang]; !ok {
			t.Errorf("Missing tests for language: %v", lang)
		}
	}
}

func TestCountTargets(t *testing.T) {
	orch := NewOrchestrator(nil)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "service.go",
				Hunks: []types.DiffHunk{
					{Lines: []string{
						"+func Func1() {}",
						"+func Func2() {}",
					}},
				},
			},
		},
	}

	counts := orch.CountTargets(diff)

	if counts[LanguageGo] != 2 {
		t.Errorf("Go targets = %d, want 2", counts[LanguageGo])
	}
}

func TestGenerateForLanguage(t *testing.T) {
	orch := NewOrchestrator(nil)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "service.go",
				Hunks: []types.DiffHunk{
					{Lines: []string{"+func TestFunc() {}"}},
				},
			},
		},
	}

	tests, err := orch.GenerateForLanguage(diff, LanguageGo)
	if err != nil {
		t.Fatalf("GenerateForLanguage failed: %v", err)
	}

	if len(tests) == 0 {
		t.Error("Expected generated tests")
	}
}

func TestUnsupportedLanguage(t *testing.T) {
	orch := NewOrchestrator(nil)

	diff := &types.Diff{Files: []types.DiffFile{}}

	_, err := orch.GenerateForLanguage(diff, Language("unsupported"))
	if err == nil {
		t.Error("Expected error for unsupported language")
	}
}

func TestMaxTargetsLimit(t *testing.T) {
	config := &Config{
		MaxTargets: 1,
	}

	orch := NewOrchestrator(config)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "service.go",
				Hunks: []types.DiffHunk{
					{Lines: []string{
						"+func Func1() {}",
						"+func Func2() {}",
						"+func Func3() {}",
					}},
				},
			},
		},
	}

	results, err := orch.GenerateTests(diff)
	if err != nil {
		t.Fatal(err)
	}

	goTests := results[LanguageGo]
	// Should be limited by config
	totalCases := 0
	for _, test := range goTests {
		totalCases += len(test.TestCases)
	}

	if totalCases > config.MaxTargets {
		t.Errorf("Total test cases %d exceeds limit %d", totalCases, config.MaxTargets)
	}
}
