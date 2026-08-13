package sitepublish

// The publisher's one claim is that it is a verbatim transform: what the
// generated markdown carries becomes the corresponding element, and nothing
// appears that the markdown does not carry. AUR-429's lanes enforce that
// claim end to end (breaks applied to the markdown must flip the verdict);
// these tests pin it at the seam.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePage(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

func readPublished(t *testing.T, dir, rel string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("published page: %v", err)
	}
	return string(content)
}

const generatedIndex = `---
layout: default
title: Home
permalink: /
---

# fixture documentation

## Generated API documentation

- [Root - Go](/go/root/)
`

const generatedPage = `---
title: Root - Go
layout: default
permalink: /go/root/
---

# Package goproject

#### func NewGreeting

NewGreeting builds a Greeting for name in the given language.

` + "```go\nfunc NewGreeting(name, lang string) *Greeting\n```\n"

func TestPublishDocsRendersTheGeneratedTreeUnderItsPermalinks(t *testing.T) {
	docs := t.TempDir()
	writePage(t, docs, "index.md", generatedIndex)
	writePage(t, docs, "go/root.md", generatedPage)

	out := filepath.Join(t.TempDir(), "published")
	routes, err := PublishDocs(docs, out)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(routes) != 2 || routes["/"] == "" || routes["/go/root/"] == "" {
		t.Fatalf("expected the two permalinks as routes, got %v", routes)
	}

	index := readPublished(t, out, "index.html")
	for _, want := range []string{
		"<h2>Generated API documentation</h2>",
		`<a href="/go/root/">Root - Go</a>`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("published index lacks %q:\n%s", want, index)
		}
	}

	page := readPublished(t, out, "go/root/index.html")
	for _, want := range []string{
		"<h4>func NewGreeting</h4>",
		"<p>NewGreeting builds a Greeting for name in the given language.</p>",
		"<pre><code>func NewGreeting(name, lang string) *Greeting",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("published page lacks %q:\n%s", want, page)
		}
	}
}

func TestPublishDocsInventsNothing(t *testing.T) {
	docs := t.TempDir()
	writePage(t, docs, "index.md", "---\ntitle: Home\npermalink: /\n---\n")

	out := filepath.Join(t.TempDir(), "published")
	if _, err := PublishDocs(docs, out); err != nil {
		t.Fatalf("publish: %v", err)
	}

	index := readPublished(t, out, "index.html")
	body := index[strings.Index(index, "<body>")+len("<body>") : strings.Index(index, "</body>")]
	for _, forbidden := range []string{"<a ", "<h", "<li", "<p>"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("an empty page grew %q the markdown never carried:\n%s", forbidden, index)
		}
	}
	if text := strings.TrimSpace(body); text != "" {
		t.Errorf("an empty page grew text the markdown never carried: %q", text)
	}
}

func TestPublishDocsEscapesInsteadOfInjecting(t *testing.T) {
	docs := t.TempDir()
	writePage(t, docs, "index.md",
		"---\ntitle: Home\npermalink: /\n---\n\n# T\n\nTexto com <script>alert(1)</script> dentro.\n")

	out := filepath.Join(t.TempDir(), "published")
	if _, err := PublishDocs(docs, out); err != nil {
		t.Fatalf("publish: %v", err)
	}

	index := readPublished(t, out, "index.html")
	if strings.Contains(index, "<script>") {
		t.Fatalf("markdown text injected an element:\n%s", index)
	}
	if !strings.Contains(index, "&lt;script&gt;") {
		t.Fatalf("markdown text was not escaped:\n%s", index)
	}
}

func TestPublishDocsRoutesAPageWithoutAPermalinkByItsPath(t *testing.T) {
	docs := t.TempDir()
	writePage(t, docs, "guia/extra.md", "# Extra\n\nSem front matter.\n")

	out := filepath.Join(t.TempDir(), "published")
	routes, err := PublishDocs(docs, out)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if routes["/guia/extra.html"] == "" {
		t.Fatalf("expected the path-derived route, got %v", routes)
	}
	if !strings.Contains(readPublished(t, out, "guia/extra.html"), "<h1>Extra</h1>") {
		t.Fatal("path-routed page lost its heading")
	}
}

func TestPublishDocsIsDeterministic(t *testing.T) {
	docs := t.TempDir()
	writePage(t, docs, "index.md", generatedIndex)
	writePage(t, docs, "go/root.md", generatedPage)

	first := filepath.Join(t.TempDir(), "one")
	second := filepath.Join(t.TempDir(), "two")
	if _, err := PublishDocs(docs, first); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := PublishDocs(docs, second); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for _, rel := range []string{"index.html", "go/root/index.html"} {
		if readPublished(t, first, rel) != readPublished(t, second, rel) {
			t.Errorf("published %s differs between two runs over the same input", rel)
		}
	}
}

func TestPublishDocsRefusesAMissingTree(t *testing.T) {
	if _, err := PublishDocs(filepath.Join(t.TempDir(), "nao-existe"), t.TempDir()); err == nil {
		t.Fatal("a missing docs tree published successfully")
	}
}
