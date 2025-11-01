package linkcheck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Checker orchestrates link checking
type Checker struct {
	scanner   *Scanner
	validator *Validator
}

// NewChecker creates a new link checker
func NewChecker(baseDir string) *Checker {
	return &Checker{
		scanner:   NewScanner(),
		validator: NewValidator(baseDir),
	}
}

// WithIgnorePatterns sets URL patterns to ignore
func (c *Checker) WithIgnorePatterns(patterns []string) *Checker {
	c.scanner = c.scanner.WithIgnorePatterns(patterns)
	return c
}

// WithExternalCheck enables external link checking
func (c *Checker) WithExternalCheck(enabled bool) *Checker {
	c.validator = c.validator.WithExternalCheck(enabled)
	return c
}

// CheckDirectory checks all links in a directory
func (c *Checker) CheckDirectory(ctx context.Context, dir string) (*Report, error) {
	// Scan for links
	links, err := c.scanner.ScanDirectory(dir)
	if err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}

	// Validate links
	report := &Report{
		TotalLinks: len(links),
		Results:    make([]LinkResult, 0, len(links)),
	}

	for _, link := range links {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
		}

		result := c.validator.Validate(ctx, link)
		report.Results = append(report.Results, result)

		// Update counts
		switch result.Status {
		case LinkStatusOK:
			report.OKLinks++
		case LinkStatusBroken:
			report.BrokenLinks++
		case LinkStatusSkipped:
			report.SkippedLinks++
		}
	}

	return report, nil
}

// CheckFile checks links in a single file
func (c *Checker) CheckFile(ctx context.Context, filePath string) (*Report, error) {
	// Scan file
	links, err := c.scanner.ScanFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	// Validate links
	report := &Report{
		TotalLinks: len(links),
		Results:    make([]LinkResult, 0, len(links)),
	}

	for _, link := range links {
		result := c.validator.Validate(ctx, link)
		report.Results = append(report.Results, result)

		switch result.Status {
		case LinkStatusOK:
			report.OKLinks++
		case LinkStatusBroken:
			report.BrokenLinks++
		case LinkStatusSkipped:
			report.SkippedLinks++
		}
	}

	return report, nil
}

// GenerateReport generates a human-readable report
func (c *Checker) GenerateReport(report *Report) string {
	var sb strings.Builder

	sb.WriteString("# Link Validation Report\n\n")
	sb.WriteString(fmt.Sprintf("**Total Links:** %d\n", report.TotalLinks))
	sb.WriteString(fmt.Sprintf("**OK:** %d\n", report.OKLinks))
	sb.WriteString(fmt.Sprintf("**Broken:** %d\n", report.BrokenLinks))
	sb.WriteString(fmt.Sprintf("**Skipped:** %d\n\n", report.SkippedLinks))

	// Show broken links
	if report.BrokenLinks > 0 {
		sb.WriteString("## Broken Links\n\n")
		for _, result := range report.Results {
			if result.Status == LinkStatusBroken {
				sb.WriteString(fmt.Sprintf("- **%s** in `%s:%d`\n",
					result.Link.URL,
					result.Link.SourceFile,
					result.Link.LineNumber))
				if result.Message != "" {
					sb.WriteString(fmt.Sprintf("  - %s\n", result.Message))
				}
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// WriteReport writes the report to a file
func (c *Checker) WriteReport(report *Report, outputPath string) error {
	content := c.GenerateReport(report)

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// HasErrors returns true if there are broken links
func (c *Checker) HasErrors(report *Report) bool {
	return report.BrokenLinks > 0
}
