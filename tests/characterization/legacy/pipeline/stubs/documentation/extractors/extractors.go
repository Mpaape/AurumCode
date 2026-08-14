package extractors

import (
	"context"
	"errors"
	"fmt"
)

type Language string

const (
	LanguageGo         Language = "go"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguagePython     Language = "python"
	LanguageCSharp     Language = "csharp"
	LanguageJava       Language = "java"
	LanguageCPP        Language = "cpp"
	LanguageRust       Language = "rust"
	LanguageBash       Language = "bash"
	LanguagePowerShell Language = "powershell"
)

type ExtractionStats struct {
	FilesProcessed int
	DocsGenerated  int
}

type ExtractRequest struct {
	Language  Language
	SourceDir string
	OutputDir string
}

type ExtractResult struct {
	Stats  ExtractionStats
	Files  []string
	Errors []error
}

type Extractor interface {
	Language() Language
	Validate(context.Context) error
	Extract(context.Context, *ExtractRequest) (*ExtractResult, error)
}

type Registry struct {
	entries map[Language]Extractor
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[Language]Extractor)}
}

func (r *Registry) Register(extractor Extractor) error {
	if extractor == nil {
		return errors.New("nil extractor")
	}
	lang := extractor.Language()
	if lang == "" {
		return errors.New("empty extractor language")
	}
	if _, exists := r.entries[lang]; exists {
		return fmt.Errorf("extractor already registered: %s", lang)
	}
	r.entries[lang] = extractor
	return nil
}

func (r *Registry) Get(language Language) (Extractor, error) {
	extractor, ok := r.entries[language]
	if !ok {
		return nil, fmt.Errorf("extractor not registered: %s", language)
	}
	return extractor, nil
}

type missingTool interface {
	MissingTool() string
}

func MissingTool(err error) (string, bool) {
	var missing missingTool
	if errors.As(err, &missing) {
		return missing.MissingTool(), true
	}
	return "", false
}
