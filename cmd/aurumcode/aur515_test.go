package main

// AUR-515 behavior proof: the --pr path's codebase-context pass only ever
// reads the local checkout when that checkout is verified as the reviewed
// repository at the reviewed head commit. These tests drive the real
// `--pr` path through the httptest GitHub pattern cloned from
// TestAUR476PRDeclaresOmittedTests / TestAUR518PRPolicyWarningReachesPublishedReview,
// with the AUR-476 git-object fixture helpers (gitObject, treeEntry) reused
// to build a local checkout with no git binary required, plus a configured
// "origin" remote so the local repository/commit identity can be compared
// against the pull request under review.

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur515LocalMarker is a symbol name that exists only in the LOCAL
// checkout's app.go, never in the remote diff the fixture GitHub server
// below serves. If the codebase-context pass ever reads this file without
// verifying the checkout first, this name reaches the resolved Pack's
// Symbols and, from there, the captured provider prompt.
const aur515LocalMarker = "AUR515OnlyInLocalCheckoutNeverInTheRemoteDiff"

// aur515Fixture builds a local git checkout (loose objects only, no git
// binary required to create it) whose tracked app.go carries
// aur515LocalMarker, with an "origin" remote configured to remoteURL. It
// returns the checkout directory and its HEAD commit hex id.
func aur515Fixture(t *testing.T, remoteURL string) (dir, headSHA string) {
	t.Helper()
	dir = t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	appLocal := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n")
	appBlob := gitObject(t, dir, "blob", appLocal)
	rootTree := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", appBlob))
	commitBody := "tree " + rootTree + "\n" +
		"author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nhead\n"
	head := gitObject(t, dir, "commit", []byte(commitBody))

	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	cfg := "[core]\n\trepositoryformatversion = 0\n\tbare = false\n"
	if remoteURL != "" {
		cfg += "[remote \"origin\"]\n\turl = " + remoteURL + "\n"
	}
	write(".git/config", []byte(cfg))
	write("app.go", appLocal)

	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir, head
}

// aur515DiffBody is the remote diff a pull request proposes: a hardcoded
// secret in app.go, the same deterministic finding
// (hardcodedSecretMessage, aur518_test.go) AUR-476/AUR-518 already use to
// prove the remote diff path independently of any model content.
const aur515DiffBody = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n"

// aur515Server starts a fixture GitHub server for owner/repo pull request
// 48: it serves aur515DiffBody for the diff Accept header, apiHeadSHA for
// the metadata Accept header (GetPullRequestMetadata), empty config/empty
// history, and captures the single posted formal review body into posted.
func aur515Server(t *testing.T, apiHeadSHA string, posted *struct{ Body string }) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(aur515DiffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprintf(w, `{"head":{"sha":%q}}`, apiHeadSHA)
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

// aur515Env wires the --pr path's standard env for these tests: the offline
// LLM fixture, the fixture GitHub server, endpoint-mode permissions and a
// capture file for the exact prompt sent to the provider.
func aur515Env(t *testing.T, serverURL string) (capturePath string) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"reviewed","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	capturePath = filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capturePath)
	t.Setenv("AURUMCODE_GITHUB_API_URL", serverURL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	return capturePath
}

// TestAUR515DifferentRepoOmitsContextFromPrompt covers AC-001: --pr for
// owner/repo, run inside a checkout whose own origin names a different
// repository, must never let that checkout's code reach the provider
// prompt, and the published review must declare the omission.
func TestAUR515DifferentRepoOmitsContextFromPrompt(t *testing.T) {
	_, localHead := aur515Fixture(t, "https://github.com/other/repo.git")

	var posted struct{ Body string }
	server := aur515Server(t, localHead, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
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
	if strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("the other repository's local checkout content reached the provider prompt:\n%s", string(captured))
	}
	if !strings.Contains(posted.Body, "Repository context omitted") {
		t.Fatalf("published review does not declare the omitted codebase context:\n%s", posted.Body)
	}
}

// TestAUR515MatchingHeadUsesContext covers the positive half of AC-002: a
// checkout that IS owner/repo at the exact SHA the API reports as the pull
// request's head gets its codebase context used -- the local marker
// symbol reaches the provider prompt.
func TestAUR515MatchingHeadUsesContext(t *testing.T) {
	_, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")

	var posted struct{ Body string }
	server := aur515Server(t, localHead, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
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
	if !strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("a verified matching checkout's codebase context did not reach the provider prompt:\n%s", string(captured))
	}
	if strings.Contains(posted.Body, "Repository context omitted") {
		t.Fatalf("a verified matching checkout should not carry the omission limitation:\n%s", posted.Body)
	}
}

// TestAUR515DivergentHeadOmitsContextButKeepsDiffReview covers the negative
// half of AC-002 plus AC-003: the checkout IS owner/repo, but at a
// different commit than the pull request's reported head. The codebase
// context must be omitted (with a declared limitation) while the remote
// diff review -- the hardcoded-secret finding from aur515DiffBody -- is
// still published unchanged.
func TestAUR515DivergentHeadOmitsContextButKeepsDiffReview(t *testing.T) {
	aur515Fixture(t, "https://github.com/owner/repo.git")
	const divergentHeadSHA = "0000000000000000000000000000000000000000"

	var posted struct{ Body string }
	server := aur515Server(t, divergentHeadSHA, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
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
	if strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("a checkout at a divergent HEAD must not reach the provider prompt as context:\n%s", string(captured))
	}
	if !strings.Contains(posted.Body, "Repository context omitted") {
		t.Fatalf("published review does not declare the divergent-HEAD omission:\n%s", posted.Body)
	}
	if !strings.Contains(posted.Body, hardcodedSecretMessage) {
		t.Fatalf("the remote diff review must still be published when context is omitted (AC-003):\n%s", posted.Body)
	}
}
