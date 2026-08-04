package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot locates the repository root from this package's directory so the
// data-level tests below read the real .aurumcode/ tree rather than a fixture.
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// TestScaffoldDoesNotIndexProductInputDirectories pins the ghost-index defect:
// collectPages walked every .md under OutputDir, so the product's own prompt
// templates and rule files - inputs that live inside the default OutputDir
// (.aurumcode) and are read by internal/documentation/welcome - were indexed as
// if they were generated documentation pages.
//
// Two things go wrong when they are. They are excluded by the site config, so
// every link 404s; and their Go template placeholders are copied into index.md,
// where Liquid parses "{{" before markdown ever runs and fails the build.
func TestScaffoldDoesNotIndexProductInputDirectories(t *testing.T) {
	dir := t.TempDir()

	// A real generated documentation page.
	writeFile(t, filepath.Join(dir, "go", "ledger.md"),
		"---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n\n## func AddMoney\n")

	// Product input that happens to live under the same root.
	writeFile(t, filepath.Join(dir, "prompts", "documentation.md"),
		"---\ntitle: Documentation\npermalink: /prompts/documentation/\n---\n\n"+
			"# Documentation Generation Prompt\n\n## Language: {{.Language}}\n\n{{.DiffContent}}\n")
	writeFile(t, filepath.Join(dir, "prompts", "documentation", "welcome-page.md"),
		"---\ntitle: Welcome Page\n---\n\n# Welcome Page Prompt\n\n## Input\n")
	writeFile(t, filepath.Join(dir, "rules", "quality.md"),
		"---\ntitle: Quality\n---\n\n# Quality rules\n\n## Threshold\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "tinyrepo"}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, page := range result.Pages {
		if strings.HasPrefix(page.Source, "prompts/") || strings.HasPrefix(page.Source, "rules/") {
			t.Errorf("indexed a product input file as a generated page: %+v", page)
		}
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d (%+v), want only the single generated page", len(result.Pages), result.Pages)
	}

	index := readGeneratedFile(t, result.IndexPath)

	// The placeholders must never reach the landing page: Liquid would fail on them.
	for _, leak := range []string{"{{.Language}}", "{{.DiffContent}}", "{{"} {
		if strings.Contains(index, leak) {
			t.Errorf("index.md leaks the template placeholder %q:\n%s", leak, index)
		}
	}
	if strings.Contains(index, "Welcome Page") || strings.Contains(index, "Quality") {
		t.Errorf("index.md advertises a product input page:\n%s", index)
	}
}

// TestScaffoldIndexesExactlyThePagesTheRunWrote pins the stronger contract: when
// the caller states which files this run actually produced, the index is built
// from that list alone. Walking the output directory guesses; this knows. Any
// pre-existing markdown sharing the directory is then irrelevant by construction.
func TestScaffoldIndexesExactlyThePagesTheRunWrote(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go", "ledger.md"),
		"---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n\n## func AddMoney\n")
	// Written by an earlier run / by hand. Not produced by this run.
	writeFile(t, filepath.Join(dir, "go", "stale.md"),
		"---\ntitle: Stale\npermalink: /go/stale/\n---\n\n# stale\n")
	writeFile(t, filepath.Join(dir, "prompts", "documentation.md"),
		"---\ntitle: Prompt\n---\n\n# Prompt\n\n## Language: {{.Language}}\n")

	result, err := NewScaffold(ScaffoldConfig{
		DocsDir:        dir,
		OutputDir:      dir,
		Title:          "tinyrepo",
		GeneratedPages: []string{filepath.Join(dir, "go", "ledger.md")},
	}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result.Pages) != 1 {
		t.Fatalf("Pages = %d (%+v), want exactly the one page this run wrote", len(result.Pages), result.Pages)
	}
	if result.Pages[0].Source != "go/ledger.md" {
		t.Errorf("Source = %q, want go/ledger.md", result.Pages[0].Source)
	}

	index := readGeneratedFile(t, result.IndexPath)
	if strings.Contains(index, "Stale") {
		t.Errorf("index.md lists a page this run did not write:\n%s", index)
	}
	if strings.Contains(index, "{{") {
		t.Errorf("index.md leaks a template placeholder:\n%s", index)
	}
}

// TestScaffoldReportsPagesHiddenByExistingConfigExclude is the test whose absence
// let the contradiction ship. The scaffold refuses to overwrite a consumer's
// _config.yml - a deliberate promise - but it also never read the file, so it
// happily wrote an index full of links to pages that same config excludes from
// the build. Every one of those links is a 404, and nothing in the run said so.
//
// Not overwriting is right. Staying silent is not.
func TestScaffoldReportsPagesHiddenByExistingConfigExclude(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "bash", "deploy.sh.md"),
		"---\ntitle: deploy.sh\npermalink: /bash/deploy.sh/\n---\n\n# deploy.sh\n")
	writeFile(t, filepath.Join(dir, "bash", "build.sh.md"),
		"---\ntitle: build.sh\npermalink: /bash/build.sh/\n---\n\n# build.sh\n")
	writeFile(t, filepath.Join(dir, "go", "ledger.md"),
		"---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n")

	// A consumer-owned config that excludes the directory the index links to.
	// The inline comment is exactly the shape this repository's own config uses.
	configPath := filepath.Join(dir, "_config.yml")
	writeFile(t, configPath, "title: hand written\n"+
		"exclude:\n"+
		"  - Gemfile\n"+
		"  - bash/              # Build scripts, not API documentation\n"+
		"  - vendor/\n"+
		"theme: just-the-docs\n")

	result, err := NewScaffold(ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "tinyrepo"}).Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if result.ConfigCreated {
		t.Fatal("the consumer's _config.yml must not be replaced")
	}
	if got := readGeneratedFile(t, configPath); !strings.Contains(got, "hand written") {
		t.Fatalf("the consumer's _config.yml was rewritten:\n%s", got)
	}

	// The run must name the pages it listed but the site will not publish.
	want := map[string]bool{"bash/build.sh.md": false, "bash/deploy.sh.md": false}
	for _, hidden := range result.ExcludedPages {
		if _, ok := want[hidden]; !ok {
			t.Errorf("ExcludedPages reports %q, which the config does not hide", hidden)
			continue
		}
		want[hidden] = true
	}
	for page, reported := range want {
		if !reported {
			t.Errorf("ExcludedPages does not report %q, which 'exclude: bash/' hides", page)
		}
	}

	if len(result.ExcludedPages) == 0 {
		t.Fatal("ExcludedPages is empty: the run accepted a config that hides every page it just advertised")
	}
}

// TestAurumcodeIndexHasExactlyOneFrontMatter is a data test over this
// repository's own published site root. An older normalizer stamped a second
// front matter on top of the scaffold's, and Jekyll only consumes the first:
// the second block is served to readers as literal text on the home page.
func TestAurumcodeIndexHasExactlyOneFrontMatter(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".aurumcode", "index.md")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	frontMatter, body := splitFrontMatter(string(data))
	if frontMatter == "" {
		t.Fatalf("%s has no front matter:\n%s", path, data)
	}

	if strings.HasPrefix(strings.TrimSpace(body), "---") {
		t.Errorf("%s stacks a second front matter that Jekyll renders as visible text:\n%s",
			path, strings.TrimSpace(body)[:min(200, len(strings.TrimSpace(body)))])
	}
	for _, dup := range []string{"layout:", "title:"} {
		if strings.Contains(body, "\n"+dup) || strings.HasPrefix(strings.TrimSpace(body), dup) {
			t.Errorf("%s leaks front-matter key %q into the rendered body", path, dup)
		}
	}

	// "/./" is not a path Jekyll can serve as the site root.
	if permalink := frontMatterValue(frontMatter, "permalink"); permalink != "/" {
		t.Errorf("permalink = %q, want %q", permalink, "/")
	}
}

// TestAurumcodeConfigDoesNotExcludeItsOwnGeneratedPages closes the loop on the
// live data: whatever the site config excludes must not be a directory whose
// pages the scaffold indexes. bash/ holds generated shdoc pages that the index
// links, so excluding it publishes a list of 404s.
func TestAurumcodeConfigDoesNotExcludeItsOwnGeneratedPages(t *testing.T) {
	root := filepath.Join(repoRoot(t), ".aurumcode")

	configData, err := os.ReadFile(filepath.Join(root, "_config.yml"))
	if err != nil {
		t.Fatalf("read _config.yml: %v", err)
	}
	excludes := parseExcludeList(string(configData))
	if len(excludes) == 0 {
		t.Fatal("parsed no exclude entries from .aurumcode/_config.yml; the test would pass vacuously")
	}

	pages, err := NewScaffold(ScaffoldConfig{DocsDir: root, OutputDir: root}).collectPages()
	if err != nil {
		t.Fatalf("collectPages: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("collectPages found no pages under .aurumcode; the test would pass vacuously")
	}

	for _, page := range pages {
		if hidden, pattern := excludeHidesPage(excludes, page.Source); hidden {
			t.Errorf("index lists %q but _config.yml excludes it via %q (published as 404)", page.Source, pattern)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
