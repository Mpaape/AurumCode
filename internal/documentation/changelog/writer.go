package changelog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Writer writes changelog to Markdown format
type Writer struct {
	includeAuthors bool
	includeHashes  bool
}

// NewWriter creates a new changelog writer
func NewWriter() *Writer {
	return &Writer{
		includeAuthors: false,
		includeHashes:  true,
	}
}

// WithAuthors enables author names in changelog
func (w *Writer) WithAuthors(enabled bool) *Writer {
	w.includeAuthors = enabled
	return w
}

// WithHashes enables commit hashes in changelog
func (w *Writer) WithHashes(enabled bool) *Writer {
	w.includeHashes = enabled
	return w
}

// Write writes changelog to a file
func (w *Writer) Write(changelog *Changelog, outputPath string) error {
	content := w.Generate(changelog)

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

// Generate generates changelog markdown content
func (w *Writer) Generate(changelog *Changelog) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# Changelog\n\n")
	sb.WriteString("All notable changes to this project will be documented in this file.\n\n")
	sb.WriteString("The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),\n")
	sb.WriteString("and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).\n\n")

	// Unreleased section
	if len(changelog.Unreleased) > 0 {
		sb.WriteString("## [Unreleased]\n\n")
		w.writeCommitGroup(changelog.Unreleased, &sb)
		sb.WriteString("\n")
	}

	// Releases
	for _, release := range changelog.Releases {
		// Release header
		version := release.Version
		if version == "" {
			version = release.Tag
		}

		dateStr := release.Date.Format("2006-01-02")
		sb.WriteString(fmt.Sprintf("## [%s] - %s\n\n", version, dateStr))

		// Write commits
		w.writeCommitGroup(release.Commits, &sb)
		sb.WriteString("\n")
	}

	return sb.String()
}

// writeCommitGroup writes a group of commits organized by type
func (w *Writer) writeCommitGroup(commits []Commit, sb *strings.Builder) {
	grouped := &GroupedCommits{}
	for _, commit := range commits {
		grouped.Add(commit)
	}

	// Breaking Changes (highest priority)
	if len(grouped.Breaking) > 0 {
		sb.WriteString("### ⚠ BREAKING CHANGES\n\n")
		for _, commit := range grouped.Breaking {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Features
	if len(grouped.Features) > 0 {
		sb.WriteString("### Features\n\n")
		for _, commit := range grouped.Features {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Bug Fixes
	if len(grouped.Fixes) > 0 {
		sb.WriteString("### Bug Fixes\n\n")
		for _, commit := range grouped.Fixes {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Performance Improvements
	if len(grouped.Perf) > 0 {
		sb.WriteString("### Performance Improvements\n\n")
		for _, commit := range grouped.Perf {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Refactoring
	if len(grouped.Refactor) > 0 {
		sb.WriteString("### Code Refactoring\n\n")
		for _, commit := range grouped.Refactor {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Documentation
	if len(grouped.Docs) > 0 {
		sb.WriteString("### Documentation\n\n")
		for _, commit := range grouped.Docs {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Tests
	if len(grouped.Test) > 0 {
		sb.WriteString("### Tests\n\n")
		for _, commit := range grouped.Test {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Build System
	if len(grouped.Build) > 0 {
		sb.WriteString("### Build System\n\n")
		for _, commit := range grouped.Build {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// CI/CD
	if len(grouped.CI) > 0 {
		sb.WriteString("### Continuous Integration\n\n")
		for _, commit := range grouped.CI {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Chores (usually omitted from changelog, but included for completeness)
	if len(grouped.Chore) > 0 {
		sb.WriteString("### Chores\n\n")
		for _, commit := range grouped.Chore {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Reverts
	if len(grouped.Revert) > 0 {
		sb.WriteString("### Reverts\n\n")
		for _, commit := range grouped.Revert {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}

	// Other
	if len(grouped.Other) > 0 {
		sb.WriteString("### Other Changes\n\n")
		for _, commit := range grouped.Other {
			w.writeCommitLine(commit, sb)
		}
		sb.WriteString("\n")
	}
}

// writeCommitLine writes a single commit as a bullet point
func (w *Writer) writeCommitLine(commit Commit, sb *strings.Builder) {
	// Build commit line
	line := "* "

	// Add scope if present
	if commit.Scope != "" {
		line += fmt.Sprintf("**%s:** ", commit.Scope)
	}

	// Add subject
	line += commit.Subject

	// Add hash if enabled
	if w.includeHashes && commit.Hash != "" {
		shortHash := commit.Hash
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}
		line += fmt.Sprintf(" ([%s])", shortHash)
	}

	// Add author if enabled
	if w.includeAuthors && commit.Author != "" {
		line += fmt.Sprintf(" - %s", commit.Author)
	}

	sb.WriteString(line)
	sb.WriteString("\n")

	// Add breaking change details if present
	if commit.Breaking && commit.Footer != "" {
		// Extract breaking change description from footer
		if strings.Contains(commit.Footer, "BREAKING") {
			lines := strings.Split(commit.Footer, "\n")
			for _, l := range lines {
				if strings.HasPrefix(l, "BREAKING") {
					desc := strings.TrimPrefix(l, "BREAKING CHANGE:")
					desc = strings.TrimPrefix(desc, "BREAKING-CHANGE:")
					desc = strings.TrimSpace(desc)
					if desc != "" {
						sb.WriteString(fmt.Sprintf("  * %s\n", desc))
					}
				}
			}
		}
	}
}

// UpdateChangelog updates an existing changelog file or creates a new one
func (w *Writer) UpdateChangelog(repoPath string, commits []Commit, tags map[string]time.Time) error {
	parser := NewParser()
	changelog := parser.GroupCommits(commits, tags)

	outputPath := filepath.Join(repoPath, "docs", "CHANGELOG.md")
	return w.Write(changelog, outputPath)
}
