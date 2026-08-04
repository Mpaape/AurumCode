package e2e

// This file closes the gap between the two halves of the acceptance story.
//
// smoke_test.go proves that the binary writes .aurumcode/index.md, _config.yml
// and the per-symbol markdown. internal/qa/browserproof proves that a served
// site puts a promised effect on screen. Neither one proves the other: a smoke
// test that reads bytes off disk never navigates, and a browser proof pointed at
// a hand-written fixture proves the fixture.
//
// What runs below is one chain. The generator produces the markdown, that
// markdown is published as the HTML a static host would serve, and browserproof
// navigates the published tree: the index route has to exist, it has to carry a
// link that reaches the symbol page, and the symbol page has to show the symbol
// under the declared selector.
//
// The publishing step is a stand-in and is named as one. The pinned container
// (golang:1.21-alpine) has no Ruby, so kramdown/Jekyll cannot run there. What
// publishGeneratedSite does is a verbatim transform: headings, links, list items
// and code fences of the generated markdown become the corresponding elements,
// and nothing else is added — it never invents an index, never invents a link
// and never invents page text. That claim is not prose: the three deliberate
// breaks in TestBrowserChainDeliberateBreaksRefuseWithDistinctCodes each delete
// one of those three things from the *generated markdown* and require the chain
// to refuse, which a publisher that synthesised any of them could not do.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

const (
	// indexHeading is written by the site scaffold, not by this test.
	indexHeading = "Generated API documentation"

	// indexSelector pins the index page's heading, which the site scaffold
	// writes and therefore stays stable across documentation tools.
	indexSelector = "h2"

	// symbolSelector deliberately reads the whole rendered body. Which heading
	// level carries the documented symbol is a choice the documentation tool
	// makes — the docgen stand-in emits it as an h2 while real gomarkdoc puts
	// an "Index" section h2 first — so pinning a heading level asserts the
	// tool, not the site. What a reader must find on the symbol's page is the
	// symbol itself, anywhere in the rendered text.
	symbolSelector = "body"

	proofCard         = "tests/e2e/browser-chain"
	proofNavDeadline  = 15 * time.Second
	generatedIndexRel = "index.md"
)

// publishedSite is the HTML tree a static host would serve for one generated
// documentation directory, plus the generated file each route came from.
type publishedSite struct {
	root   string
	routes map[string]string
}

// TestBrowserChainProvesTheGeneratedSiteIsNavigable is the chain itself: real
// binary, real generated markdown, published, then navigated.
func TestBrowserChainProvesTheGeneratedSiteIsNavigable(t *testing.T) {
	docsDir := generateSite(t)
	site := publishGeneratedSite(t, docsDir)
	indexRoute, symbolRoute := routesOf(t, site)

	result, err := proveSite(t, site, indexRoute, symbolRoute)
	if err != nil {
		t.Fatalf("the generated site was not proved navigable: %v\nverdict: %+v", err, result)
	}

	if !result.Proved || result.Outcome != browserproof.OutcomeProved {
		t.Fatalf("verdict is not proof: proved=%v outcome=%q code=%q", result.Proved, result.Outcome, result.Code)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("the verdict does not carry the evidence it claims: %v", err)
	}
	if len(result.Routes) != 2 {
		t.Fatalf("expected two observed routes, got %d", len(result.Routes))
	}
	if result.ArtifactDigest == "" {
		t.Error("a proved verdict must name the bytes it navigated")
	}

	for _, route := range result.Routes {
		if route.Assertion != browserproof.AssertionPass || !route.Reachable || route.Status != 200 {
			t.Errorf("route %s: assertion=%s reachable=%v status=%d",
				route.Route, route.Assertion, route.Reachable, route.Status)
		}
		t.Logf("observed %s selector=%s text=%q", route.Route, route.Selector, route.ObservedText)
	}

	// The symbol page must show the symbol, not merely answer 200.
	symbol := result.Routes[1]
	if !strings.Contains(symbol.ObservedText, documentedSymbol) {
		t.Errorf("the symbol page rendered %q, which does not show %s", symbol.ObservedText, documentedSymbol)
	}
}

// TestBrowserChainDeliberateBreaksRefuseWithDistinctCodes breaks one link of the
// chain at a time, in the generated markdown, and requires the run to refuse.
// Each break has to be told apart from the others, so a regression cannot be
// waved through as "some other failure".
func TestBrowserChainDeliberateBreaksRefuseWithDistinctCodes(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(t *testing.T, docsDir string)
		code    string
	}{
		{
			name:    "index page deleted",
			corrupt: func(t *testing.T, docsDir string) { removeGeneratedIndex(t, docsDir) },
			code:    browserproof.CodeTargetAbsent,
		},
		{
			name:    "index keeps no link to the symbol page",
			corrupt: func(t *testing.T, docsDir string) { removeIndexLink(t, docsDir) },
			code:    browserproof.CodeUnreachableRoute,
		},
		{
			name:    "symbol page renders nothing",
			corrupt: func(t *testing.T, docsDir string) { emptySymbolPage(t, docsDir) },
			code:    browserproof.CodeEmptyRender,
		},
	}

	seen := map[string]string{}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			docsDir := generateSite(t)

			// The routes are read before the break so the run still asks for the
			// same two routes the intact site promised.
			indexRoute, symbolRoute := routesOf(t, publishGeneratedSite(t, docsDir))

			testCase.corrupt(t, docsDir)
			site := publishGeneratedSite(t, docsDir)

			result, err := proveSite(t, site, indexRoute, symbolRoute)
			if err == nil {
				t.Fatalf("breaking %q still produced proof: %+v", testCase.name, result)
			}
			if result.Proved {
				t.Fatalf("a refused run must not report proof: %+v", result)
			}
			if result.Code != testCase.code {
				t.Fatalf("expected %s, got %s (%s): %s", testCase.code, result.Code, result.Outcome, result.Detail)
			}
			if result.Outcome != browserproof.OutcomeRefused {
				t.Fatalf("a broken artifact must be refused, not diagnosed as %s: %s", result.Outcome, result.Detail)
			}

			t.Logf("break %q -> %s (%s): %s", testCase.name, result.Code, result.Outcome, result.Detail)
			seen[testCase.code] = testCase.name
		})
	}

	if len(seen) != len(cases) {
		t.Fatalf("the three breaks did not produce three distinct codes: %v", seen)
	}
}

// TestBrowserChainScriptedDoubleCannotReportSuccessOnItsOwn is named after what
// it guards. Without a pinned browser this chain navigates through the in-process
// double, so the whole result would be theatre if the double could hand out a
// passing observation. It cannot: the harness keeps its own copy of every byte it
// served, and a claim that copy does not back is an inconclusive diagnosis.
func TestBrowserChainScriptedDoubleCannotReportSuccessOnItsOwn(t *testing.T) {
	docsDir := generateSite(t)
	site := publishGeneratedSite(t, docsDir)
	indexRoute, symbolRoute := routesOf(t, site)

	forgeries := map[string]browserproof.ForgeMode{
		"claims success after navigating": browserproof.ForgeClaimSuccess,
		"claims success without fetching": browserproof.ForgeWithoutFetch,
	}

	for name, mode := range forgeries {
		mode := mode
		t.Run(name, func(t *testing.T) {
			driver := browserproof.NewScriptedDriver()
			driver.Forge = mode

			lock := browserproof.DriverLock{
				Kind:   browserproof.ScriptedDriverKind,
				Digest: browserproof.ScriptedDriverDigest,
			}

			result, err := runProof(t, driver, lock, site, indexRoute, symbolRoute)
			if err == nil || result.Proved {
				t.Fatalf("a forging driver produced proof over the generated site: %+v", result)
			}
			if result.Outcome != browserproof.OutcomeInconclusive {
				t.Fatalf("an uncorroborated observation is a harness diagnosis, got %s: %s", result.Outcome, result.Detail)
			}
			if result.Code != browserproof.CodeObservationUnsupported {
				t.Fatalf("expected %s, got %s: %s", browserproof.CodeObservationUnsupported, result.Code, result.Detail)
			}

			t.Logf("forged observation refused: %s (%s): %s", result.Code, result.Outcome, result.Detail)
		})
	}
}

// generateSite runs the real binary over the fixture repository and returns the
// documentation directory it wrote. The unsupported-language file is removed so
// this test is about navigation, not about the partial verdict smoke_test.go
// already covers.
func generateSite(t *testing.T) string {
	t.Helper()

	repo := copyFixture(t)
	if err := os.Remove(filepath.Join(repo, "Manifest.java")); err != nil {
		t.Fatalf("remove unsupported fixture file: %v", err)
	}

	result := runGenerator(t, repo, nil, toolPATH(t))
	if result.exitCode != 0 {
		t.Fatalf("generation must exit 0, got %d\n%s", result.exitCode, result.output)
	}

	docsDir := filepath.Join(repo, ".aurumcode")

	// The generated bytes are the ground truth of every assertion below, so the
	// texts this test navigates for are confirmed to come from the product.
	index := readFile(t, filepath.Join(docsDir, generatedIndexRel))
	if !strings.Contains(index, indexHeading) {
		t.Fatalf("the generated index does not carry %q:\n%s", indexHeading, index)
	}
	if _, err := os.Stat(filepath.Join(docsDir, "_config.yml")); err != nil {
		t.Fatalf("the generated site has no _config.yml: %v", err)
	}
	symbolPage := readFile(t, symbolPagePath(t, docsDir))
	if !strings.Contains(symbolPage, documentedSymbol) {
		t.Fatalf("the generated symbol page does not document %s:\n%s", documentedSymbol, symbolPage)
	}

	return docsDir
}

// proveSite runs the proof with whatever driver this environment pins.
func proveSite(t *testing.T, site publishedSite, indexRoute, symbolRoute string) (browserproof.BrowserProofResultV1, error) {
	t.Helper()

	driver, lock := resolveProofDriver(t)
	return runProof(t, driver, lock, site, indexRoute, symbolRoute)
}

func runProof(
	t *testing.T,
	driver browserproof.Driver,
	lock browserproof.DriverLock,
	site publishedSite,
	indexRoute, symbolRoute string,
) (browserproof.BrowserProofResultV1, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	return browserproof.New(driver).Run(ctx, browserproof.RunRequest{
		Card:       proofCard,
		SiteDir:    site.root,
		EntryRoute: indexRoute,
		Assertions: []browserproof.RouteAssertion{
			{Route: indexRoute, Selector: indexSelector, ExpectedText: indexHeading},
			{Route: symbolRoute, Selector: symbolSelector, ExpectedText: documentedSymbol},
		},
		DriverLock:         lock,
		NavigationDeadline: proofNavDeadline,
	})
}

// resolveProofDriver reports honestly what navigated. A pinned browser is used
// when one is configured; otherwise the in-process double runs and says so, and
// the lock pins that double so it can never pass for a browser.
func resolveProofDriver(t *testing.T) (browserproof.Driver, browserproof.DriverLock) {
	t.Helper()

	if path := strings.TrimSpace(os.Getenv(browserproof.DriverPathEnv)); path != "" {
		digest, err := browserproof.DriverDigestOf(path)
		if err != nil {
			// A configured driver that cannot be identified is a broken
			// environment, not a passing or failing feature.
			t.Fatalf("%s=%s is configured but unusable: %v", browserproof.DriverPathEnv, path, err)
		}

		t.Logf("navigation driver: pinned external headless browser %s (%s)", path, digest)
		return browserproof.NewExternalDriver(path),
			browserproof.DriverLock{Kind: browserproof.ExternalDriverKind, Digest: digest}
	}

	t.Logf("navigation driver: in-process scripted double (%s is unset, no browser pinned); "+
		"see TestBrowserChainScriptedDoubleCannotReportSuccessOnItsOwn for why it cannot self-certify",
		browserproof.DriverPathEnv)

	return browserproof.NewScriptedDriver(),
		browserproof.DriverLock{Kind: browserproof.ScriptedDriverKind, Digest: browserproof.ScriptedDriverDigest}
}

// routesOf names the two routes the chain is about: the site root and the page
// of the documented symbol. Both are read out of the generated markdown, so a
// generator that changes where it publishes moves this test with it.
func routesOf(t *testing.T, site publishedSite) (indexRoute, symbolRoute string) {
	t.Helper()

	for route, source := range site.routes {
		if filepath.Base(source) == generatedIndexRel {
			indexRoute = route
			continue
		}
		if symbolRoute != "" {
			t.Fatalf("the fixture must publish exactly one symbol page, found %s and %s", symbolRoute, route)
		}
		symbolRoute = route
	}

	if indexRoute == "" {
		t.Fatalf("the generated site publishes no index route: %v", site.routes)
	}
	if symbolRoute == "" {
		t.Fatalf("the generated site publishes no symbol page: %v", site.routes)
	}

	return indexRoute, symbolRoute
}

func symbolPagePath(t *testing.T, docsDir string) string {
	t.Helper()

	var found string
	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if filepath.Base(path) == generatedIndexRel {
			return nil
		}
		found = path
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated documentation: %v", err)
	}
	if found == "" {
		t.Fatalf("no generated symbol page under %s", docsDir)
	}

	return found
}

// ---------------------------------------------------------------------------
// Deliberate breaks. Each one removes exactly one link of the chain from the
// generated markdown; none of them touches the publisher or the proof.
// ---------------------------------------------------------------------------

func removeGeneratedIndex(t *testing.T, docsDir string) {
	t.Helper()

	if err := os.Remove(filepath.Join(docsDir, generatedIndexRel)); err != nil {
		t.Fatalf("break the index: %v", err)
	}
}

// removeIndexLink deletes the list entry that points at the symbol page and
// leaves the rest of the index intact, which is what a scaffold that stopped
// listing a generated page would produce.
func removeIndexLink(t *testing.T, docsDir string) {
	t.Helper()

	path := filepath.Join(docsDir, generatedIndexRel)
	content := readFile(t, path)

	var kept []string
	removed := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [") {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if removed == 0 {
		t.Fatalf("the generated index carried no page link to remove:\n%s", content)
	}

	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatalf("break the index link: %v", err)
	}
}

// emptySymbolPage keeps the page and its route and drops everything a reader
// would see, which is what a documentation tool that produced an empty file
// would leave behind.
func emptySymbolPage(t *testing.T, docsDir string) {
	t.Helper()

	path := symbolPagePath(t, docsDir)
	frontMatter, _ := splitGeneratedFrontMatter(readFile(t, path))
	if strings.TrimSpace(frontMatter) == "" {
		t.Fatalf("the generated symbol page has no front matter to keep: %s", path)
	}

	if err := os.WriteFile(path, []byte("---\n"+frontMatter+"\n---\n"), 0o644); err != nil {
		t.Fatalf("break the symbol page: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Publishing stand-in: the markdown the generator wrote, rendered as the HTML a
// static host publishes. It adds no content of its own.
// ---------------------------------------------------------------------------

func publishGeneratedSite(t *testing.T, docsDir string) publishedSite {
	t.Helper()

	site := publishedSite{
		root:   filepath.Join(t.TempDir(), "published"),
		routes: map[string]string{},
	}
	if err := os.MkdirAll(site.root, 0o755); err != nil {
		t.Fatalf("create published root: %v", err)
	}

	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
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

		frontMatter, body := splitGeneratedFrontMatter(string(source))

		// The route comes from the page's own front matter. A page the
		// generator stops publishing is a page this publisher stops serving.
		route := frontMatterField(frontMatter, "permalink")
		if route == "" {
			route = "/" + strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative)) + ".html"
		}

		target := filepath.Join(site.root, filepath.FromSlash(strings.TrimPrefix(route, "/")))
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

		site.routes[route] = path
		return nil
	})
	if err != nil {
		t.Fatalf("publish generated markdown: %v", err)
	}

	return site
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
	// would put literal angle brackets into the page text a reader sees,
	// which browserproof rightly refuses as markup-in-text.
	rawAnchorLine = regexp.MustCompile(`^<a\s+(?:name|id)="[^"]*"\s*>\s*</a>$`)
)

// renderMarkdown turns the generated markdown into HTML. It only ever maps a
// construct that is present in the source onto its element; it emits nothing for
// an input that carries nothing.
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

// splitGeneratedFrontMatter separates the YAML block the normalizer writes from
// the body it precedes.
func splitGeneratedFrontMatter(content string) (frontMatter, body string) {
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
