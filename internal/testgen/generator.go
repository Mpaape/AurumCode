package testgen

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/llm"
	"aurumcode/internal/prompt"
	"aurumcode/pkg/types"
	"context"
	"fmt"
)

// Generator generates tests from code diffs
type Generator struct {
	orchestrator     *llm.Orchestrator
	languageDetector *analyzer.LanguageDetector
	promptBuilder    *prompt.PromptBuilder
	parser           *prompt.ResponseParser
}

// NewGenerator creates a new test generator
func NewGenerator(orchestrator *llm.Orchestrator) *Generator {
	return &Generator{
		orchestrator:     orchestrator,
		languageDetector: analyzer.NewLanguageDetector(),
		promptBuilder:    prompt.NewPromptBuilder(),
		parser:           prompt.NewResponseParser(),
	}
}

// Generate generates tests for the given diff
func (g *Generator) Generate(ctx context.Context, diff *types.Diff) (string, error) {
	if len(diff.Files) == 0 {
		return "", fmt.Errorf("no files in diff")
	}

	// Detect primary language
	language := g.detectPrimaryLanguage(diff)
	if language == "unknown" {
		return "", fmt.Errorf("unable to detect language for test generation")
	}

	// Build test prompt
	testPrompt := g.promptBuilder.BuildTestPrompt(diff, language)

	// Call LLM
	resp, err := g.orchestrator.Complete(ctx, testPrompt, llm.Options{
		MaxTokens:   4000,
		Temperature: 0.4,
	})
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}

	// Parse test response
	tests, err := g.parser.ParseTestResponse(resp.Text, language)
	if err != nil {
		return "", fmt.Errorf("parse failed: %w", err)
	}

	return tests, nil
}

// detectPrimaryLanguage detects the primary language from the diff
// Excludes test files to avoid generating tests for tests
func (g *Generator) detectPrimaryLanguage(diff *types.Diff) string {
	languageCounts := make(map[string]int)

	for _, file := range diff.Files {
		// Skip test files
		if g.languageDetector.IsTestFile(file.Path) {
			continue
		}

		lang := g.languageDetector.DetectLanguage(file.Path)
		if lang != "unknown" {
			languageCounts[lang]++
		}
	}

	// Find most common language
	maxCount := 0
	primaryLang := "unknown"
	for lang, count := range languageCounts {
		if count > maxCount {
			maxCount = count
			primaryLang = lang
		}
	}

	return primaryLang
}
