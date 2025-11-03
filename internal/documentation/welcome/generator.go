package welcome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aurumcode/internal/llm"
)

const (
	defaultPromptPath = ".aurumcode/prompts/documentation/welcome-page.md"
	promptPlaceholder = "{{README_CONTENT}}"
)

// Generator creates AI-powered welcome pages from README content
type Generator struct {
	orchestrator *llm.Orchestrator
	promptPath   string
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
		orchestrator: orchestrator,
		promptPath:   promptPath,
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
	// Read README content
	readmeContent, err := g.readREADME(opts.ReadmePath, opts.ProjectDir)
	if err != nil {
		return "", fmt.Errorf("failed to read README: %w", err)
	}

	// Load prompt template
	promptTemplate, err := g.loadPromptTemplate(opts.ProjectDir)
	if err != nil {
		return "", fmt.Errorf("failed to load prompt template: %w", err)
	}

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

	// Add Jekyll front matter
	content := g.addFrontMatter(resp.Text, opts)

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

// loadPromptTemplate loads the prompt template file
func (g *Generator) loadPromptTemplate(projectDir string) (string, error) {
	path := g.promptPath
	if projectDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}

	// Check if custom prompt exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("prompt template not found at %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	template := string(content)

	// Validate template has placeholder
	if !strings.Contains(template, promptPlaceholder) {
		return "", fmt.Errorf("prompt template missing %s placeholder", promptPlaceholder)
	}

	return template, nil
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
