package normalizer

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// FrontMatter represents the YAML front matter of a documentation page.
type FrontMatter struct {
	Title       string `yaml:"title,omitempty"`
	Layout      string `yaml:"layout,omitempty"`
	Parent      string `yaml:"parent,omitempty"`
	GrandParent string `yaml:"grand_parent,omitempty"`
	NavOrder    int    `yaml:"nav_order,omitempty"`
	HasChildren bool   `yaml:"has_children,omitempty"`
	Permalink   string `yaml:"permalink,omitempty"`

	// Extra holds entries this package does not model. They are carried
	// through unchanged: the site generator and its theme read keys beyond
	// the ones above, so dropping them would silently change the site.
	Extra map[string]interface{} `yaml:"-"`
}

// knownFrontMatterKeys is derived from the struct tags so that adding a field
// to FrontMatter cannot leave a stale duplicate of its key behind.
var knownFrontMatterKeys = func() map[string]bool {
	keys := make(map[string]bool)
	structType := reflect.TypeOf(FrontMatter{})
	for i := 0; i < structType.NumField(); i++ {
		name := strings.Split(structType.Field(i).Tag.Get("yaml"), ",")[0]
		if name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}()

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

	if opts.CustomTitle != "" {
		fm.Title = opts.CustomTitle
	} else {
		fm.Title = generateTitle(opts.FilePath, opts.Language)
	}

	if opts.IsIndex {
		fm.HasChildren = true
	}

	if opts.Section != "" {
		fm.Parent = sectionToParent(opts.Section)
	}

	if opts.FilePath != "" {
		fm.Permalink = generatePermalink(opts.FilePath, opts.Section)
	}

	return fm
}

// MergeFrontMatter merges new front matter with existing, preferring existing values
func MergeFrontMatter(existing, new *FrontMatter) *FrontMatter {
	merged := &FrontMatter{}

	merged.Title = preferExisting(existing.Title, new.Title)
	merged.Layout = preferExisting(existing.Layout, new.Layout)
	merged.Parent = preferExisting(existing.Parent, new.Parent)
	merged.GrandParent = preferExisting(existing.GrandParent, new.GrandParent)
	merged.Permalink = preferExisting(existing.Permalink, new.Permalink)

	if existing.NavOrder != 0 {
		merged.NavOrder = existing.NavOrder
	} else {
		merged.NavOrder = new.NavOrder
	}

	merged.HasChildren = existing.HasChildren || new.HasChildren

	merged.Extra = mergeExtra(existing.Extra, new.Extra)

	return merged
}

func mergeExtra(existing, new map[string]interface{}) map[string]interface{} {
	if len(existing) == 0 && len(new) == 0 {
		return nil
	}
	merged := make(map[string]interface{}, len(existing)+len(new))
	for key, value := range new {
		merged[key] = value
	}
	for key, value := range existing {
		merged[key] = value
	}
	return merged
}

// ToYAML converts front matter to YAML string with delimiters.
func (fm *FrontMatter) ToYAML() (string, error) {
	known, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal front matter: %w", err)
	}

	body := string(known)
	if len(fm.Extra) > 0 {
		body, err = appendExtra(known, fm.Extra)
		if err != nil {
			return "", err
		}
	}

	return "---\n" + body + "---\n\n", nil
}

// appendExtra re-emits the marshalled known keys with the unmodelled keys
// appended in sorted order, so that repeated normalization is byte stable.
func appendExtra(known []byte, extra map[string]interface{}) (string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(known, &document); err != nil {
		return "", fmt.Errorf("failed to marshal front matter: %w", err)
	}

	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if len(document.Content) > 0 {
		mapping = document.Content[0]
	}
	mapping.Style = 0

	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		value, err := yaml.Marshal(extra[name])
		if err != nil {
			return "", fmt.Errorf("failed to marshal front matter entry %q: %w", name, err)
		}
		var valueDocument yaml.Node
		if err := yaml.Unmarshal(value, &valueDocument); err != nil {
			return "", fmt.Errorf("failed to marshal front matter entry %q: %w", name, err)
		}
		if len(valueDocument.Content) == 0 {
			continue
		}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			valueDocument.Content[0],
		)
	}

	out, err := yaml.Marshal(mapping)
	if err != nil {
		return "", fmt.Errorf("failed to marshal front matter: %w", err)
	}
	return string(out), nil
}

// ParseFrontMatter extracts front matter from markdown content. Content
// without a recognizable block is returned unchanged with a nil front matter.
func ParseFrontMatter(content string) (*FrontMatter, string, error) {
	// A UTF-8 BOM ahead of the delimiter hides the block from every parser,
	// this one included, so strip it rather than carry it into the output.
	content = strings.TrimPrefix(content, "\ufeff")

	raw, body, ok := splitFrontMatter(content)
	if !ok {
		return nil, content, nil
	}

	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		return nil, "", fmt.Errorf("failed to parse front matter: %w", err)
	}

	entries := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, "", fmt.Errorf("failed to parse front matter: %w", err)
	}
	for key := range entries {
		if knownFrontMatterKeys[key] {
			delete(entries, key)
		}
	}
	if len(entries) > 0 {
		fm.Extra = entries
	}

	return &fm, body, nil
}

// splitFrontMatter separates a leading delimited block from the body. It
// accepts the line endings and delimiter padding real editors produce; a
// block that is not recognized here gets a second one prepended by the
// normalizer, which the site then renders as page text.
func splitFrontMatter(content string) (raw, body string, ok bool) {
	line, rest, hasNewline := cutLine(content)
	if !hasNewline || !isDelimiter(line) {
		return "", "", false
	}

	var lines []string
	for {
		line, next, hasNewline := cutLine(rest)
		if isDelimiter(line) {
			return strings.Join(lines, "\n"), trimLeadingBlankLine(next), true
		}
		if !hasNewline {
			return "", "", false
		}
		lines = append(lines, strings.TrimSuffix(line, "\r"))
		rest = next
	}
}

func cutLine(content string) (line, rest string, hasNewline bool) {
	if i := strings.IndexByte(content, '\n'); i >= 0 {
		return content[:i], content[i+1:], true
	}
	return content, "", false
}

func isDelimiter(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}

func trimLeadingBlankLine(content string) string {
	if line, rest, hasNewline := cutLine(content); hasNewline && strings.TrimSpace(line) == "" {
		return rest
	}
	return content
}

// generateTitle creates a human-readable title from file path
func generateTitle(filePath, language string) string {
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	if base == "index" {
		dir := filepath.Dir(filePath)
		if dir != "." && dir != "/" {
			base = filepath.Base(dir)
		}
	}

	title := strings.ReplaceAll(base, "_", " ")
	title = titleCase(strings.ReplaceAll(title, "-", " "))

	if language != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(language)) {
		title = fmt.Sprintf("%s - %s", title, titleCase(language))
	}

	return title
}

func titleCase(text string) string {
	words := strings.Fields(text)
	for i, word := range words {
		runes := []rune(word)
		words[i] = string(unicode.ToUpper(runes[0])) + string(runes[1:])
	}
	return strings.Join(words, " ")
}

// sectionToParent converts section name to parent navigation name
func sectionToParent(section string) string {
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
		return titleCase(strings.ReplaceAll(section, "_", " "))
	}
}

// generatePermalink creates a rooted directory-style permalink from a path.
func generatePermalink(filePath, section string) string {
	path := filepath.ToSlash(filePath)
	path = strings.TrimSuffix(path, filepath.Ext(path))

	var segments []string
	for _, segment := range strings.Split(path, "/") {
		// A leading underscore marks a collection directory on disk; it is
		// not part of the route the collection is served under.
		segment = sanitizeURLSegment(strings.TrimPrefix(segment, "_"))
		if segment == "" || segment == "." {
			continue
		}
		segments = append(segments, segment)
	}

	// An index page addresses its own directory rather than a child route.
	if last := len(segments) - 1; last >= 0 && segments[last] == "index" {
		segments = segments[:last]
	}

	if section != "" {
		name := sanitizeURLSegment(strings.TrimPrefix(section, "_"))
		if name != "" && (len(segments) == 0 || segments[0] != name) {
			segments = append([]string{name}, segments...)
		}
	}

	if len(segments) == 0 {
		return "/"
	}
	return "/" + strings.Join(segments, "/") + "/"
}

// sanitizeURLSegment keeps a permalink a single-line URL path. A file name is
// attacker-controlled input: a newline would terminate the YAML value and "#"
// or "?" would truncate the route the site actually serves.
func sanitizeURLSegment(segment string) string {
	var out strings.Builder
	for _, r := range segment {
		switch {
		case r < 0x20 || r == 0x7f || unicode.IsSpace(r):
			out.WriteRune('-')
		case strings.ContainsRune("\"'#?&%<>\\^{}|`:;@=+,$[]()", r):
			// Reserved, unsafe, or requiring escaping in a URL path.
		default:
			out.WriteRune(r)
		}
	}

	collapsed := out.String()
	for strings.Contains(collapsed, "--") {
		collapsed = strings.ReplaceAll(collapsed, "--", "-")
	}
	return strings.Trim(collapsed, "-.")
}

// preferExisting returns existing value if non-empty, otherwise returns new value
func preferExisting(existing, new string) string {
	if existing != "" {
		return existing
	}
	return new
}
