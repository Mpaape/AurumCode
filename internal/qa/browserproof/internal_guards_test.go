package browserproof

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
