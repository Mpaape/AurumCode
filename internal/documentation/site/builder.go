package site

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// SiteBuilder orchestrates Hugo and Pagefind builds
type SiteBuilder struct {
	hugo     *HugoBuilder
	pagefind *PagefindBuilder
}

// NewSiteBuilder creates a new site builder
func NewSiteBuilder(runner CommandRunner) *SiteBuilder {
	return &SiteBuilder{
		hugo:     NewHugoBuilder(runner),
		pagefind: NewPagefindBuilder(runner),
	}
}

// Build builds the complete site with search
func (s *SiteBuilder) Build(ctx context.Context, config *BuildConfig) (*BuildResult, error) {
	start := time.Now()

	// Step 1: Build Hugo site
	if err := s.hugo.BuildWithConfig(ctx, config); err != nil {
		return &BuildResult{
			Success: false,
			Error:   fmt.Errorf("hugo build failed: %w", err),
		}, err
	}

	// Step 2: Build Pagefind index
	if err := s.pagefind.BuildWithConfig(ctx, config); err != nil {
		return &BuildResult{
			Success: false,
			Error:   fmt.Errorf("pagefind build failed: %w", err),
		}, err
	}

	duration := time.Since(start)

	// Determine output path
	outputPath := config.OutputDir
	if outputPath == "" {
		outputPath = filepath.Join(config.WorkDir, "public")
	}

	return &BuildResult{
		Success:    true,
		OutputPath: outputPath,
		Duration:   duration.Milliseconds(),
	}, nil
}

// Validate validates both Hugo and Pagefind are available
func (s *SiteBuilder) Validate(ctx context.Context) error {
	// Validate Hugo
	if err := s.hugo.Validate(ctx); err != nil {
		return fmt.Errorf("hugo validation failed: %w", err)
	}

	// Validate Pagefind
	if err := s.pagefind.Validate(ctx); err != nil {
		return fmt.Errorf("pagefind validation failed: %w", err)
	}

	return nil
}

// BuildHugoOnly builds only the Hugo site (skip search index)
func (s *SiteBuilder) BuildHugoOnly(ctx context.Context, config *BuildConfig) (*BuildResult, error) {
	start := time.Now()

	if err := s.hugo.BuildWithConfig(ctx, config); err != nil {
		return &BuildResult{
			Success: false,
			Error:   err,
		}, err
	}

	duration := time.Since(start)

	outputPath := config.OutputDir
	if outputPath == "" {
		outputPath = filepath.Join(config.WorkDir, "public")
	}

	return &BuildResult{
		Success:    true,
		OutputPath: outputPath,
		Duration:   duration.Milliseconds(),
	}, nil
}

// BuildSearchOnly builds only the Pagefind search index
func (s *SiteBuilder) BuildSearchOnly(ctx context.Context, config *BuildConfig) (*BuildResult, error) {
	start := time.Now()

	if err := s.pagefind.BuildWithConfig(ctx, config); err != nil {
		return &BuildResult{
			Success: false,
			Error:   err,
		}, err
	}

	duration := time.Since(start)

	return &BuildResult{
		Success:  true,
		Duration: duration.Milliseconds(),
	}, nil
}
