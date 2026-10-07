package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur599CIContext is the PR #96 shape: running checks, the review job itself
// running, a concluded failure and the statuses published on the previous
// round by this product.
const aur599CIContext = `[{"name":"Build and test in OCI","state":"IN_PROGRESS","workflow":"CI","link":"https://example.test/1"},
{"name":"review / Review pull request","state":"IN_PROGRESS","workflow":"AurumCode self review","link":"https://example.test/2"},
{"name":"Lint","state":"FAILURE","workflow":"CI","link":"https://example.test/3"},
{"name":"semgrep","state":"FAILURE","workflow":"CI","link":"https://example.test/4"},
{"name":"aurumcode/policy-gate","state":"FAILURE","workflow":"","link":""}]`

// aur599Response is a model answer that invents analysis for the running
// check, the previous own status and a scanner it never called, next to the
// one concluded failure.
const aur599Response = `{"summary":"Mudança pequena.","issues":[],"ci_analysis":[
{"check":"scanner_semgrep","status":"inconclusive","cause":"CAUSA-SEMGREP","evidence":"sast_invalid_output","fix":"x","next_verification":"y","confidence":"low"},
{"check":"Build and test in OCI","status":"in_progress","cause":"CAUSA-BUILD","evidence":"IN_PROGRESS","fix":"x","next_verification":"y","confidence":"low"},
{"check":"aurumcode/policy-gate","status":"failure","cause":"CAUSA-GATE","evidence":"rodada anterior","fix":"x","next_verification":"y","confidence":"low"},
{"check":"Lint","status":"failure","cause":"CAUSA-LINT","evidence":"contexto de CI: FAILURE","fix":"x","next_verification":"y","confidence":"medium"},
{"check":"semgrep","status":"failure","cause":"CAUSA-JOB-SEMGREP","evidence":"contexto de CI: FAILURE","fix":"x","next_verification":"y","confidence":"medium"}]}`

// aur599Review runs a --pr review against a fake GitHub with the CI context
// above and returns the published body, the prompt and stderr.
func aur599Review(t *testing.T) (string, string, string) {
	t.Helper()
	var posted githubclient.PullRequestReview
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42":
			w.Header().Set("ETag", `"aur599"`)
			_, _ = w.Write([]byte("diff --git a/main.go b/main.go\n@@ -1,1 +1,3 @@\n package main\n+func main() { run() }\n+// trailing\n"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/pulls/42/reviews":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Errorf("decoding formal review: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	fixture, ciFile, capture := filepath.Join(dir, "response.json"), filepath.Join(dir, "ci.json"), filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(fixture, []byte(aur599Response), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ciFile, []byte(aur599CIContext), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", ciFile)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 42, repo: "owner/repo", publicar: true, naLinha: true,
		publicationSet: true, publication: "review"})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	sent, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	return posted.Body, string(sent), stderr.String()
}

// AC-001: the previous own status and the running checks never reach the
// model as facts, and the published CI status carries no item for them.
func TestAUR599AC001RunningAndOwnStatusesAreNotPublished(t *testing.T) {
	body, sent, errOut := aur599Review(t)
	for _, absent := range []string{`"name":"aurumcode/policy-gate"`, "IN_PROGRESS", "Review pull request"} {
		if strings.Contains(sent, absent) {
			t.Errorf("the prompt's CI context carries %q", absent)
		}
	}
	for _, absent := range []string{"CAUSA-BUILD", "CAUSA-GATE", "aurumcode/policy-gate — failure", "in_progress"} {
		if strings.Contains(body, absent) {
			t.Errorf("the published review carries %q:\n%s", absent, body)
		}
	}
	if !strings.Contains(errOut, "3 ci_analysis item(s) discarded") {
		t.Errorf("stderr does not count the discarded items:\n%s", errOut)
	}
}

// AC-002: the scanner the model never called is not in the review.
func TestAUR599AC002ScannerNotRunIsNotPublished(t *testing.T) {
	body, _, _ := aur599Review(t)
	if strings.Contains(body, "scanner_semgrep") || strings.Contains(body, "CAUSA-SEMGREP") {
		t.Fatalf("a scanner that did not run appears in the review:\n%s", body)
	}
}

// AC-003: the concluded failures stay in the prompt and in the review, even
// a CI job named like a scanner engine this review did not run.
func TestAUR599AC003ConcludedFailureIsPublished(t *testing.T) {
	body, sent, _ := aur599Review(t)
	if !strings.Contains(sent, `"name":"Lint","state":"FAILURE"`) {
		t.Errorf("the concluded failure left the prompt's CI context")
	}
	if !strings.Contains(body, "**Lint — failure**") || !strings.Contains(body, "CAUSA-LINT") {
		t.Fatalf("the concluded failure is missing from the review:\n%s", body)
	}
	if _, ok := scanner.Lookup("semgrep"); !ok {
		t.Fatal("the semgrep engine is not registered: the CI job below would not share an engine name")
	}
	if !strings.Contains(body, "**semgrep — failure**") || !strings.Contains(body, "CAUSA-JOB-SEMGREP") {
		t.Fatalf("a concluded CI job named like an engine was hidden:\n%s", body)
	}
}
