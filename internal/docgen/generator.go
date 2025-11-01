package docgen

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/llm"
	"aurumcode/internal/prompt"
	"aurumcode/pkg/types"
	"context"
	"fmt"
)

// Generator generates documentation from code diffs
type Generator struct {
	orchestrator  *llm.Orchestrator
	languageDetector *analyzer.LanguageDetector
	promptBuilder *prompt.PromptBuilder
	parser        *prompt.ResponseParser
}

// NewGenerator creates a new documentation generator
func NewGenerator(orchestrator *llm.Orchestrator) *Generator {
	return &Generator{
		orchestrator:  orchestrator,
		languageDetector: analyzer.NewLanguageDetector(),
		promptBuilder: prompt.NewPromptBuilder(),
		parser:        prompt.NewResponseParser(),
	}
}

// Generate generates documentation for the given diff
func (g *Generator) Generate(ctx context.Context, diff *types.Diff) (string, error) {
	if len(diff.Files) == 0 {
		return "", fmt.Errorf("no files in diff")
	}

	// Detect primary language
	language := g.detectPrimaryLanguage(diff)

	// Build documentation prompt
	docPrompt := g.promptBuilder.BuildDocumentationPrompt(diff, language)

	// Call LLM
	resp, err := g.orchestrator.Complete(ctx, docPrompt, llm.Options{
		MaxTokens:   4000,
		Temperature: 0.5,
	})
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}

	// Parse documentation response
	documentation, err := g.parser.ParseDocumentationResponse(resp.Text)
	if err != nil {
		return "", fmt.Errorf("parse failed: %w", err)
	}

	return documentation, nil
}

// detectPrimaryLanguage detects the primary language from the diff
func (g *Generator) detectPrimaryLanguage(diff *types.Diff) string {
	languageCounts := make(map[string]int)

	for _, file := range diff.Files {
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
