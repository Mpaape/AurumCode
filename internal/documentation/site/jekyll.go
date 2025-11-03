package site

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// JekyllBuilder builds sites using Jekyll
type JekyllBuilder struct {
	runner          CommandRunner
	requiredVersion string
}

// NewJekyllBuilder creates a new Jekyll builder
func NewJekyllBuilder(runner CommandRunner) *JekyllBuilder {
	return &JekyllBuilder{
		runner:          runner,
		requiredVersion: "4.3", // Jekyll 4.3+
	}
}

// WithRequiredVersion sets the required Jekyll version
func (j *JekyllBuilder) WithRequiredVersion(version string) *JekyllBuilder {
	j.requiredVersion = version
	return j
}

// Build builds the Jekyll site
func (j *JekyllBuilder) Build(ctx context.Context, workdir string) error {
	// Validate first
	if err := j.Validate(ctx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Build command arguments
	args := []string{"build"}

	// Run Jekyll build
	output, err := j.runner.Run(ctx, "jekyll", args, workdir, nil)
	if err != nil {
		return fmt.Errorf("jekyll build failed: %w", err)
	}

	// Check for success indicators in output
	if !strings.Contains(output, "done") && !strings.Contains(output, "generated") {
		return fmt.Errorf("unexpected build output: %s", output)
	}

	return nil
}

// Validate checks if Jekyll is available
func (j *JekyllBuilder) Validate(ctx context.Context) error {
	version, err := j.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("jekyll not found: %w", err)
	}

	// Check version if required version is set
	if j.requiredVersion != "" {
		// Extract version number from output like "jekyll 4.3.3"
		if !strings.Contains(version, j.requiredVersion) {
			return fmt.Errorf("jekyll version mismatch: found %s, want %s", version, j.requiredVersion)
		}
	}

	return nil
}

// GetVersion returns the Jekyll version
func (j *JekyllBuilder) GetVersion(ctx context.Context) (string, error) {
	output, err := j.runner.Run(ctx, "jekyll", []string{"--version"}, ".", nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// BuildWithConfig builds with custom configuration
func (j *JekyllBuilder) BuildWithConfig(ctx context.Context, config *BuildConfig) error {
	args := []string{"build"}

	// Add destination
	if config.OutputDir != "" {
		args = append(args, "--destination", config.OutputDir)
	}

	// Add base URL
	if config.BaseURL != "" {
		args = append(args, "--baseurl", config.BaseURL)
	}

	// Run build
	_, err := j.runner.Run(ctx, "jekyll", args, config.WorkDir, config.Env)
	if err != nil {
		return fmt.Errorf("jekyll build failed: %w", err)
	}

	return nil
}
