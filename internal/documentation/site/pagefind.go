package site

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// PagefindBuilder builds search index using Pagefind
type PagefindBuilder struct {
	runner CommandRunner
}

// NewPagefindBuilder creates a new Pagefind builder
func NewPagefindBuilder(runner CommandRunner) *PagefindBuilder {
	return &PagefindBuilder{
		runner: runner,
	}
}

// Build builds the Pagefind search index
func (p *PagefindBuilder) Build(ctx context.Context, workdir string) error {
	// Validate first
	if err := p.Validate(ctx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Default to public/ directory
	sourceDir := filepath.Join(workdir, "public")

	// Build command arguments
	args := []string{"--source", sourceDir}

	// Run Pagefind via npx
	_, err := p.runner.Run(ctx, "npx", append([]string{"pagefind"}, args...), workdir, nil)
	if err != nil {
		return fmt.Errorf("pagefind build failed: %w", err)
	}

	return nil
}

// BuildWithSource builds with a custom source directory
func (p *PagefindBuilder) BuildWithSource(ctx context.Context, workdir string, sourceDir string) error {
	args := []string{"--source", sourceDir}

	_, err := p.runner.Run(ctx, "npx", append([]string{"pagefind"}, args...), workdir, nil)
	if err != nil {
		return fmt.Errorf("pagefind build failed: %w", err)
	}

	return nil
}

// Validate checks if Pagefind is available
func (p *PagefindBuilder) Validate(ctx context.Context) error {
	version, err := p.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("pagefind not available: %w", err)
	}

	if version == "" {
		return fmt.Errorf("could not determine pagefind version")
	}

	return nil
}

// GetVersion returns the Pagefind version
func (p *PagefindBuilder) GetVersion(ctx context.Context) (string, error) {
	output, err := p.runner.Run(ctx, "npx", []string{"pagefind", "--version"}, ".", nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// BuildWithConfig builds with custom configuration
func (p *PagefindBuilder) BuildWithConfig(ctx context.Context, config *BuildConfig) error {
	sourceDir := config.OutputDir
	if sourceDir == "" {
		sourceDir = filepath.Join(config.WorkDir, "public")
	}

	args := []string{"--source", sourceDir}

	_, err := p.runner.Run(ctx, "npx", append([]string{"pagefind"}, args...), config.WorkDir, config.Env)
	if err != nil {
		return fmt.Errorf("pagefind build failed: %w", err)
	}

	return nil
}
