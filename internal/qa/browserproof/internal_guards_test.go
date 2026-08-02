package browserproof

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The guards below cannot be reached through the published API: the local server
// is never handed to a caller, and route resolution runs before any driver sees a
// URL. They are still the boundary that keeps a proof run inside the artifact, so
// they are pinned here at the seam where they live.

func tempSite(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"),
		[]byte(`<!doctype html><html><body><p>index</p></body></html>`), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}

	return root
}

// TestSiteServerRefusesWritingMethods keeps the artifact server read-only: it
// publishes a built site and answers nothing that could change it.
func TestSiteServerRefusesWritingMethods(t *testing.T) {
	server, err := startSiteServer(tempSite(t))
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		request, err := http.NewRequest(method, server.URL("/index.html"), strings.NewReader("x"))
		if err != nil {
			t.Fatalf("build %s request: %v", method, err)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		_ = response.Body.Close()

		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s answered %d, want %d", method, response.StatusCode, http.StatusMethodNotAllowed)
		}

		entry, ok := server.lastServed("/index.html")
		if !ok || entry.Status != http.StatusMethodNotAllowed || entry.Body != nil {
			t.Fatalf("%s: the ledger recorded %+v", method, entry)
		}
	}
}

// TestSiteServerServesOnlyUnderTheArtifactRoot pins the containment check: a
// route that would resolve outside the served artifact resolves to nothing.
func TestSiteServerServesOnlyUnderTheArtifactRoot(t *testing.T) {
	root := tempSite(t)
	server := &siteServer{root: root}

	outside := []string{
		"/../secrets.env",
		"/../../etc/passwd",
		"/..",
	}
	for _, route := range outside {
		if full, ok := server.resolve(route); ok {
			t.Errorf("route %q resolved to %q outside the artifact root %q", route, full, root)
		}
	}

	inside := map[string]string{
		"/":           root,
		"/index.html": filepath.Join(root, "index.html"),
		"/a/b.html":   filepath.Join(root, "a", "b.html"),
	}
	for route, want := range inside {
		full, ok := server.resolve(route)
		if !ok || full != want {
			t.Errorf("route %q resolved to %q (ok=%v), want %q", route, full, ok, want)
		}
	}
}

// TestResolveLinkNeverLeavesTheArtifact keeps the crawl on the served site: an
// href that points anywhere else is not a route of this artifact.
func TestResolveLinkNeverLeavesTheArtifact(t *testing.T) {
	external := []string{
		"https://example.com/symbols/index.html",
		"http://example.com/",
		"//example.com/symbols/index.html",
		"mailto:someone@example.com",
		"javascript:alert(1)",
		"tel:+551199999999",
		"data:text/html,<h1>x</h1>",
		"#anchor",
		"   ",
	}
	for _, href := range external {
		if route, ok := resolveLink("/symbols/index.html", href); ok {
			t.Errorf("href %q became local route %q", href, route)
		}
	}

	local := map[string]string{
		"/index.html":    "/index.html",
		"extractor.html": "/symbols/extractor.html",
		"../index.html":  "/index.html",
		"./sub/x.html":   "/symbols/sub/x.html",
		"x.html#frag":    "/symbols/x.html",
	}
	for href, want := range local {
		route, ok := resolveLink("/symbols/index.html", href)
		if !ok || route != want {
			t.Errorf("href %q resolved to %q (ok=%v), want %q", href, route, ok, want)
		}
	}
}

// TestEntryRouteIsReachableWithoutAnythingLinkingToIt pins the seed of the walk,
// which decides every BROWSERPROOF_UNREACHABLE_ROUTE verdict this package emits.
// The route a run enters through is delivered because the run reaches it, and
// every other page is delivered only because a link on a page that was served
// points at it. Drop the seed and an artifact asserted at its own entry route is
// refused as undelivered; drop the walk and an orphan page passes as delivered.
func TestEntryRouteIsReachableWithoutAnythingLinkingToIt(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		// Nothing in the artifact links to the entry route itself.
		"entry.html":  `<!doctype html><html><body><a href="/linked.html">onward</a></body></html>`,
		"linked.html": `<!doctype html><html><body><p>linked from the entry route</p></body></html>`,
		"orphan.html": `<!doctype html><html><body><p>no navigation reaches this</p></body></html>`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	server, err := startSiteServer(root)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	reachable, err := New(NewScriptedDriver()).reachableRoutes(
		context.Background(),
		server,
		RunRequest{EntryRoute: "/entry.html", NavigationDeadline: 5 * time.Second},
		false,
	)
	if err != nil {
		t.Fatalf("walk the artifact: %v", err)
	}

	for route, want := range map[string]bool{
		"/entry.html":  true,
		"/linked.html": true,
		"/orphan.html": false,
	} {
		if reachable[route] != want {
			t.Errorf("reachable[%q] = %v, want %v", route, reachable[route], want)
		}
	}
}

// TestSiteServerRefusesAnArtifactThatIsNotADirectory keeps a built site and a
// single file apart at the server's own door. Run happens to fingerprint the
// artifact first, so this refusal never decides a verdict there — which is
// exactly why it has to be pinned where it lives.
func TestSiteServerRefusesAnArtifactThatIsNotADirectory(t *testing.T) {
	for name, root := range map[string]string{
		"a single page":  filepath.Join(tempSite(t), "index.html"),
		"nothing at all": filepath.Join(t.TempDir(), "there-is-no-built-site"),
	} {
		root := root
		t.Run(name, func(t *testing.T) {
			server, err := startSiteServer(root)
			if err == nil {
				server.Close()
				t.Fatalf("%q was published as a built site", root)
			}
		})
	}
}

// TestParseSelectorRefusesAnIncompleteIdOrClass keeps a half-written selector
// from being read as a spec that matches nothing: "#" is a request the harness
// cannot evaluate, and answering it with "the page lacks that selector" would
// blame the artifact for a fault in the request.
func TestParseSelectorRefusesAnIncompleteIdOrClass(t *testing.T) {
	for _, selector := range []string{"#", ".", "h1#", "h1."} {
		if spec, err := parseSelector(selector); err == nil {
			t.Errorf("selector %q was accepted as %+v", selector, spec)
		}
	}
}

// TestAnEmptySelectorSpecMatchesNothing is the second half of that refusal: were
// a spec with no tag, no id and no class ever built, it must match no element at
// all. A spec that matches the first element of a page turns whatever text is on
// it into the text of an assertion.
func TestAnEmptySelectorSpecMatchesNothing(t *testing.T) {
	tokens := tokenizeHTML(
		`<!doctype html><html><body><h1 id="symbol-name" class="title">ExtractorPipeline</h1></body></html>`)

	for _, tok := range tokens {
		if (selectorSpec{}).matches(tok) {
			t.Fatalf("an empty selector matched <%s>", tok.name)
		}
	}

	text, found := selectorText(tokens, selectorSpec{})
	if found || text != "" {
		t.Fatalf("an empty selector extracted %q (found=%v)", text, found)
	}
}
