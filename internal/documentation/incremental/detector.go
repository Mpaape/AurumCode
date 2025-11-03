package incremental

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// ChangeDetector detects source file changes using git
type ChangeDetector struct {
	runner  site.CommandRunner
	repoDir string
}

// NewChangeDetector creates a new change detector
func NewChangeDetector(runner site.CommandRunner, repoDir string) *ChangeDetector {
	return &ChangeDetector{
		runner:  runner,
		repoDir: repoDir,
	}
}

// DetectChanges detects changed files between two commits
func (d *ChangeDetector) DetectChanges(ctx context.Context, fromCommit, toCommit string) ([]string, error) {
	if fromCommit == "" || toCommit == "" {
		return nil, fmt.Errorf("both fromCommit and toCommit are required")
	}

	// Use git diff to find changed files
	args := []string{"diff", "--name-only", fmt.Sprintf("%s...%s", fromCommit, toCommit)}
	output, err := d.runner.Run(ctx, "git", args, d.repoDir, nil)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	return d.parseGitOutput(output), nil
}

// DetectChangesSinceCommit detects changes since a specific commit
func (d *ChangeDetector) DetectChangesSinceCommit(ctx context.Context, since string) ([]string, error) {
	if since == "" {
		return nil, fmt.Errorf("since commit is required")
	}

	// Use git diff to find changed files since commit
	args := []string{"diff", "--name-only", since, "HEAD"}
	output, err := d.runner.Run(ctx, "git", args, d.repoDir, nil)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	return d.parseGitOutput(output), nil
}

// DetectUnstagedChanges detects changes in working directory
func (d *ChangeDetector) DetectUnstagedChanges(ctx context.Context) ([]string, error) {
	// Get unstaged changes
	args := []string{"diff", "--name-only"}
	output, err := d.runner.Run(ctx, "git", args, d.repoDir, nil)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	files := d.parseGitOutput(output)

	// Also get untracked files
	untracked, err := d.getUntrackedFiles(ctx)
	if err != nil {
		return files, nil // Return what we have
	}

	files = append(files, untracked...)
	return files, nil
}

// GetCurrentCommit returns the current commit hash
func (d *ChangeDetector) GetCurrentCommit(ctx context.Context) (string, error) {
	args := []string{"rev-parse", "HEAD"}
	output, err := d.runner.Run(ctx, "git", args, d.repoDir, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get current commit: %w", err)
	}

	return strings.TrimSpace(output), nil
}

// FilterByLanguage filters files by programming language extensions
func (d *ChangeDetector) FilterByLanguage(files []string, extensions []string) []string {
	if len(extensions) == 0 {
		return files
	}

	var filtered []string
	for _, file := range files {
		ext := filepath.Ext(file)
		for _, allowed := range extensions {
			if ext == allowed || ext == "."+allowed {
				filtered = append(filtered, file)
				break
			}
		}
	}

	return filtered
}

// IsGitRepository checks if the directory is a git repository
func (d *ChangeDetector) IsGitRepository(ctx context.Context) bool {
	args := []string{"rev-parse", "--git-dir"}
	_, err := d.runner.Run(ctx, "git", args, d.repoDir, nil)
	return err == nil
}

// getUntrackedFiles gets list of untracked files
func (d *ChangeDetector) getUntrackedFiles(ctx context.Context) ([]string, error) {
	args := []string{"ls-files", "--others", "--exclude-standard"}
	output, err := d.runner.Run(ctx, "git", args, d.repoDir, nil)
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}

	return d.parseGitOutput(output), nil
}

// parseGitOutput parses git command output into file list
func (d *ChangeDetector) parseGitOutput(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var files []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}

	return files
}
