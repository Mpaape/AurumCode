package site

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// SiteBuilder orchestrates Jekyll and Pagefind builds
type SiteBuilder struct {
	jekyll   *JekyllBuilder
	pagefind *PagefindBuilder
}

// NewSiteBuilder creates a new site builder
func NewSiteBuilder(runner CommandRunner) *SiteBuilder {
	return &SiteBuilder{
		jekyll:   NewJekyllBuilder(runner),
		pagefind: NewPagefindBuilder(runner),
	}
}

// Build builds the complete site with search
func (s *SiteBuilder) Build(ctx context.Context, config *BuildConfig) (*BuildResult, error) {
	start := time.Now()

	// Step 1: Build Jekyll site
	if err := s.jekyll.BuildWithConfig(ctx, config); err != nil {
		return &BuildResult{
			Success: false,
			Error:   fmt.Errorf("jekyll build failed: %w", err),
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

	// Determine output path (Jekyll uses _site by default)
	outputPath := config.OutputDir
	if outputPath == "" {
		outputPath = filepath.Join(config.WorkDir, "_site")
	}

	return &BuildResult{
		Success:    true,
		OutputPath: outputPath,
		Duration:   duration.Milliseconds(),
	}, nil
}

// Validate validates both Jekyll and Pagefind are available
func (s *SiteBuilder) Validate(ctx context.Context) error {
	// Validate Jekyll
	if err := s.jekyll.Validate(ctx); err != nil {
		return fmt.Errorf("jekyll validation failed: %w", err)
	}

	// Validate Pagefind
	if err := s.pagefind.Validate(ctx); err != nil {
		return fmt.Errorf("pagefind validation failed: %w", err)
	}

	return nil
}

// BuildJekyllOnly builds only the Jekyll site (skip search index)
func (s *SiteBuilder) BuildJekyllOnly(ctx context.Context, config *BuildConfig) (*BuildResult, error) {
	start := time.Now()

	if err := s.jekyll.BuildWithConfig(ctx, config); err != nil {
		return &BuildResult{
			Success: false,
			Error:   err,
		}, err
	}

	duration := time.Since(start)

	outputPath := config.OutputDir
	if outputPath == "" {
		outputPath = filepath.Join(config.WorkDir, "_site")
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
