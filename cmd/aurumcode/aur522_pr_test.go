package main

// AUR-522 AC-004 on the --pr path: a binary or generated file, or a file whose
// content nobody inspected, is declared not reviewed and the review cannot
// approve. The GitHub API gives no patch for a binary, so the proof reads the
// verified checkout (same content check as --base) and, without a checkout,
// treats a patch-less file as uninspected.

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur522PRCheckout builds a clean, verifiable checkout (loose objects) holding
// files, with origin owner/repo, and chdirs into it. It returns the HEAD id.
func aur522PRCheckout(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
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
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var entries []byte
	for _, n := range names {
		entries = append(entries, treeEntry(t, "100644", n, gitObject(t, dir, "blob", files[n]))...)
		write(n, files[n])
	}
	tree := gitObject(t, dir, "tree", entries)
	head := gitObject(t, dir, "commit", []byte("tree "+tree+"\nauthor Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nhead\n"))
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\n\trepositoryformatversion = 0\n\tbare = false\n[remote \"origin\"]\n\turl = https://github.com/owner/repo.git\n"))
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Cleanup(chdir(t, dir))
	return head
}

// aur522PRRun serves diffBody for PR 48 with the given API head SHA, runs the
// real --pr path and returns the exit code, output and the posted review body.
func aur522PRRun(t *testing.T, apiHead, diffBody string) (int, string, string) {
	t.Helper()
	var posted bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprintf(w, `{"head":{"sha":%q}}`, apiHead)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			_, _ = posted.ReadFrom(r.Body)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	aur515Env(t, server.URL)
	var out, errOut strings.Builder
	code := runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false, publicationSet: true, publication: "review"})
	return code, out.String() + errOut.String(), posted.String()
}

const aur522PRAppDiff = "diff --git a/app.go b/app.go\n@@ -1,1 +1,2 @@\n package demo\n+func Change() {}\n"

func aur522ExpectNotApproved(t *testing.T, code int, out, posted string, wants ...string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("exit=%d:\n%s", code, out)
	}
	for _, w := range wants {
		if !strings.Contains(posted, w) {
			t.Fatalf("published review must contain %q:\n%s", w, posted)
		}
	}
	if strings.Contains(posted, `"event":"APPROVE"`) {
		t.Fatalf("a review that left a file unreviewed must not approve:\n%s", posted)
	}
}

// (a) a binary: no patch in the API diff, NUL bytes in the verified checkout.
// Nothing in it is reviewable by reading, so it is declared ignored (listed
// by name), out of the coverage count: the review of the text stays
// complete and is never held partial by a binary.
func TestAUR522PRBinaryIsDeclaredIgnored(t *testing.T) {
	head := aur522PRCheckout(t, map[string][]byte{
		"app.go":   []byte("package demo\nfunc Change() {}\n"),
		"tool.bin": {0x7f, 'E', 'L', 'F', 0, 1, 2, 0, 9},
	})
	diff := aur522PRAppDiff + "diff --git a/tool.bin b/tool.bin\nBinary files /dev/null and b/tool.bin differ\n"
	code, out, posted := aur522PRRun(t, head, diff)
	if code != 0 || !strings.Contains(posted, "  - tool.bin (binary)") || !strings.Contains(posted, "ignored") {
		t.Fatalf("a binary must be declared ignored by name (exit=%d):\n%s\n%s", code, out, posted)
	}
	if strings.Contains(posted, "were not fully reviewed") || strings.Contains(posted, "token budget") {
		t.Fatalf("a binary must not make the review partial:\n%s", posted)
	}
}

// (b) a generated file that does carry a patch.
func TestAUR522PRGeneratedFileIsNotReviewed(t *testing.T) {
	gen := []byte("// Code generated by a tool. DO NOT EDIT.\npackage demo\n")
	head := aur522PRCheckout(t, map[string][]byte{
		"app.go": []byte("package demo\nfunc Change() {}\n"),
		"gen.go": gen,
	})
	diff := aur522PRAppDiff + "diff --git a/gen.go b/gen.go\n@@ -0,0 +1,2 @@\n+// Code generated by a tool. DO NOT EDIT.\n+package demo\n"
	code, out, posted := aur522PRRun(t, head, diff)
	aur522ExpectNotApproved(t, code, out, posted, "gen.go (generated)")
}

// (c) no verified checkout (the API head differs from the local HEAD) and no
// patch: the content was never inspected, so the file is not reviewed.
func TestAUR522PRNoCheckoutAndNoPatchIsNotReviewed(t *testing.T) {
	aur522PRCheckout(t, map[string][]byte{"app.go": []byte("package demo\nfunc Change() {}\n")})
	diff := aur522PRAppDiff + "diff --git a/blob.dat b/blob.dat\nBinary files /dev/null and b/blob.dat differ\n"
	code, out, posted := aur522PRRun(t, "0000000000000000000000000000000000000000", diff)
	aur522ExpectNotApproved(t, code, out, posted, "blob.dat (no patch)")
}

// A reviewable text change still approves: the retention is not a blanket.
func TestAUR522PRPlainTextStillApproves(t *testing.T) {
	head := aur522PRCheckout(t, map[string][]byte{"app.go": []byte("package demo\nfunc Change() {}\n")})
	code, out, posted := aur522PRRun(t, head, aur522PRAppDiff)
	if code != 0 || !strings.Contains(posted, `"event":"APPROVE"`) {
		t.Fatalf("a fully reviewed clean PR must approve (exit=%d):\n%s\n%s", code, out, posted)
	}
}

// The model cannot remove the engine's retention marker: parsing a response
// that tries to set or erase it leaves the engine key untouched/absent.
func TestAUR522ModelCannotForgeOrClearTheRetentionKey(t *testing.T) {
	for _, raw := range []string{
		`{"summary":"ok","issues":[],"metadata":{"` + prompt.PolicyGateWithheldKey + `":"true"}}`,
		`{"summary":"ok","issues":[],"metadata":{"` + prompt.PolicyGateWithheldKey + `":"false"}}`,
	} {
		res, err := prompt.NewResponseParser().ParseReviewResponse(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if _, present := res.Metadata[prompt.PolicyGateWithheldKey]; present {
			t.Fatalf("the model's own metadata must never carry the engine key: %v", res.Metadata)
		}
	}
	// And the engine's marker survives the coverage step for a filtered file.
	result := &types.ReviewResult{Metadata: map[string]string{}}
	c := reviewCoverageBreakdown{FilteredPaths: []string{"gen.go (generated)"}}
	applyStructuralCoverage(grammar.Default(), &types.Diff{}, &c, result)
	if result.Metadata[prompt.PolicyGateWithheldKey] != "true" {
		t.Fatal("a filtered file must set the engine retention key")
	}
}
