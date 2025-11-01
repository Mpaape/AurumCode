package readme

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Section represents a named section in README
type Section struct {
	Name    string
	Content string
}

// UpdateResult contains the result of an update operation
type UpdateResult struct {
	Updated  bool
	Changes  []string
	FilePath string
}

// Updater updates README sections between markers
type Updater struct {
	dryRun bool
}

// NewUpdater creates a new README updater
func NewUpdater() *Updater {
	return &Updater{
		dryRun: false,
	}
}

// WithDryRun enables dry-run mode (no file writes)
func (u *Updater) WithDryRun(enabled bool) *Updater {
	u.dryRun = enabled
	return u
}

// Update updates README sections with given content
func (u *Updater) Update(filePath string, sections []Section) (*UpdateResult, error) {
	// Read existing file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	originalContent := string(content)
	updatedContent := originalContent
	changes := []string{}

	// Update each section
	for _, section := range sections {
		newContent, changed, err := u.updateSection(updatedContent, section)
		if err != nil {
			return nil, fmt.Errorf("update section %s: %w", section.Name, err)
		}

		if changed {
			changes = append(changes, fmt.Sprintf("Updated section: %s", section.Name))
			updatedContent = newContent
		}
	}

	// Check if anything changed
	if updatedContent == originalContent {
		return &UpdateResult{
			Updated:  false,
			Changes:  changes,
			FilePath: filePath,
		}, nil
	}

	// Write file if not in dry-run mode
	if !u.dryRun {
		if err := os.WriteFile(filePath, []byte(updatedContent), 0644); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}
	}

	return &UpdateResult{
		Updated:  true,
		Changes:  changes,
		FilePath: filePath,
	}, nil
}

// updateSection updates a single section between markers
func (u *Updater) updateSection(content string, section Section) (string, bool, error) {
	// Build marker patterns
	startMarker := fmt.Sprintf("<!-- aurum:start:%s -->", section.Name)
	endMarker := fmt.Sprintf("<!-- aurum:end:%s -->", section.Name)

	// Also support generic markers if no name
	if section.Name == "" || section.Name == "default" {
		startMarker = "<!-- aurum:start -->"
		endMarker = "<!-- aurum:end -->"
	}

	// Check if markers exist
	if !strings.Contains(content, startMarker) {
		return content, false, fmt.Errorf("start marker not found: %s", startMarker)
	}

	if !strings.Contains(content, endMarker) {
		return content, false, fmt.Errorf("end marker not found: %s", endMarker)
	}

	// Build regex pattern to match section
	// Escape special characters in markers for regex
	escapedStart := regexp.QuoteMeta(startMarker)
	escapedEnd := regexp.QuoteMeta(endMarker)

	pattern := fmt.Sprintf(`(%s)[\s\S]*?(%s)`, escapedStart, escapedEnd)
	re := regexp.MustCompile(pattern)

	// Extract current content
	matches := re.FindStringSubmatch(content)
	if len(matches) < 3 {
		return content, false, fmt.Errorf("could not extract section content")
	}

	currentContent := strings.TrimSpace(matches[0])
	currentContent = strings.TrimPrefix(currentContent, startMarker)
	currentContent = strings.TrimSuffix(currentContent, endMarker)
	currentContent = strings.TrimSpace(currentContent)

	// Prepare new content
	newSectionContent := strings.TrimSpace(section.Content)

	// Check if content actually changed
	if currentContent == newSectionContent {
		return content, false, nil
	}

	// Build replacement
	replacement := fmt.Sprintf("%s\n\n%s\n\n%s", startMarker, newSectionContent, endMarker)

	// Replace content
	updated := re.ReplaceAllString(content, replacement)

	return updated, true, nil
}

// GetSection extracts a section's current content
func (u *Updater) GetSection(filePath string, sectionName string) (string, error) {
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Build markers
	startMarker := fmt.Sprintf("<!-- aurum:start:%s -->", sectionName)
	endMarker := fmt.Sprintf("<!-- aurum:end:%s -->", sectionName)

	if sectionName == "" || sectionName == "default" {
		startMarker = "<!-- aurum:start -->"
		endMarker = "<!-- aurum:end -->"
	}

	// Check markers exist
	text := string(content)
	if !strings.Contains(text, startMarker) || !strings.Contains(text, endMarker) {
		return "", fmt.Errorf("markers not found for section: %s", sectionName)
	}

	// Extract content
	escapedStart := regexp.QuoteMeta(startMarker)
	escapedEnd := regexp.QuoteMeta(endMarker)
	pattern := fmt.Sprintf(`%s([\s\S]*?)%s`, escapedStart, escapedEnd)
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract section content")
	}

	return strings.TrimSpace(matches[1]), nil
}

// HasSection checks if a section exists
func (u *Updater) HasSection(filePath string, sectionName string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("read file: %w", err)
	}

	startMarker := fmt.Sprintf("<!-- aurum:start:%s -->", sectionName)
	if sectionName == "" || sectionName == "default" {
		startMarker = "<!-- aurum:start -->"
	}

	return strings.Contains(string(content), startMarker), nil
}

// Diff returns a diff between current and new content
func (u *Updater) Diff(filePath string, sections []Section) (string, error) {
	var diff strings.Builder

	for _, section := range sections {
		current, err := u.GetSection(filePath, section.Name)
		if err != nil {
			diff.WriteString(fmt.Sprintf("Section %s: markers not found\n", section.Name))
			continue
		}

		new := strings.TrimSpace(section.Content)
		if current != new {
			diff.WriteString(fmt.Sprintf("Section: %s\n", section.Name))
			diff.WriteString("--- Current\n")
			diff.WriteString("+++ New\n")
			diff.WriteString(fmt.Sprintf("- %s\n", current))
			diff.WriteString(fmt.Sprintf("+ %s\n\n", new))
		}
	}

	return diff.String(), nil
}

// InsertSection inserts new markers and content if they don't exist
func (u *Updater) InsertSection(filePath string, section Section, afterLine string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	text := string(content)

	// Check if section already exists
	startMarker := fmt.Sprintf("<!-- aurum:start:%s -->", section.Name)
	if section.Name == "" || section.Name == "default" {
		startMarker = "<!-- aurum:start -->"
	}

	if strings.Contains(text, startMarker) {
		return fmt.Errorf("section already exists: %s", section.Name)
	}

	// Build section content
	endMarker := fmt.Sprintf("<!-- aurum:end:%s -->", section.Name)
	if section.Name == "" || section.Name == "default" {
		endMarker = "<!-- aurum:end -->"
	}

	newSection := fmt.Sprintf("\n\n%s\n\n%s\n\n%s\n", startMarker, section.Content, endMarker)

	// Find insertion point
	var updatedContent string
	if afterLine != "" {
		// Insert after specific line
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if strings.Contains(line, afterLine) {
				// Insert after this line
				before := strings.Join(lines[:i+1], "\n")
				after := strings.Join(lines[i+1:], "\n")
				updatedContent = before + newSection + after
				break
			}
		}
		if updatedContent == "" {
			return fmt.Errorf("insertion point not found: %s", afterLine)
		}
	} else {
		// Append to end
		updatedContent = text + newSection
	}

	// Write file if not in dry-run mode
	if !u.dryRun {
		if err := os.WriteFile(filePath, []byte(updatedContent), 0644); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
	}

	return nil
}
