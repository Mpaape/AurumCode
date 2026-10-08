package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/review/cistatus"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur516Review runs a real --pr review against a fake GitHub with ci as the
// workflow's CI context ("" = none supplied) and response as the model's
// answer, and returns the published review body.
func aur516Review(t *testing.T, ci, response string) string {
	t.Helper()
	var posted githubclient.PullRequestReview
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42":
			w.Header().Set("ETag", `"aur516"`)
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
	fixture := filepath.Join(dir, "response.json")
	if err := os.WriteFile(fixture, []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	ciFile := ""
	if ci != "" {
		ciFile = filepath.Join(dir, "ci.json")
		if err := os.WriteFile(ciFile, []byte(ci), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
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
	if posted.Body == "" {
		t.Fatal("no review was published")
	}
	return posted.Body
}

// aur516Copies is the review copy in both catalog locales: the published
// language is the session's, and every assertion must hold in either.
func aur516Copies() []reviewCopy {
	return []reviewCopy{reviewCopyFor("en-US"), reviewCopyFor("pt-BR")}
}

func aur516ContainsInSomeLanguage(body string, text func(reviewCopy) string) bool {
	for _, c := range aur516Copies() {
		if strings.Contains(body, text(c)) {
			return true
		}
	}
	return false
}

// TestAUR516AC001ModelSaysCIGreenWithoutContext: with no CI context at all,
// a model that reports CI green never publishes that as a verified state.
func TestAUR516AC001ModelSaysCIGreenWithoutContext(t *testing.T) {
	response := `{"summary":"ok","issues":[],"ci_analysis":[{"check":"CI","status":"success","cause":"todos os jobs passaram","evidence":"CI verde","fix":"","next_verification":"","confidence":"high"}]}`
	body := aur516Review(t, "", response)
	if strings.Contains(body, "**CI — success**") || strings.Contains(body, "— success") {
		t.Fatalf("AUR-516 model state published as CI state: the model's own status became a state of CI:\n%s", body)
	}
	for _, c := range aur516Copies() {
		if strings.Contains(body, c.ciVerified) {
			t.Fatalf("AUR-516 model state published as CI state: an item with no CI context is labeled verified:\n%s", body)
		}
	}
	if !aur516ContainsInSomeLanguage(body, func(c reviewCopy) string { return "**CI — " + c.ciUnverified + "**" }) {
		t.Fatalf("the unverified item does not say its state was not verified:\n%s", body)
	}
}

// aur516Document renders one CI analysis item through the real split and
// verification of a CI context, in language.
func aur516Document(t *testing.T, ci string, item types.CIAnalysis, language string) string {
	t.Helper()
	ctx := cistatus.Parse(ci, ownStatusPrefix)
	kept, discarded := cistatus.Facts{Context: ctx}.Keep([]types.CIAnalysis{item})
	if len(discarded) != 0 || len(kept) != 1 {
		t.Fatalf("the item was discarded: %+v", discarded)
	}
	result := &types.ReviewResult{CIAnalysis: ctx.Verify(kept)}
	return formatReviewDocument(result, nil, language, blocking.Ungated())
}

// TestAUR516AC002FailedCheckWithoutLogsGivesDiagnosis: a failed check with a
// name and link and no logs shows the observed state and link, says the
// cause is unknown, keeps the model's cause only as a hypothesis and
// publishes no fix.
func TestAUR516AC002FailedCheckWithoutLogsGivesDiagnosis(t *testing.T) {
	ci := `[{"name":"Lint","state":"FAILURE","workflow":"CI","link":"https://example.test/lint"}]`
	item := types.CIAnalysis{Check: "Lint", Status: "success", Cause: "A dependencia X quebrou", Evidence: "o lint falhou",
		Fix: "Atualize X para 2.0", NextVerification: "Rode o lint de novo"}
	for _, language := range []string{"pt-BR", "en-US"} {
		c := reviewCopyFor(language)
		body := aur516Document(t, ci, item, language)
		for _, want := range []string{
			"**Lint — failure** (" + c.ciVerified + ": https://example.test/lint)",
			"**" + c.cause + ":** " + c.ciCauseUnknown,
			"**" + c.ciHypothesis + ":** A dependencia X quebrou",
			fmt.Sprintf(c.ciDiagnose, "https://example.test/lint"),
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("(%s) missing %q:\n%s", language, want, body)
			}
		}
		for _, absent := range []string{"Atualize X para 2.0", "Rode o lint de novo", "— success"} {
			if strings.Contains(body, absent) {
				t.Fatalf("(%s) a cause or fix without evidence was published as certain (%q):\n%s", language, absent, body)
			}
		}
	}
}

// TestAUR516AC003EvidenceQuotedKeepsInferenceApart: when the CI context
// supplies a sanitized log excerpt and the model's evidence quotes it, the
// fix is published citing the observation, on separate lines from the
// model's inference. Evidence that does not quote the excerpt is AC-002.
func TestAUR516AC003EvidenceQuotedKeepsInferenceApart(t *testing.T) {
	ci := `[{"name":"Lint","state":"FAILURE","link":"https://example.test/lint","excerpt":"golangci-lint: main.go:3:1: x declared and not used (unused)\nexit 1"}]`
	item := types.CIAnalysis{Check: "Lint", Status: "failure", Cause: "variavel sem uso", Evidence: "main.go:3:1: x declared and not used",
		Fix: "Remova a variavel x", NextVerification: "Rode o lint"}
	for _, language := range []string{"pt-BR", "en-US"} {
		c := reviewCopyFor(language)
		body := aur516Document(t, ci, item, language)
		for _, want := range []string{
			"  - **" + c.ciObserved + ":** main.go:3:1: x declared and not used\n",
			"  - **" + c.ciInferredCause + ":** variavel sem uso\n",
			"  - **" + c.ciInferredFix + ":** Remova a variavel x\n",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("(%s) missing %q:\n%s", language, want, body)
			}
		}
		if strings.Contains(body, c.ciCauseUnknown) {
			t.Fatalf("(%s) grounded evidence still says the cause is unknown:\n%s", language, body)
		}
		unquoted := item
		unquoted.Evidence = "o lint reclamou de algo"
		other := aur516Document(t, ci, unquoted, language)
		if strings.Contains(other, "Remova a variavel x") || !strings.Contains(other, c.ciCauseUnknown) {
			t.Fatalf("(%s) evidence that does not quote the log was treated as observed:\n%s", language, other)
		}
	}
}
