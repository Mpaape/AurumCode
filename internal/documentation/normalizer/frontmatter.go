package normalizer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// FrontMatter represents Jekyll/Hugo YAML front matter
type FrontMatter struct {
	Title       string `yaml:"title,omitempty"`
	Layout      string `yaml:"layout,omitempty"`
	Parent      string `yaml:"parent,omitempty"`
	GrandParent string `yaml:"grand_parent,omitempty"`
	NavOrder    int    `yaml:"nav_order,omitempty"`
	HasChildren bool   `yaml:"has_children,omitempty"`
	Permalink   string `yaml:"permalink,omitempty"`
}

// FrontMatterOptions provides context for generating front matter
type FrontMatterOptions struct {
	FilePath    string // Relative file path from docs root
	Language    string // Programming language (go, python, etc.)
	Section     string // Documentation section (_api, _stack, etc.)
	IsIndex     bool   // Whether this is an index.md file
	CustomTitle string // Custom title override
}

// GenerateFrontMatter creates appropriate front matter based on context
func GenerateFrontMatter(opts FrontMatterOptions) *FrontMatter {
	fm := &FrontMatter{
		Layout: "default",
	}

	// Generate title from file path if not provided
	if opts.CustomTitle != "" {
		fm.Title = opts.CustomTitle
	} else {
		fm.Title = generateTitle(opts.FilePath, opts.Language)
	}

	// Handle index files
	if opts.IsIndex {
		fm.HasChildren = true
	}

	// Set parent based on section
	if opts.Section != "" {
		fm.Parent = sectionToParent(opts.Section)
	}

	// Generate permalink
	if opts.FilePath != "" {
		fm.Permalink = generatePermalink(opts.FilePath, opts.Section)
	}

	return fm
}

// MergeFrontMatter merges new front matter with existing, preferring existing values
func MergeFrontMatter(existing, new *FrontMatter) *FrontMatter {
	merged := &FrontMatter{}

	// Prefer existing values over new
	merged.Title = preferExisting(existing.Title, new.Title)
	merged.Layout = preferExisting(existing.Layout, new.Layout)
	merged.Parent = preferExisting(existing.Parent, new.Parent)
	merged.GrandParent = preferExisting(existing.GrandParent, new.GrandParent)
	merged.Permalink = preferExisting(existing.Permalink, new.Permalink)

	// For integers and bools, prefer existing if set
	if existing.NavOrder != 0 {
		merged.NavOrder = existing.NavOrder
	} else {
		merged.NavOrder = new.NavOrder
	}

	merged.HasChildren = existing.HasChildren || new.HasChildren

	return merged
}

// ToYAML converts front matter to YAML string with delimiters
func (fm *FrontMatter) ToYAML() (string, error) {
	data, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal front matter: %w", err)
	}

	// Add YAML delimiters
	return fmt.Sprintf("---\n%s---\n\n", string(data)), nil
}

// ParseFrontMatter extracts front matter from markdown content
func ParseFrontMatter(content string) (*FrontMatter, string, error) {
	// Match front matter: ---\n...\n---
	re := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n\n?(.*)$`)
	matches := re.FindStringSubmatch(content)

	if len(matches) < 3 {
		// No front matter found
		return nil, content, nil
	}

	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(matches[1]), &fm); err != nil {
		return nil, "", fmt.Errorf("failed to parse front matter: %w", err)
	}

	// Return front matter and remaining content
	return &fm, matches[2], nil
}

// generateTitle creates a human-readable title from file path
func generateTitle(filePath, language string) string {
	// Remove extension
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	// Handle index files
	if base == "index" {
		dir := filepath.Dir(filePath)
		if dir != "." && dir != "/" {
			base = filepath.Base(dir)
		}
	}

	// Convert underscores and hyphens to spaces
	title := strings.ReplaceAll(base, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")

	// Capitalize first letter of each word
	words := strings.Fields(title)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}

	title = strings.Join(words, " ")

	// Add language context if available
	if language != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(language)) {
		title = fmt.Sprintf("%s - %s", title, strings.Title(language))
	}

	return title
}

// sectionToParent converts section name to parent navigation name
func sectionToParent(section string) string {
	// Remove leading underscore from Jekyll collection names
	section = strings.TrimPrefix(section, "_")

	switch section {
	case "api":
		return "API Reference"
	case "stack":
		return "Technology Stack"
	case "architecture":
		return "Architecture"
	case "tutorials":
		return "Tutorials"
	case "roadmap":
		return "Roadmap"
	case "custom":
		return "Custom Documentation"
	default:
		// Capitalize and convert underscores
		return strings.Title(strings.ReplaceAll(section, "_", " "))
	}
}

// generatePermalink creates a permalink from file path
func generatePermalink(filePath, section string) string {
	// Remove extension and normalize
	path := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	path = filepath.ToSlash(path) // Ensure forward slashes

	// For index files, use directory path
	if filepath.Base(path) == "index" {
		path = filepath.Dir(path)
	}

	// Add section prefix if provided
	if section != "" {
		section = strings.TrimPrefix(section, "_")
		if !strings.HasPrefix(path, section) {
			path = filepath.Join(section, path)
		}
	}

	// Ensure leading slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Ensure trailing slash for index-like paths
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	return path
}

// preferExisting returns existing value if non-empty, otherwise returns new value
func preferExisting(existing, new string) string {
	if existing != "" {
		return existing
	}
	return new
}
