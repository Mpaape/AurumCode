// Package sitepublish renders a generated documentation tree (the Jekyll
// markdown cmd/regenerate-docs writes) as the HTML tree a static host would
// serve, so an offline verification can navigate it.
//
// It is a publish STAND-IN and is named as one: the pinned acceptance
// container has no Ruby, so kramdown/Jekyll cannot run there. What this
// package does is a verbatim transform — headings, links, list items and
// code fences of the generated markdown become the corresponding elements,
// and nothing else is added. It never invents an index, never invents a link
// and never invents page text; that claim is behaviorally enforced by
// AUR-429's lanes, whose deliberate breaks delete content from the GENERATED
// markdown and require the published site's verdict to flip. The transform
// mirrors the publisher tests/e2e/browser_chain_test.go established; that
// one is test code inside package e2e and cannot be imported from here.
package sitepublish

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PublishDocs renders every markdown page under docsDir into outDir as the
// HTML a static host publishes, and returns the route each page came from as
// route -> source path. A page's route comes from its own permalink front
// matter; a page without one publishes under its path, as Jekyll would.
func PublishDocs(docsDir, outDir string) (map[string]string, error) {
	info, err := os.Stat(docsDir)
	if err != nil {
		return nil, fmt.Errorf("sitepublish: docs directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("sitepublish: %q is not a directory", docsDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("sitepublish: create published root: %w", err)
	}

	routes := map[string]string{}
	err = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}

		relative, err := filepath.Rel(docsDir, path)
		if err != nil {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		frontMatter, body := splitFrontMatter(string(source))

		// The route comes from the page's own front matter. A page the
		// generator stops publishing is a page this publisher stops serving.
		route := frontMatterField(frontMatter, "permalink")
		if route == "" {
			route = "/" + strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative)) + ".html"
		}

		target := filepath.Join(outDir, filepath.FromSlash(strings.TrimPrefix(route, "/")))
		if strings.HasSuffix(route, "/") {
			target = filepath.Join(target, "index.html")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		page := htmlDocument(frontMatterField(frontMatter, "title"), renderMarkdown(body))
		if err := os.WriteFile(target, []byte(page), 0o644); err != nil {
			return err
		}

		routes[route] = path
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sitepublish: publish generated markdown: %w", err)
	}

	return routes, nil
}

func htmlDocument(title, body string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>")
	b.WriteString(htmlEscape(title))
	b.WriteString("</title>\n</head>\n<body>\n")
	b.WriteString(body)
	b.WriteString("</body>\n</html>\n")

	return b.String()
}

var (
	markdownHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	markdownListItem = regexp.MustCompile(`^(\s*)-\s+(.*)$`)
	markdownLink     = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)\)`)
	markdownCode     = regexp.MustCompile("`([^`]*)`")

	// rawAnchorLine is the standalone `<a name="..."></a>` line gomarkdoc
	// emits before each symbol heading. Markdown renderers pass raw inline
	// HTML through as markup, so a faithful publisher must too; escaping it
	// would put literal angle brackets into the page text a reader sees.
	rawAnchorLine = regexp.MustCompile(`^<a\s+(?:name|id)="[^"]*"\s*>\s*</a>$`)
)

// renderMarkdown turns the generated markdown into HTML. It only ever maps a
// construct that is present in the source onto its element; it emits nothing
// for an input that carries nothing.
func renderMarkdown(body string) string {
	var b strings.Builder

	var paragraph []string
	var listIndents []int
	inFence := false

	closeLists := func() {
		for len(listIndents) > 0 {
			b.WriteString("</li>\n</ul>\n")
			listIndents = listIndents[:len(listIndents)-1]
		}
	}
	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		b.WriteString("<p>" + renderInline(strings.Join(paragraph, " ")) + "</p>\n")
		paragraph = nil
	}

	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flushParagraph()
			closeLists()
			if inFence {
				b.WriteString("</code></pre>\n")
			} else {
				b.WriteString("<pre><code>")
			}
			inFence = !inFence
			continue
		}
		if inFence {
			b.WriteString(htmlEscape(raw) + "\n")
			continue
		}

		if trimmed == "" {
			flushParagraph()
			closeLists()
			continue
		}

		// gomarkdoc precedes each symbol heading with a raw anchor line. It is
		// markup, not text: pass it through like a renderer would, instead of
		// escaping it into literal angle brackets a reader would see.
		if rawAnchorLine.MatchString(trimmed) {
			flushParagraph()
			closeLists()
			b.WriteString(trimmed + "\n")
			continue
		}

		// A markdown comment is markup already; it is passed through so the
		// scaffold's own delimiters survive publication.
		if strings.HasPrefix(trimmed, "<!--") && strings.HasSuffix(trimmed, "-->") {
			flushParagraph()
			closeLists()
			b.WriteString(trimmed + "\n")
			continue
		}

		if match := markdownHeading.FindStringSubmatch(trimmed); match != nil {
			flushParagraph()
			closeLists()
			level := len(match[1])
			b.WriteString("<h" + string(rune('0'+level)) + ">" + renderInline(match[2]) +
				"</h" + string(rune('0'+level)) + ">\n")
			continue
		}

		if match := markdownListItem.FindStringSubmatch(line); match != nil {
			flushParagraph()
			indent := len(strings.ReplaceAll(match[1], "\t", "    "))

			for len(listIndents) > 0 && indent < listIndents[len(listIndents)-1] {
				b.WriteString("</li>\n</ul>\n")
				listIndents = listIndents[:len(listIndents)-1]
			}
			if len(listIndents) == 0 || indent > listIndents[len(listIndents)-1] {
				b.WriteString("<ul>\n")
				listIndents = append(listIndents, indent)
			} else {
				b.WriteString("</li>\n")
			}

			b.WriteString("<li>" + renderInline(match[2]) + "\n")
			continue
		}

		paragraph = append(paragraph, trimmed)
	}

	flushParagraph()
	closeLists()
	if inFence {
		b.WriteString("</code></pre>\n")
	}

	return b.String()
}

// renderInline escapes first and marks up afterwards, so nothing in the
// generated text can inject an element this publisher was not asked for.
func renderInline(text string) string {
	escaped := htmlEscape(text)
	escaped = markdownLink.ReplaceAllString(escaped, `<a href="$2">$1</a>`)
	escaped = markdownCode.ReplaceAllString(escaped, "<code>$1</code>")

	return escaped
}

func htmlEscape(text string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	).Replace(text)
}

// splitFrontMatter separates the YAML block the normalizer writes from the
// body it precedes.
func splitFrontMatter(content string) (frontMatter, body string) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", normalized
	}

	rest := normalized[len("---\n"):]
	if index := strings.Index(rest, "\n---\n"); index >= 0 {
		return rest[:index], rest[index+len("\n---\n"):]
	}
	if strings.HasSuffix(rest, "\n---") {
		return strings.TrimSuffix(rest, "\n---"), ""
	}

	return "", normalized
}

func frontMatterField(frontMatter, key string) string {
	for _, line := range strings.Split(frontMatter, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}

		name, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) != key {
			continue
		}

		return strings.Trim(strings.TrimSpace(value), `"'`)
	}

	return ""
}
