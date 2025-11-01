package changelog

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// Conventional commit format: type(scope): subject
	commitRegex = regexp.MustCompile(`^(?P<type>[a-z]+)(?:\((?P<scope>[^)]+)\))?(?P<breaking>!)?: (?P<subject>.+)$`)

	// Breaking change indicators in footer
	breakingFooterRegex = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:\s*(.+)`)
)

// Parser parses conventional commits from git log output
type Parser struct{}

// NewParser creates a new commit parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseCommit parses a single conventional commit message
func (p *Parser) ParseCommit(hash, author, dateStr, message string) (*Commit, error) {
	// Parse date
	date, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		// Try alternative format
		date, err = time.Parse("2006-01-02 15:04:05 -0700", dateStr)
		if err != nil {
			date = time.Now()
		}
	}

	// Split message into parts
	parts := strings.SplitN(message, "\n\n", 3)
	subject := strings.TrimSpace(parts[0])
	body := ""
	footer := ""

	if len(parts) > 1 {
		body = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		footer = strings.TrimSpace(parts[2])
	}

	// Parse conventional commit format
	matches := commitRegex.FindStringSubmatch(subject)
	if matches == nil {
		// Not a conventional commit, treat as "chore"
		return &Commit{
			Hash:     hash,
			Type:     TypeChore,
			Subject:  subject,
			Body:     body,
			Footer:   footer,
			Breaking: false,
			Date:     date,
			Author:   author,
		}, nil
	}

	// Extract groups
	result := make(map[string]string)
	for i, name := range commitRegex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}

	commitType := CommitType(result["type"])
	scope := result["scope"]
	commitSubject := result["subject"]
	breaking := result["breaking"] == "!"

	// Check for BREAKING CHANGE in footer
	if !breaking && breakingFooterRegex.MatchString(footer) {
		breaking = true
	}

	return &Commit{
		Hash:     hash,
		Type:     commitType,
		Scope:    scope,
		Subject:  commitSubject,
		Body:     body,
		Footer:   footer,
		Breaking: breaking,
		Date:     date,
		Author:   author,
	}, nil
}

// ParseLog parses git log output in format: hash|author|date|message
func (p *Parser) ParseLog(logOutput string) ([]Commit, error) {
	var commits []Commit

	if logOutput == "" {
		return commits, nil
	}

	lines := strings.Split(strings.TrimSpace(logOutput), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Expected format: hash|author|date|message
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		hash := strings.TrimSpace(parts[0])
		author := strings.TrimSpace(parts[1])
		dateStr := strings.TrimSpace(parts[2])
		message := strings.TrimSpace(parts[3])

		commit, err := p.ParseCommit(hash, author, dateStr, message)
		if err != nil {
			// Skip unparseable commits
			continue
		}

		commits = append(commits, *commit)
	}

	return commits, nil
}

// GroupCommits groups commits into releases based on tags
func (p *Parser) GroupCommits(commits []Commit, tags map[string]time.Time) *Changelog {
	changelog := &Changelog{
		Releases: []Release{},
	}

	// If no tags, all commits are unreleased
	if len(tags) == 0 {
		changelog.Unreleased = commits
		return changelog
	}

	// Sort tags by date (newest first)
	sortedTags := p.sortTagsByDate(tags)

	// Group commits by release
	currentRelease := &Release{}
	currentTagIdx := 0

	for _, commit := range commits {
		// Check if this commit belongs to current tag
		if currentTagIdx < len(sortedTags) {
			tagName := sortedTags[currentTagIdx]
			tagDate := tags[tagName]

			// If commit is after tag date, it's unreleased
			if commit.Date.After(tagDate) {
				changelog.Unreleased = append(changelog.Unreleased, commit)
				continue
			}

			// Start new release if needed
			if currentRelease.Tag == "" {
				currentRelease = &Release{
					Version: extractVersion(tagName),
					Tag:     tagName,
					Date:    tagDate,
					Commits: []Commit{},
				}
			}

			currentRelease.Commits = append(currentRelease.Commits, commit)

			// Check if we need to finalize this release
			if commit.Date.Before(tagDate) && len(currentRelease.Commits) > 0 {
				changelog.Releases = append(changelog.Releases, *currentRelease)
				currentTagIdx++
				currentRelease = &Release{}
			}
		} else {
			// All remaining commits are in the last release
			if currentRelease.Tag != "" {
				currentRelease.Commits = append(currentRelease.Commits, commit)
			}
		}
	}

	// Add final release if it has commits
	if currentRelease.Tag != "" && len(currentRelease.Commits) > 0 {
		changelog.Releases = append(changelog.Releases, *currentRelease)
	}

	return changelog
}

// sortTagsByDate sorts tags by date (newest first)
func (p *Parser) sortTagsByDate(tags map[string]time.Time) []string {
	type tagDate struct {
		tag  string
		date time.Time
	}

	var sorted []tagDate
	for tag, date := range tags {
		sorted = append(sorted, tagDate{tag, date})
	}

	// Sort by date descending
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].date.Before(sorted[j].date) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	result := make([]string, len(sorted))
	for i, td := range sorted {
		result[i] = td.tag
	}
	return result
}

// extractVersion extracts version from tag (e.g., "v1.2.3" -> "1.2.3")
func extractVersion(tag string) string {
	version := strings.TrimPrefix(tag, "v")
	return version
}

// FormatLogCommand returns the git log format string for parsing
func FormatLogCommand() string {
	return "--pretty=format:%H|%an|%aI|%s%n%n%b"
}

// ValidateCommit checks if a commit message follows conventional commits
func ValidateCommit(message string) error {
	lines := strings.Split(message, "\n")
	if len(lines) == 0 {
		return fmt.Errorf("empty commit message")
	}

	subject := lines[0]
	matches := commitRegex.FindStringSubmatch(subject)
	if matches == nil {
		return fmt.Errorf("does not follow conventional commit format: type(scope): subject")
	}

	return nil
}
