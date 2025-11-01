package unit

import (
	"aurumcode/pkg/types"
	"fmt"
)

// Orchestrator coordinates test generation across languages
type Orchestrator struct {
	generators map[Language]Generator
	config     *Config
}

// NewOrchestrator creates a new test generation orchestrator
func NewOrchestrator(config *Config) *Orchestrator {
	if config == nil {
		config = &Config{
			EnableLLM:     false,
			MaxTargets:    100,
			SkipExisting:  true,
			GenerateStubs: true,
		}
	}

	return &Orchestrator{
		generators: map[Language]Generator{
			LanguageGo:         NewGoGenerator(),
			LanguagePython:     NewPythonGenerator(),
			LanguageJavaScript: NewJSGenerator(LanguageJavaScript),
			LanguageTypeScript: NewJSGenerator(LanguageTypeScript),
		},
		config: config,
	}
}

// GenerateTests generates tests for all changed code in the diff
func (o *Orchestrator) GenerateTests(diff *types.Diff) (map[Language][]GeneratedTest, error) {
	results := make(map[Language][]GeneratedTest)

	for lang, generator := range o.generators {
		// Extract targets for this language
		targets := generator.ExtractTargets(diff)
		if len(targets) == 0 {
			continue
		}

		// Limit targets if configured
		if o.config.MaxTargets > 0 && len(targets) > o.config.MaxTargets {
			targets = targets[:o.config.MaxTargets]
		}

		// Generate tests
		tests, err := generator.GenerateTests(targets, o.config.EnableLLM)
		if err != nil {
			return nil, fmt.Errorf("generate tests for %s: %w", lang, err)
		}

		if len(tests) > 0 {
			results[lang] = tests
		}
	}

	return results, nil
}

// GenerateForLanguage generates tests for a specific language
func (o *Orchestrator) GenerateForLanguage(diff *types.Diff, lang Language) ([]GeneratedTest, error) {
	generator, ok := o.generators[lang]
	if !ok {
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}

	targets := generator.ExtractTargets(diff)
	if len(targets) == 0 {
		return []GeneratedTest{}, nil
	}

	if o.config.MaxTargets > 0 && len(targets) > o.config.MaxTargets {
		targets = targets[:o.config.MaxTargets]
	}

	return generator.GenerateTests(targets, o.config.EnableLLM)
}

// GetSupportedLanguages returns all supported languages
func (o *Orchestrator) GetSupportedLanguages() []Language {
	languages := make([]Language, 0, len(o.generators))
	for lang := range o.generators {
		languages = append(languages, lang)
	}
	return languages
}

// CountTargets counts testable targets in a diff per language
func (o *Orchestrator) CountTargets(diff *types.Diff) map[Language]int {
	counts := make(map[Language]int)

	for lang, generator := range o.generators {
		targets := generator.ExtractTargets(diff)
		if len(targets) > 0 {
			counts[lang] = len(targets)
		}
	}

	return counts
}
