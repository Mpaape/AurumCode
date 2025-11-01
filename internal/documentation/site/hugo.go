package site

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// HugoBuilder builds sites using Hugo
type HugoBuilder struct {
	runner         CommandRunner
	requiredVersion string
}

// NewHugoBuilder creates a new Hugo builder
func NewHugoBuilder(runner CommandRunner) *HugoBuilder {
	return &HugoBuilder{
		runner:         runner,
		requiredVersion: "0.134.2",
	}
}

// WithRequiredVersion sets the required Hugo version
func (h *HugoBuilder) WithRequiredVersion(version string) *HugoBuilder {
	h.requiredVersion = version
	return h
}

// Build builds the Hugo site
func (h *HugoBuilder) Build(ctx context.Context, workdir string) error {
	// Validate first
	if err := h.Validate(ctx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Build command arguments
	args := []string{"--minify"}

	// Run Hugo build
	output, err := h.runner.Run(ctx, "hugo", args, workdir, nil)
	if err != nil {
		return fmt.Errorf("hugo build failed: %w", err)
	}

	// Check for success indicators in output
	if !strings.Contains(output, "Total") && !strings.Contains(output, "Built") {
		return fmt.Errorf("unexpected build output: %s", output)
	}

	return nil
}

// Validate checks if Hugo is available
func (h *HugoBuilder) Validate(ctx context.Context) error {
	version, err := h.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("hugo not found: %w", err)
	}

	// Check version if required version is set
	if h.requiredVersion != "" {
		// Extract version number from output like "hugo v0.134.2+extended"
		if !strings.Contains(version, h.requiredVersion) {
			return fmt.Errorf("hugo version mismatch: found %s, want %s", version, h.requiredVersion)
		}
	}

	return nil
}

// GetVersion returns the Hugo version
func (h *HugoBuilder) GetVersion(ctx context.Context) (string, error) {
	output, err := h.runner.Run(ctx, "hugo", []string{"version"}, ".", nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// BuildWithConfig builds with custom configuration
func (h *HugoBuilder) BuildWithConfig(ctx context.Context, config *BuildConfig) error {
	args := []string{}

	// Add destination
	if config.OutputDir != "" {
		args = append(args, "--destination", config.OutputDir)
	}

	// Add base URL
	if config.BaseURL != "" {
		args = append(args, "--baseURL", config.BaseURL)
	}

	// Add minify
	if config.Minify {
		args = append(args, "--minify")
	}

	// Run build
	_, err := h.runner.Run(ctx, "hugo", args, config.WorkDir, config.Env)
	if err != nil {
		return fmt.Errorf("hugo build failed: %w", err)
	}

	return nil
}
