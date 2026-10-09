package main

// AUR-517 behavior proof: the published summary must never re-present, as a
// current defect, an accusation that the scope/evidence gate (filterModelIssues)
// or the rule gate (enforceRuleCitations) already removed from result.Issues,
// and the verdict/text pair must describe the same outcome on every sink:
// the --base terminal report, a plain PR comment, and a formal PR review.
//
// Fixtures reuse gitObject/treeEntry (aur476_test.go, same package) for the
// --base path and the httptest GitHub pattern (also aur476_test.go) for the
// --pr path. Every diff here touches real source lines (app.go), never an
// empty diff.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur517AppDiff is the one pull-request diff every AC-517 scenario uses:
// app.go gains a ReadAll function that discards an error on line 8 (the
// added "data, _ := fetch()" line). Lines 1-5 are unchanged context, so
// line 1 is NOT an added line and any finding anchored there is out of the
// gate's scope.
const aur517AppDiff = `diff --git a/app.go b/app.go
@@ -1,5 +1,12 @@
 package demo

 func Greet(name string) string {
 	return "hi " + name
 }
+
+func ReadAll() []byte {
+	data, _ := fetch()
+	return data
+}
+
+func fetch() ([]byte, error) { return nil, nil }
`

// aur517Response builds a controlled model answer: one issue (in or out of
// the diff's added lines, per inScope) plus a free-text summary that
// explicitly accuses the same defect the issue names, tagged with token so
// a test can look for it without any keyword/accusation detection in the
// product itself.
func aur517Response(token string, line int) string {
	return fmt.Sprintf(`{
  "verdict": "approve",
  "summary": %q,
  "issues": [
    {
      "file": "app.go",
      "line": %d,
      "side": "RIGHT",
      "severity": "warning",
      "rule_id": "quality/missing-error-handling",
      "message": "ReadAll ignores the error fetch returns",
      "evidence": "data, _ := fetch() discards the second return value",
      "impact": "an I/O failure from fetch is silently treated as success",
      "verification": "check err before returning data"
    }
  ]
}`, token+": ReadAll ignores the error fetch returns on app.go.", line)
}

// aur517Env wires the offline fixture and PR environment this card's runs
// need, mirroring aur476_test.go's TestAUR476PRDeclaresOmittedTests setup.
func aur517Env(t *testing.T, responseJSON string) {
	t.Helper()
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL",
		"AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(responseJSON), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
}

// aur517Server starts a GitHub fixture server that serves aur517AppDiff, an
// empty repository config, no pre-existing reviews/comments, and captures
// every POST body (formal review and/or plain comments) in order.
func aur517Server(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	posted := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pulls/"):
			_, _ = w.Write([]byte(aur517AppDiff))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			encoded := base64.StdEncoding.EncodeToString([]byte(""))
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, encoded)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost:
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			*posted = append(*posted, buf.String())
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server, posted
}

func aur517PREnv(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
}

// TestAUR517SummaryWithheldWhenAccusationOutOfScope covers AC-001: a model
// response names a finding on an unchanged line (line 1, outside the diff's
// added lines) without evidence, and a free-text summary accusing that
// exact defect. The scope gate (filterModelIssues) discards the finding (a
// proved one would become a general comment instead, docs/specs/AUR-545.md),
// and the published
// review -- comment and formal review alike -- must not carry the
// accusation the rest of the result no longer makes.
func TestAUR517SummaryWithheldWhenAccusationOutOfScope(t *testing.T) {
	const token = "AUR517-OUT-OF-SCOPE-ACCUSATION"
	unproved := strings.Replace(aur517Response(token, 1), `"evidence": "data, _ := fetch() discards the second return value"`, `"evidence": ""`, 1)
	if !strings.Contains(unproved, `"evidence": ""`) {
		t.Fatal("fixture still carries evidence; the scope gate would route it, not discard it")
	}
	aur517Env(t, unproved)
	server, posted := aur517Server(t)
	aur517PREnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: false, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if len(*posted) == 0 {
		t.Fatal("no request was posted to the PR; the test asserts nothing")
	}
	body := strings.Join(*posted, "\n")
	if body == "" {
		t.Fatal("posted body is empty")
	}
	if strings.Contains(body, token) {
		t.Fatalf("published review republished an accusation the scope gate discarded:\n%s", body)
	}
	if strings.Contains(body, "ReadAll ignores the error fetch returns") {
		t.Fatalf("published review still names the out-of-scope finding:\n%s", body)
	}
	// N3a: the withholding itself must be visible, never silent.
	if !strings.Contains(body, "Summary omitted: the model proposed 1 finding(s) without proof") {
		t.Fatalf("published review gives no visible notice that the summary was withheld:\n%s", body)
	}
}

// TestAUR517DegradedParseNoticePublished covers B1's required regression: a
// non-JSON model reply ("app.go:8: warning: ...") recovers one finding with
// no evidence, so filterModelIssues always discards it -- but the parser's
// own notice that the reply was unusable must still reach the published
// review, never be erased by the same withholding that protects AC-001.
func TestAUR517DegradedParseNoticePublished(t *testing.T) {
	aur517Env(t, "app.go:8: warning: ReadAll ignores the error\n")
	server, posted := aur517Server(t)
	aur517PREnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: false, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if len(*posted) == 0 {
		t.Fatal("no request was posted to the PR")
	}
	body := strings.Join(*posted, "\n")
	if !strings.Contains(body, "Degraded parse") {
		t.Fatalf("published review lost the degraded-parse notice:\n%s", body)
	}
}

// TestAUR517ValidFindingKeepsEvidenceAndVerdict covers AC-002: a finding
// anchored on an actually-added line (line 8) survives every gate with its
// evidence, impact and rule citation intact, the verdict matches that
// surviving finding (changes_requested, since it is a warning), and the
// eligible model summary remains visible.
func TestAUR517ValidFindingKeepsEvidenceAndVerdict(t *testing.T) {
	const token = "AUR517-ELIGIBLE-CHANGE-SUMMARY"
	aur517Env(t, aur517Response(token, 8))
	server, posted := aur517Server(t)
	aur517PREnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: false, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if len(*posted) == 0 {
		t.Fatal("no request was posted to the PR")
	}
	body := strings.Join(*posted, "\n")
	for _, want := range []string{
		token, // the eligible change summary stays visible
		"data, _ := fetch() discards the second return value",      // evidence
		"an I/O failure from fetch is silently treated as success", // impact
		"quality/missing-error-handling",                           // rule citation
		"Blocked: 1 problem must be fixed",                         // matching decision
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("published review missing %q:\n%s", want, body)
		}
	}
}

// TestAUR517SameDecisionAcrossSinks covers AC-003: the --base terminal
// report, a plain PR comment, and a formal PR review -- built from the same
// controlled response and diff -- state the same decision
// (Changes requested / REQUEST_CHANGES), never an approval on one sink and a
// blocking outcome on another.
func TestAUR517SameDecisionAcrossSinks(t *testing.T) {
	const token = "AUR517-SAME-DECISION"
	responseJSON := aur517Response(token, 8)

	// --- --base (local) ---
	localDir := aur517LocalFixture(t)
	aur517Env(t, responseJSON)
	restore := chdir(t, localDir)
	var localOut, localErr strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &localOut, &localErr, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("runReview exit=%d stdout=%s stderr=%s", code, localOut.String(), localErr.String())
	}
	restore()
	if !strings.Contains(localOut.String(), "[!CAUTION]") {
		t.Fatalf("local --base report did not request changes:\n%s", localOut.String())
	}

	// --- PR plain comments ---
	aur517Env(t, responseJSON)
	commentServer, commentPosted := aur517Server(t)
	aur517PREnv(t, commentServer)
	var commentOut, commentErr strings.Builder
	code = runPRReview(reviewIO{stdout: &commentOut, stderr: &commentErr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: false, check: false,
		publicationSet: true,
		publication:    "comments",
	})
	if code != 0 {
		t.Fatalf("runPRReview(comments) exit=%d stdout=%s stderr=%s", code, commentOut.String(), commentErr.String())
	}
	commentBody := strings.Join(*commentPosted, "\n")
	if !strings.Contains(commentBody, "[!CAUTION]") {
		t.Fatalf("PR comment did not request changes:\n%s", commentBody)
	}

	// --- PR formal review ---
	aur517Env(t, responseJSON)
	reviewServer, reviewPosted := aur517Server(t)
	aur517PREnv(t, reviewServer)
	var reviewOut, reviewErr strings.Builder
	code = runPRReview(reviewIO{stdout: &reviewOut, stderr: &reviewErr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: false, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview(review) exit=%d stdout=%s stderr=%s", code, reviewOut.String(), reviewErr.String())
	}
	reviewBody := strings.Join(*reviewPosted, "\n")
	if !strings.Contains(reviewBody, "[!CAUTION]") || !strings.Contains(reviewBody, `"event":"REQUEST_CHANGES"`) {
		t.Fatalf("formal review did not request changes:\n%s", reviewBody)
	}
}

// TestAUR517QualityDegradedLocalVerdictIsComment covers N1: with --base and
// no LLM provider configured at all (quality review skipped), main.go must
// mark result.Metadata["quality_degraded"]="true" so the local report's
// canonicalized verdict stays "Comment" -- never "Approve" -- matching the
// explicit "LLM quality review did not run" line printed just above it
// (AUR-449/AUR-458). Turning that metadata assignment into a no-op removes
// the only signal canonicalVerdict has for this path and must fail this
// test.
func TestAUR517QualityDegradedLocalVerdictIsComment(t *testing.T) {
	dir := aur517LocalFixture(t)
	restore := chdir(t, dir)
	defer restore()

	var stdout, stderr strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &stdout, &stderr, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("runReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "LLM quality review did not run") {
		t.Fatalf("expected the quality-skip notice, got:\n%s", out)
	}
	if !strings.Contains(out, "[!WARNING]") || strings.Contains(out, "[!TIP]") {
		t.Fatalf("quality-degraded local report did not read inconclusive:\n%s", out)
	}
}

// aur517LocalFixture builds a tiny real git repository whose HEAD diff
// (base..HEAD) is line-for-line identical to aur517AppDiff: app.go gains the
// same ReadAll function with the same error discarded on line 8. It mirrors
// aur476_test.go's coverageFixture/gitObject/treeEntry pattern, without that
// fixture's hardcoded-secret content (which would otherwise taint this
// card's own verdict assertions with an unrelated static-analysis finding).
func aur517LocalFixture(t *testing.T) string {
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

	baseContent := "package demo\n\nfunc Greet(name string) string {\n\treturn \"hi \" + name\n}\n"
	headContent := baseContent + "\nfunc ReadAll() []byte {\n\tdata, _ := fetch()\n\treturn data\n}\n\nfunc fetch() ([]byte, error) { return nil, nil }\n"

	appBase := gitObject(t, dir, "blob", []byte(baseContent))
	appHead := gitObject(t, dir, "blob", []byte(headContent))
	rootBase := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", appBase))
	rootHead := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", appHead))

	commit := func(tree, parent, msg string) string {
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\n" + msg + "\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	base := commit(rootBase, "", "base")
	head := commit(rootHead, base, "head")

	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(headContent))

	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	return dir
}
