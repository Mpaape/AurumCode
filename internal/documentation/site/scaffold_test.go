package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readGeneratedFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	return string(data)
}

// TestScaffoldGeneratesIndexAndConfig is the base guarantee: given generated
// markdown and nothing else, the scaffold produces the two files a static host
// needs before it can serve anything.
func TestScaffoldGeneratesIndexAndConfig(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go", "ledger.md"), "---\n"+
		"title: Ledger\npermalink: /go/ledger/\n---\n\n"+
		"# ledger\n\n## Index\n\n## func AddMoney\n\nAdds money.\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "tinyrepo"}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !result.ConfigCreated {
		t.Error("_config.yml should have been created")
	}
	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %+v, want the single generated page", result.Pages)
	}

	index := readGeneratedFile(t, result.IndexPath)

	for _, want := range []string{"layout: default", "permalink: /", "[Ledger](/go/ledger/)", "func AddMoney", "### Go"} {
		if !strings.Contains(index, want) {
			t.Errorf("index.md is missing %q:\n%s", want, index)
		}
	}

	// "Index" is gomarkdoc's table-of-contents heading, not a documented symbol.
	if strings.Contains(index, "- `Index`") {
		t.Errorf("index.md lists the table-of-contents heading as a symbol:\n%s", index)
	}

	siteConfig := readGeneratedFile(t, result.ConfigPath)
	for _, want := range []string{`title: "tinyrepo"`, "theme:", "markdown: kramdown"} {
		if !strings.Contains(siteConfig, want) {
			t.Errorf("_config.yml is missing %q:\n%s", want, siteConfig)
		}
	}
}

// TestScaffoldKeepsExistingConfig pins that a consumer's own site
// configuration is never clobbered by a regeneration.
func TestScaffoldKeepsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "_config.yml")
	writeFile(t, configPath, "title: hand written\n")
	writeFile(t, filepath.Join(dir, "go", "ledger.md"), "# ledger\n\n## func AddMoney\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if result.ConfigCreated {
		t.Error("an existing _config.yml must not be reported as created")
	}
	if got := readGeneratedFile(t, configPath); got != "title: hand written\n" {
		t.Errorf("_config.yml was overwritten: %q", got)
	}
}

// TestScaffoldPreservesIntroAndReplacesListing covers the enrichment contract:
// prose written above the generated listing survives regeneration, and the
// listing itself is replaced rather than appended twice.
func TestScaffoldPreservesIntroAndReplacesListing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go", "ledger.md"), "# ledger\n\n## func AddMoney\n")
	writeFile(t, filepath.Join(dir, "index.md"), "---\ntitle: Home\n---\n\n# Welcome\n\nHand written intro.\n")

	scaffold := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir})

	first, err := scaffold.Generate()
	if err != nil {
		t.Fatalf("first Generate failed: %v", err)
	}
	firstIndex := readGeneratedFile(t, first.IndexPath)

	if _, err := scaffold.Generate(); err != nil {
		t.Fatalf("second Generate failed: %v", err)
	}
	secondIndex := readGeneratedFile(t, first.IndexPath)

	if firstIndex != secondIndex {
		t.Errorf("regeneration is not idempotent:\nfirst:\n%s\nsecond:\n%s", firstIndex, secondIndex)
	}
	if !strings.Contains(secondIndex, "Hand written intro.") {
		t.Errorf("the existing introduction was dropped:\n%s", secondIndex)
	}
	if got := strings.Count(secondIndex, "Generated API documentation"); got != 1 {
		t.Errorf("the generated listing appears %d times, want 1:\n%s", got, secondIndex)
	}
}

// TestScaffoldLinksPagesWithoutPermalink pins the fallback for pages that carry
// no permalink: Jekyll publishes page.md as page.html.
func TestScaffoldLinksPagesWithoutPermalink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "python", "app.md"), "# app\n\n## class Ledger\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	index := readGeneratedFile(t, result.IndexPath)
	if !strings.Contains(index, "(python/app.html)") {
		t.Errorf("index.md does not link to python/app.html:\n%s", index)
	}
	if !strings.Contains(index, "### Python") {
		t.Errorf("index.md does not group the page under Python:\n%s", index)
	}
}

// TestScaffoldWithoutPagesStillWritesSite keeps an empty run honest: the site
// is still servable, and it says outright that nothing was documented.
func TestScaffoldWithoutPagesStillWritesSite(t *testing.T) {
	dir := t.TempDir()

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: filepath.Join(dir, "missing")}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	index := readGeneratedFile(t, result.IndexPath)
	if !strings.Contains(index, "No documentation pages were generated") {
		t.Errorf("an empty run must say so on the index page:\n%s", index)
	}
	if _, err := os.Stat(result.ConfigPath); err != nil {
		t.Errorf("_config.yml must exist even for an empty run: %v", err)
	}
}

func TestExtractSymbols(t *testing.T) {
	body := "# ledger\n\n" +
		"```go\n## not a heading, it is code\n```\n\n" +
		"## Index\n\n" +
		"<a name=\"AddMoney\"></a>\n" +
		"## func [AddMoney](<#AddMoney>)\n\n" +
		"### type `Entry`\n\n" +
		"## func AddMoney\n"

	got := extractSymbols(body)
	want := []string{"func AddMoney", "type Entry"}

	if len(got) != len(want) {
		t.Fatalf("extractSymbols = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("extractSymbols[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitFrontMatter(t *testing.T) {
	frontMatter, body := splitFrontMatter("---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n")

	if frontMatterValue(frontMatter, "title") != "Ledger" {
		t.Errorf("title = %q", frontMatterValue(frontMatter, "title"))
	}
	if frontMatterValue(frontMatter, "permalink") != "/go/ledger/" {
		t.Errorf("permalink = %q", frontMatterValue(frontMatter, "permalink"))
	}
	if !strings.HasPrefix(body, "\n# ledger") {
		t.Errorf("body = %q", body)
	}

	if _, plain := splitFrontMatter("# no front matter\n"); plain != "# no front matter\n" {
		t.Errorf("a page without front matter must be returned unchanged, got %q", plain)
	}
}
