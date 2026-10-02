package main

// AUR-536 behavior proof: closes two gaps AUR-515's own review left open.
//
//   - AC-001: a checkout verified as the reviewed repository at the
//     reviewed head commit (AUR-515) can still carry an untracked file.
//     verifiedCleanCheckoutReason (aur536.go) must still catch it: the
//     untracked file's own path -- the fixture's observable marker here --
//     must never reach the provider prompt, and the published review must
//     declare why.
//   - AC-002: codebaseContextMismatch (aur515.go) already returns
//     codebaseContextReasonUnverifiable for three cases -- no local git
//     metadata at all, a checkout with no configured "origin", and a
//     failure fetching the pull request's head SHA from the GitHub API --
//     but AUR-515 never actually drove any of them through the real --pr
//     path. These tests are that missing proof: codebase context is
//     omitted and the omission is published, for each case, with the
//     marker absent and the reason-specific wording present (so a mutation
//     that papers over one case with another's wording still gets caught).
//
// All four tests reuse aur515_test.go's fixture/server/env helpers
// (aur515Fixture, aur515Server, aur515Env) and aur476_test.go's loose-object
// helpers (gitObject, treeEntry) already in this package.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur536UntrackedMarkerPath is the relative path of the untracked file
// TestAUR536UntrackedFileOmitsContextFromPrompt adds on top of an otherwise
// clean, verified checkout. Its path is the fixture's marker: the resolver
// can only ever surface it through a Reference edge's File field (it
// references the committed aur515LocalMarker symbol below), never through
// Pack.Symbols (which comes only from the diff's own changed paths), so
// this path reaching the prompt is specific evidence of the dirty tree
// leaking, not a coincidence of some other field.
const aur536UntrackedMarkerPath = "zz_aur536_untracked_marker.go"

// TestAUR536UntrackedFileOmitsContextFromPrompt covers AC-001.
func TestAUR536UntrackedFileOmitsContextFromPrompt(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")

	// An uncommitted, untracked file on top of the otherwise clean,
	// verified checkout -- it references the committed aur515LocalMarker
	// symbol so the pre-AUR-536 resolver would have recorded a Reference
	// edge naming this file's own path.
	untracked := filepath.Join(dir, aur536UntrackedMarkerPath)
	content := "package demo\n\n// references " + aur515LocalMarker + "\nvar _ = \"" + aur515LocalMarker + "\"\n"
	if err := os.WriteFile(untracked, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var posted struct{ Body string }
	server := aur515Server(t, localHead, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), aur536UntrackedMarkerPath) {
		t.Fatalf("an untracked file's path must never reach the provider prompt as context:\n%s", string(captured))
	}
	if !strings.Contains(posted.Body, "Repository context omitted") {
		t.Fatalf("published review does not declare the dirty-tree omission:\n%s", posted.Body)
	}
	if !strings.Contains(posted.Body, "uncommitted changes, untracked files, or content that does not match the reviewed commit") {
		t.Fatalf("published review does not use the dirty-tree-specific wording:\n%s", posted.Body)
	}
}

// aur536UnverifiableBareDir builds a plain directory with a committed-style
// app.go carrying aur515LocalMarker, but NO ".git" at all, and chdir's the
// test into it. This is AC-002's first unverifiable case: there is no local
// git metadata to even attempt an identity check against.
func aur536UnverifiableBareDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	app := filepath.Join(dir, "app.go")
	body := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n")
	if err := os.WriteFile(app, body, 0600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir
}

// assertContextOmittedUnverifiable is the shared assertion for AC-002's
// three cases: exit 0, the marker absent from the captured prompt, and the
// published review declaring the codebaseContextReasonUnverifiable-specific
// wording -- not just the generic "Repository context omitted" prefix the
// dirty-tree case (AC-001) also uses, so a mutation that collapses every
// reason into one cannot slip past this by reusing another case's text.
func assertContextOmittedUnverifiable(t *testing.T, code int, stdout, stderr string, capturePath string, posted string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("an unverifiable checkout's content must never reach the provider prompt as context:\n%s", string(captured))
	}
	if !strings.Contains(posted, "Repository context omitted") {
		t.Fatalf("published review does not declare the omitted codebase context:\n%s", posted)
	}
	if !strings.Contains(posted, "the local checkout's identity could not be confirmed") {
		t.Fatalf("published review does not use the unverifiable-specific wording:\n%s", posted)
	}
}

// TestAUR536NoGitMetadataOmitsContext covers AC-002's first case: a
// checkout with no ".git" at all.
func TestAUR536NoGitMetadataOmitsContext(t *testing.T) {
	aur536UnverifiableBareDir(t)

	var posted struct{ Body string }
	server := aur515Server(t, "head-sha", &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	assertContextOmittedUnverifiable(t, code, stdout.String(), stderr.String(), capturePath, posted.Body)
}

// TestAUR536NoOriginRemoteOmitsContext covers AC-002's second case: a real
// checkout, at the reviewed head, with no configured "origin" remote at
// all, so the local repository identity can never be read.
func TestAUR536NoOriginRemoteOmitsContext(t *testing.T) {
	aur515Fixture(t, "")

	var posted struct{ Body string }
	server := aur515Server(t, "head-sha", &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	assertContextOmittedUnverifiable(t, code, stdout.String(), stderr.String(), capturePath, posted.Body)
}

// aur536MetadataFailureServer serves aur515DiffBody for the diff Accept
// header (so the remote diff review still runs) but fails the pull
// request metadata fetch (GetPullRequestMetadata) with a 500 -- AC-002's
// third unverifiable case: the API call resolvePullRequestHeadSHA needs
// simply errors.
func aur536MetadataFailureServer(t *testing.T, posted *struct{ Body string }) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(aur515DiffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			posted.Body = buf.String()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestAUR536PullRequestMetadataFailureOmitsContext covers AC-002's third
// case: the local checkout IS owner/repo, but the GitHub API call for the
// pull request's reviewed head SHA fails, so there is nothing to verify
// the local HEAD against.
func TestAUR536PullRequestMetadataFailureOmitsContext(t *testing.T) {
	aur515Fixture(t, "https://github.com/owner/repo.git")

	var posted struct{ Body string }
	server := aur536MetadataFailureServer(t, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	assertContextOmittedUnverifiable(t, code, stdout.String(), stderr.String(), capturePath, posted.Body)
}
