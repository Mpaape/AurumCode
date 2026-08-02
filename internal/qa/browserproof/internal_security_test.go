package browserproof

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The artifact root is the boundary the whole verdict rests on: a proof claims
// something about the bytes of one built site. These cases attack that boundary
// from inside the served directory, where route resolution alone cannot see the
// escape, and they name their bait rather than using anything real.

// baitMarker stands in for content the harness must never read. It is invented
// by the test, never a credential.
const baitMarker = "AURUM-BAIT-OUTSIDE-THE-ARTIFACT-1d9e"

func requireSymlinks(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
}

// baitOutside writes a decoy file in a directory that is not the artifact root
// and returns its path.
func baitOutside(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write bait: %v", err)
	}

	return path
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()

	response, err := http.Get(url) //nolint:noctx // the server is on loopback and closes with the test
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	return response.StatusCode, string(body)
}

// TestSiteServerNeverServesThroughASymlink attacks the served root from inside
// it: a link in the artifact and a linked directory both point outside, and the
// server must answer neither, keep no copy of either, and leak nothing of what
// they point at.
func TestSiteServerNeverServesThroughASymlink(t *testing.T) {
	requireSymlinks(t)

	bait := baitOutside(t, "bait.html", `<!doctype html><html><body><p>`+baitMarker+`</p></body></html>`)

	root := tempSite(t)
	if err := os.Symlink(bait, filepath.Join(root, "leak.html")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}
	if err := os.Symlink(filepath.Dir(bait), filepath.Join(root, "leakdir")); err != nil {
		t.Fatalf("symlink directory: %v", err)
	}

	server, err := startSiteServer(root)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	for _, route := range []string{"/leak.html", "/leakdir/bait.html", "/leakdir"} {
		status, body := get(t, server.URL(route))
		if status != http.StatusNotFound {
			t.Fatalf("route %s answered %d, want %d", route, status, http.StatusNotFound)
		}
		if strings.Contains(body, baitMarker) {
			t.Fatalf("route %s leaked content from outside the artifact", route)
		}

		entry, ok := server.lastServed(route)
		if !ok || entry.Status != http.StatusNotFound || entry.Body != nil || entry.Digest != "" {
			t.Fatalf("route %s left a ledger entry that backs an observation: status %d, %d body bytes",
				route, entry.Status, len(entry.Body))
		}
	}

	// The artifact's own pages are still served: the refusal is about leaving
	// the root, not about refusing everything.
	if status, _ := get(t, server.URL("/index.html")); status != http.StatusOK {
		t.Fatalf("the artifact's own page answered %d, want %d", status, http.StatusOK)
	}
}

// TestArtifactDigestNeverReadsThroughASymlink pins both halves of the artifact
// identity: what a link points at never enters the digest, and the link itself
// is still not invisible to it.
func TestArtifactDigestNeverReadsThroughASymlink(t *testing.T) {
	requireSymlinks(t)

	withLink := func(t *testing.T, body string) string {
		t.Helper()

		root := tempSite(t)
		if err := os.Symlink(baitOutside(t, "bait.html", body), filepath.Join(root, "leak.html")); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		digest, err := artifactDigest(root)
		if err != nil {
			t.Fatalf("digest artifact: %v", err)
		}

		return digest
	}

	first := withLink(t, `<!doctype html><html><body><p>`+baitMarker+`</p></body></html>`)
	second := withLink(t, "a completely different file, of a completely different length, "+baitMarker+baitMarker)
	if first != second {
		t.Fatal("what the link points at changed the artifact digest, so it was read")
	}

	plain, err := artifactDigest(tempSite(t))
	if err != nil {
		t.Fatalf("digest artifact: %v", err)
	}
	if plain == first {
		t.Fatal("an artifact carrying a link out of its root digests the same as one without it")
	}
}

// TestIrregularArtifactEntriesAreRefusedNotRead keeps the harness from reading a
// file that never ends: a pipe left in the artifact would block the digest and
// the server forever, which is a hang no deadline in this package covers.
func TestIrregularArtifactEntriesAreRefusedNotRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes are created differently on windows")
	}

	root := tempSite(t)
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.html"), 0o600); err != nil {
		t.Skipf("the filesystem under test refuses a fifo: %v", err)
	}

	if _, err := artifactDigest(root); err != nil {
		t.Fatalf("digest artifact: %v", err)
	}

	server, err := startSiteServer(root)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	if status, _ := get(t, server.URL("/pipe.html")); status != http.StatusNotFound {
		t.Fatalf("the fifo answered %d, want %d", status, http.StatusNotFound)
	}
}

// TestCorroborationRefusesARouteTheHarnessKeptNoCopyOf covers the nil-body
// refusal on its own terms: with no copy to check against, the harness reports
// that it could not judge instead of blaming the artifact for what it did not
// see. Deleting the guard turns this into a functional refusal of a page that
// was served whole.
func TestCorroborationRefusesARouteTheHarnessKeptNoCopyOf(t *testing.T) {
	served := ledgerEntry{
		Route:  "/p5.html",
		Status: http.StatusOK,
		Bytes:  4_000_000,
		Digest: "sha256:" + strings.Repeat("0", 64),
		Body:   nil, // past the ledger budget the server keeps nothing
	}

	// The driver reports a page that renders nothing, which is the one shape
	// that every later corroboration check waves through.
	observation := PageObservation{
		Status:         http.StatusOK,
		Bytes:          served.Bytes,
		SelectorFound:  true,
		Text:           "ExtractorPipeline",
		BodyTextLength: 0,
	}

	step := corroborateContent(observation, served, "#symbol-name", false)
	if step == nil {
		t.Fatal("an observation with nothing to check it against was corroborated")
	}
	if step.code != CodeObservationUnsupported {
		t.Fatalf("code = %q, want %s", step.code, CodeObservationUnsupported)
	}
	if outcomeOfCode(step.code) != OutcomeInconclusive {
		t.Fatalf("a route the harness kept no copy of was reported as %q", outcomeOfCode(step.code))
	}
}

// TestSiteServerNeverServesALinkThatStaysInsideTheRoot keeps the served set and
// the hashed set identical. artifactDigest records a link by name and never
// reads it, so serving one — even one that points at a page of this same
// artifact — would answer a route with bytes the digest never attributed to it.
func TestSiteServerNeverServesALinkThatStaysInsideTheRoot(t *testing.T) {
	requireSymlinks(t)

	root := tempSite(t)
	if err := os.Symlink(filepath.Join(root, "index.html"), filepath.Join(root, "alias.html")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	server, err := startSiteServer(root)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	if status, _ := get(t, server.URL("/alias.html")); status != http.StatusNotFound {
		t.Fatalf("a link inside the root answered %d, want %d", status, http.StatusNotFound)
	}
	if status, _ := get(t, server.URL("/index.html")); status != http.StatusOK {
		t.Fatalf("the page the link points at answered %d, want %d", status, http.StatusOK)
	}
}

// TestSiteServerServesAnArtifactReachedThroughALink keeps the containment check
// honest about its own root: a build directory named through a link is a normal
// artifact, and every page of it must still be served.
func TestSiteServerServesAnArtifactReachedThroughALink(t *testing.T) {
	requireSymlinks(t)

	real := tempSite(t)
	alias := filepath.Join(t.TempDir(), "build")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	server, err := startSiteServer(alias)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	if status, body := get(t, server.URL("/index.html")); status != http.StatusOK || !strings.Contains(body, "index") {
		t.Fatalf("an artifact named through a link answered %d", status)
	}
}

// TestArtifactDigestRefusesSomethingThatIsNotAnArtifact keeps a single file from
// being fingerprinted as a built site. The digest runs before the server, so it
// is the check that decides.
func TestArtifactDigestRefusesSomethingThatIsNotAnArtifact(t *testing.T) {
	page := filepath.Join(tempSite(t), "index.html")

	if digest, err := artifactDigest(page); err == nil {
		t.Fatalf("a single file was fingerprinted as an artifact: %s", digest)
	}
}

// TestNormalizeRouteCollapsesEverySpellingOfARoute keeps one page from being
// recorded under several keys: the ledger is looked up by route, and an
// observation checked against the wrong entry is an observation checked against
// nothing.
func TestNormalizeRouteCollapsesEverySpellingOfARoute(t *testing.T) {
	for route, want := range map[string]string{
		"/a/../index.html":    "/index.html",
		"/./index.html":       "/index.html",
		"/symbols//x.html":    "/symbols/x.html",
		"/index.html?v=2":     "/index.html",
		"/index.html#section": "/index.html",
		"index.html":          "/index.html",
		"":                    "/",
	} {
		if got := normalizeRoute(route); got != want {
			t.Errorf("normalizeRoute(%q) = %q, want %q", route, got, want)
		}
	}
}

// TestObserveRefusesARouteTheServerNeverServed is the forgery the ledger exists
// for, reduced to the seam that decides it: the driver reports a route the local
// server has no record of, and the harness must say it cannot check that instead
// of letting a later check happen to catch it.
func TestObserveRefusesARouteTheServerNeverServed(t *testing.T) {
	server, err := startSiteServer(tempSite(t))
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer server.Close()

	proof := New(&silentDriver{})

	_, _, err = proof.observe(context.Background(), server, "/never-requested.html", "", time.Second, false)
	if err == nil {
		t.Fatal("an observation of a route the server never served was accepted")
	}

	var step *stepError
	if !errors.As(err, &step) || step.code != CodeObservationUnsupported {
		t.Fatalf("want %s, got %v", CodeObservationUnsupported, err)
	}
	if !strings.Contains(step.detail, "never served") {
		t.Fatalf("the refusal must name the missing ledger entry, got %q", step.detail)
	}
}

// silentDriver reports an observation without ever reaching the server, and
// reports the status an unserved ledger entry carries, so the only thing that
// can refuse it is the ledger lookup itself.
type silentDriver struct{}

func (silentDriver) Fingerprint(context.Context) (DriverFingerprint, error) {
	return DriverFingerprint{Kind: ScriptedDriverKind, Digest: ScriptedDriverDigest}, nil
}

func (silentDriver) Navigate(context.Context, string, string) (PageObservation, error) {
	return PageObservation{Status: 0, Bytes: 0, BodyTextLength: 0}, nil
}

// TestFinalizeDowngradesAVerdictTheContractRejects is the last gate: whatever
// path produced the result, a verdict that does not satisfy the published
// contract leaves finalize as an inconclusive rejection, never as proof.
func TestFinalizeDowngradesAVerdictTheContractRejects(t *testing.T) {
	// Every field says proof, and the contract still refuses it: the route
	// carries raw markup where the extracted text belongs.
	result := BrowserProofResultV1{
		Schema:         SchemaBrowserProofResultV1,
		Card:           "AUR-423",
		ArtifactDigest: "sha256:" + strings.Repeat("a", 64),
		DriverDigest:   ScriptedDriverDigest,
		DriverKind:     ScriptedDriverKind,
		Routes: []RouteObservationV1{{
			Route:         "/index.html",
			Status:        http.StatusOK,
			Selector:      "#symbol-name",
			ObservedText:  "<h1>ExtractorPipeline</h1>",
			SelectorFound: true,
			Reachable:     true,
			Assertion:     AssertionPass,
		}},
	}

	final, err := finalize(result, "", "")
	if err == nil || final.Proved {
		t.Fatalf("a verdict the contract rejects was returned as proof: outcome %q", final.Outcome)
	}
	if final.Outcome != OutcomeInconclusive || final.Code != CodeVerdictRejected {
		t.Fatalf("outcome/code = %q/%q, want inconclusive/%s", final.Outcome, final.Code, CodeVerdictRejected)
	}
}

// TestParseSelectorRefusesASelectorWithNothingInIt keeps an empty selector from
// matching the first element of a page, which would make any text on it pass for
// the text of an assertion.
func TestParseSelectorRefusesASelectorWithNothingInIt(t *testing.T) {
	for _, selector := range []string{"", "   ", "\t\n"} {
		if spec, err := parseSelector(selector); err == nil {
			t.Fatalf("selector %q was accepted as %+v", selector, spec)
		}
	}
}
