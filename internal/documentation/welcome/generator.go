package welcome

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
)

const (
	defaultPromptPath = ".aurumcode/prompts/documentation/welcome-page.md"
	promptPlaceholder = "{{README_CONTENT}}"
)

// embeddedPromptTemplate is the welcome-page prompt shipped inside the binary.
// Consumers of the Action do not have .aurumcode/prompts/ in their own
// repository, so without this the feature could never run outside AurumCode.
//
//go:embed templates/welcome-page.md
var embeddedPromptTemplate string

var (
	// ErrPromptTemplateNotFound is returned when a prompt template path that the
	// caller supplied explicitly does not exist. The default path is not covered:
	// it falls back to the embedded template instead of failing.
	ErrPromptTemplateNotFound = errors.New("prompt template not found")

	// ErrPromptTemplateInvalid is returned when a prompt template does not
	// contain the README placeholder.
	ErrPromptTemplateInvalid = errors.New("invalid prompt template")

	// ErrNoLLMProvider is returned when generation is attempted without a
	// configured LLM provider. Callers are expected to downgrade this to a
	// warning: welcome page generation is optional.
	ErrNoLLMProvider = errors.New("no LLM provider configured for welcome page generation")
)

// PromptSource identifies which prompt template a generation run used.
type PromptSource string

const (
	// PromptSourceRepository means the template came from the target repository.
	PromptSourceRepository PromptSource = "repository"

	// PromptSourceEmbedded means the template compiled into the binary was used.
	PromptSourceEmbedded PromptSource = "embedded"
)

// Generator creates AI-powered welcome pages from README content
type Generator struct {
	orchestrator *llm.Orchestrator
	promptPath   string
	// promptRequired is set when the caller chose promptPath explicitly. A
	// missing file is then a configuration error rather than a silent fallback.
	promptRequired bool
}

// NewGenerator creates a new welcome page generator
func NewGenerator(orchestrator *llm.Orchestrator) *Generator {
	return &Generator{
		orchestrator: orchestrator,
		promptPath:   defaultPromptPath,
	}
}

// NewGeneratorWithPrompt creates a generator with custom prompt path
func NewGeneratorWithPrompt(orchestrator *llm.Orchestrator, promptPath string) *Generator {
	return &Generator{
		orchestrator:   orchestrator,
		promptPath:     promptPath,
		promptRequired: true,
	}
}

// GenerateOptions provides configuration for welcome page generation
type GenerateOptions struct {
	ReadmePath string // Path to README.md file
	OutputPath string // Path for generated welcome page
	ProjectDir string // Project root directory for resolving paths
	Title      string // Optional custom title override
	NavOrder   int    // Navigation order in site
}

// Generate creates a welcome page from README content using LLM
func (g *Generator) Generate(ctx context.Context, opts GenerateOptions) (string, error) {
	if g.orchestrator == nil {
		return "", ErrNoLLMProvider
	}

	// Read README content
	readmeContent, err := g.readREADME(opts.ReadmePath, opts.ProjectDir)
	if err != nil {
		return "", fmt.Errorf("failed to read README: %w", err)
	}

	// Load prompt template
	promptTemplate, source, err := g.loadPromptTemplate(opts.ProjectDir)
	if err != nil {
		return "", fmt.Errorf("failed to load prompt template: %w", err)
	}
	log.Printf("[Welcome] using %s prompt template", source)

	// Build prompt with README content
	prompt := strings.Replace(promptTemplate, promptPlaceholder, readmeContent, 1)

	// Generate welcome page content using LLM
	llmOpts := llm.DefaultOptions()
	llmOpts.Temperature = 0.7 // More creative for documentation writing
	llmOpts.MaxTokens = 4000
	llmOpts.System = "You are an expert technical writer creating engaging documentation."

	resp, err := g.orchestrator.Complete(ctx, prompt, llmOpts)
	if err != nil {
		return "", fmt.Errorf("LLM generation failed: %w", err)
	}

	// Deterministic safety net around free-form LLM output (AUR-465): the
	// model is not trusted to reliably avoid a mutable Action ref or an
	// invented internal link just because the prompt asked nicely.
	generated, _ := SanitizeActionRef(resp.Text)
	generated, _ = SanitizeInternalLinks(generated)

	// Add Jekyll front matter
	content := g.addFrontMatter(generated, opts)

	// Write to output file if specified
	if opts.OutputPath != "" {
		if err := g.writeOutput(content, opts.OutputPath, opts.ProjectDir); err != nil {
			return "", fmt.Errorf("failed to write output: %w", err)
		}
	}

	return content, nil
}

// readREADME reads and returns README.md content
func (g *Generator) readREADME(readmePath, projectDir string) (string, error) {
	path := readmePath
	if projectDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// loadPromptTemplate resolves the prompt template, preferring one shipped by
// the target repository and falling back to the embedded default.
func (g *Generator) loadPromptTemplate(projectDir string) (string, PromptSource, error) {
	template, source, err := g.resolvePromptTemplate(projectDir)
	if err != nil {
		return "", source, err
	}

	if !strings.Contains(template, promptPlaceholder) {
		return "", source, fmt.Errorf("%w: %s template missing %s placeholder",
			ErrPromptTemplateInvalid, source, promptPlaceholder)
	}

	return template, source, nil
}

// resolvePromptTemplate picks between the repository override and the embedded
// default without validating the contents.
func (g *Generator) resolvePromptTemplate(projectDir string) (string, PromptSource, error) {
	path := g.promptPath
	if path == "" {
		return embeddedPromptTemplate, PromptSourceEmbedded, nil
	}
	if projectDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}

	content, err := os.ReadFile(path)
	if err == nil {
		return string(content), PromptSourceRepository, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return "", PromptSourceRepository, fmt.Errorf("failed to read %s: %w", path, err)
	}

	// An explicitly configured path that is absent is a configuration error;
	// substituting the embedded default would silently ignore the caller.
	if g.promptRequired {
		return "", PromptSourceRepository, fmt.Errorf("%w at %s", ErrPromptTemplateNotFound, path)
	}

	return embeddedPromptTemplate, PromptSourceEmbedded, nil
}

// addFrontMatter adds Jekyll YAML front matter to the content
func (g *Generator) addFrontMatter(content string, opts GenerateOptions) string {
	// Build front matter
	var fm strings.Builder
	fm.WriteString("---\n")
	fm.WriteString("layout: default\n")

	if opts.Title != "" {
		fm.WriteString(fmt.Sprintf("title: %s\n", opts.Title))
	} else {
		fm.WriteString("title: Home\n")
	}

	if opts.NavOrder > 0 {
		fm.WriteString(fmt.Sprintf("nav_order: %d\n", opts.NavOrder))
	} else {
		fm.WriteString("nav_order: 1\n")
	}

	fm.WriteString("description: Welcome to the documentation\n")
	fm.WriteString("permalink: /\n")
	fm.WriteString("---\n\n")

	// Strip any existing front matter from LLM output
	cleanContent := stripFrontMatter(content)

	return fm.String() + cleanContent
}

// stripFrontMatter removes any existing YAML front matter from content
func stripFrontMatter(content string) string {
	// If content starts with ---, remove everything until second ---
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[2])
		}
	}
	return strings.TrimSpace(content)
}

// writeOutput writes the generated content to a file
func (g *Generator) writeOutput(content, outputPath, projectDir string) error {
	path := outputPath
	if projectDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0644)
}
