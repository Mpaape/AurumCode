package site

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// markdownLinkRE matches only links a CommonMark/kramdown parser will actually
// resolve: the label carries no unescaped bracket and the destination carries no
// whitespace and no unescaped parenthesis. A link the scaffold emits that this
// pattern cannot find is a link the rendered site will not have.
var markdownLinkRE = regexp.MustCompile(`\[((?:[^\[\]\\]|\\.)*)\]\(([^()\s]*)\)`)

// unescapeMarkdown reverses the backslash escaping of a link label.
func unescapeMarkdown(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) {
			i++
			b.WriteByte(text[i])
			continue
		}
		b.WriteByte(text[i])
	}
	return b.String()
}

// TestScaffoldConfigIsValidYAMLUnderHostileMetadata feeds the scaffold the
// characters a repository directory name may legally contain and requires the
// generated _config.yml to survive a real YAML parser with its values intact.
// The site title is derived from a directory name, so this is reachable input,
// and an unparseable _config.yml fails the whole site build.
func TestScaffoldConfigIsValidYAMLUnderHostileMetadata(t *testing.T) {
	dir := t.TempDir()

	title := "acme: \"quoted\"\nsecond line\twith tab"
	description := "Docs for #1 & C:\\share -- see {a: b}"
	baseURL := "/repo\nname"

	result, err := NewScaffold(ScaffoldConfig{
		DocsDir:     dir,
		OutputDir:   dir,
		Title:       title,
		Description: description,
		BaseURL:     baseURL,
	}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	raw := readGeneratedFile(t, result.ConfigPath)

	var parsed struct {
		Title       string `yaml:"title"`
		Description string `yaml:"description"`
		Baseurl     string `yaml:"baseurl"`
		Theme       string `yaml:"theme"`
		Markdown    string `yaml:"markdown"`
	}
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("generated _config.yml is not valid YAML: %v\n---\n%s---", err, raw)
	}

	if parsed.Title != title {
		t.Errorf("title round-trip = %q, want %q", parsed.Title, title)
	}
	if parsed.Description != description {
		t.Errorf("description round-trip = %q, want %q", parsed.Description, description)
	}
	if parsed.Baseurl != baseURL {
		t.Errorf("baseurl round-trip = %q, want %q", parsed.Baseurl, baseURL)
	}
	if parsed.Theme == "" || parsed.Markdown == "" {
		t.Errorf("the site generator needs theme and markdown keys, got %+v", parsed)
	}
}

// TestScaffoldIndexFrontMatterIsValidYAML pins that the landing page the site
// root serves starts with front matter a YAML parser accepts. Jekyll renders a
// page with broken front matter as raw text or drops it entirely.
func TestScaffoldIndexFrontMatterIsValidYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go", "ledger.md"), "# ledger\n\n## func AddMoney\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	frontMatter, body := splitFrontMatter(readGeneratedFile(t, result.IndexPath))
	if frontMatter == "" {
		t.Fatal("index.md has no front matter, the site generator will not render it as a page")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(frontMatter), &parsed); err != nil {
		t.Fatalf("index.md front matter is not valid YAML: %v\n---\n%s---", err, frontMatter)
	}
	if parsed["permalink"] != "/" {
		t.Errorf("permalink = %v, want / so the site root is not a 404", parsed["permalink"])
	}
	if !strings.Contains(body, "func AddMoney") {
		t.Errorf("index body does not list the generated symbol:\n%s", body)
	}
}

// TestScaffoldEmitsResolvableLinksForHostilePaths uses a page title and a file
// name made of characters that terminate a markdown link early. The listing is
// the only navigation the site has, so a link that no parser resolves is a page
// the reader cannot reach.
func TestScaffoldEmitsResolvableLinksForHostilePaths(t *testing.T) {
	dir := t.TempDir()

	base := "od d(1).md"
	title := "Ledger [beta] (v2)"
	writeFile(t, filepath.Join(dir, "go", base), "---\ntitle: "+title+"\n---\n\n# x\n\n## func A\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	index := readGeneratedFile(t, result.IndexPath)

	var line string
	for _, candidate := range strings.Split(index, "\n") {
		if strings.HasPrefix(candidate, "- [") {
			line = candidate
			break
		}
	}
	if line == "" {
		t.Fatalf("no page entry in index.md:\n%s", index)
	}

	matches := markdownLinkRE.FindAllStringSubmatch(line, -1)
	if len(matches) != 1 {
		t.Fatalf("entry %q yields %d parseable links, want exactly 1", line, len(matches))
	}

	gotLabel := unescapeMarkdown(matches[0][1])
	if gotLabel != title {
		t.Errorf("link label = %q, want %q", gotLabel, title)
	}

	gotTarget, err := url.PathUnescape(matches[0][2])
	if err != nil {
		t.Fatalf("link destination %q is not a valid URL path: %v", matches[0][2], err)
	}
	if want := "go/od d(1).html"; gotTarget != want {
		t.Errorf("link destination = %q (raw %q), want %q", gotTarget, matches[0][2], want)
	}
}

// TestScaffoldSectionHeadingCannotEscapeItsLine covers a directory name holding
// a newline: the listing groups pages by directory, and an unfolded name would
// let a directory inject its own headings into the landing page.
func TestScaffoldSectionHeadingCannotEscapeItsLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go\n# Injected", "page.md"), "# page\n\n## func A\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	index := readGeneratedFile(t, result.IndexPath)

	start := strings.Index(index, scaffoldBlockStart)
	end := strings.Index(index, scaffoldBlockEnd)
	if start < 0 || end < start {
		t.Fatalf("generated block is missing from index.md:\n%s", index)
	}
	block := index[start:end]

	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "# ") {
			t.Errorf("a directory name injected the heading %q into the listing:\n%s", line, block)
		}
	}
}

// TestLanguageDisplayNameKeepsNonASCIIIntact pins that titling a section name
// does not corrupt it: the name is sliced for its first letter, and slicing a
// multi-byte rune by byte produces text no reader can use.
func TestLanguageDisplayNameKeepsNonASCIIIntact(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"go", "Go"},
		{"csharp", "C#"},
		{"ölang", "Ölang"},
		{"日本語", "日本語"},
	} {
		got := languageDisplayName(tc.in)
		if !utf8.ValidString(got) {
			t.Errorf("languageDisplayName(%q) = %q, which is not valid UTF-8", tc.in, got)
		}
		if got != tc.want {
			t.Errorf("languageDisplayName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestScaffoldHonoursBaseURLInAbsoluteLinks holds the scaffold to what its own
// configuration promises: when the site is published under a base path, the
// root-absolute permalinks written into _config.yml's site must be reachable.
func TestScaffoldHonoursBaseURLInAbsoluteLinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go", "ledger.md"),
		"---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n\n## func AddMoney\n")
	writeFile(t, filepath.Join(dir, "python", "app.md"), "# app\n\n## class Ledger\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir, BaseURL: "/my-repo"}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	index := readGeneratedFile(t, result.IndexPath)
	if !strings.Contains(index, "(/my-repo/go/ledger/)") {
		t.Errorf("an absolute permalink ignores baseurl, so it 404s on a project site:\n%s", index)
	}
	// A relative link resolves against the page that contains it and must stay
	// untouched, otherwise it would pick up the base path twice.
	if !strings.Contains(index, "(python/app.html)") {
		t.Errorf("a relative link must not be rewritten:\n%s", index)
	}
}
