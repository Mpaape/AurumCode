package main

// AUR-537 end-to-end behavior proof: a provider/transport failure on --pr
// (every configured provider failed the call, llm.ErrAllProvidersFailed; or
// --limite refused the call before any provider was reached,
// llm.ErrBudgetExceeded) now routes through the AUR-519 policy gate instead
// of returning before it is ever reached -- driven through the real
// runPRReview with the same httptest GitHub mock aur519_e2e_test.go's --pr
// table already uses (runPRGateMockServer/setPRGateEnv) for the no-provider
// cases, plus a local Multi variant below that records EVERY commit status
// (needed because --check publishes two: aurumcode/review and
// aurumcode/policy-gate, to the same path).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// simpleDiffAUR537 is a tiny, secret-free diff: static analysis/security
// must find nothing on it, so every assertion below is explained only by
// the provider-failure routing this card adds, never by an unrelated
// deterministic finding.
const simpleDiffAUR537 = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ _ = 1\n+}\n"

// deadProviderEnv points LLM_API_KEY/LLM_BASE_URL at a port nothing
// listens on, so litellm's single HTTP call fails immediately
// (connection refused, no retry inside the orchestrator -- see
// internal/llm/orchestrator.go's Complete) and
// reviewer.GenerateReviewWithContext returns an error wrapping
// llm.ErrAllProvidersFailed. AURUMCODE_LLM_FIXTURE is cleared so
// selectProvider actually builds this live provider instead of the
// offline fixture one.
func deadProviderEnv(t *testing.T, server *httptest.Server) {
	t.Helper()
	setPRGateEnv(t, server, "")
	t.Setenv("LLM_API_KEY", "offline-test-key")
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("AURUMCODE_LLM_TIMEOUT_SECONDS", "2")
}

// tinyBudgetEnv is the --limite/ErrBudgetExceeded counterpart to
// deadProviderEnv: a VALID, reachable fixture provider (so selectProvider
// succeeds and the orchestrator would otherwise answer fine) paired with a
// --limite ceiling so small that the pre-call cost estimate always exceeds
// it (costPrice's own default price is nonzero with no env override), so
// reviewer.GenerateReviewWithContext returns llm.ErrBudgetExceeded without
// ever calling the model.
func tinyBudgetEnv(t *testing.T, server *httptest.Server) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	setPRGateEnv(t, server, fixture)
}

type aur537Audit struct {
	Verdict string `json:"verdict"`
	Gate    struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason,omitempty"`
	} `json:"gate"`
}

type aur537SARIF struct {
	Runs []struct {
		Invocations []struct {
			ExecutionSuccessful bool `json:"executionSuccessful"`
		} `json:"invocations"`
	} `json:"runs"`
}

func readAUR537Audit(t *testing.T, path string) aur537Audit {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit aur537Audit
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, raw)
	}
	return audit
}

func readAUR537SARIF(t *testing.T, path string) aur537SARIF {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	var doc aur537SARIF
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, raw)
	}
	return doc
}

func assertInconclusiveSARIF(t *testing.T, doc aur537SARIF) {
	t.Helper()
	if len(doc.Runs) != 1 || len(doc.Runs[0].Invocations) != 1 {
		t.Fatalf("SARIF runs/invocations shape = %+v", doc.Runs)
	}
	if doc.Runs[0].Invocations[0].ExecutionSuccessful {
		t.Fatal("SARIF executionSuccessful must be false for a run that did not review")
	}
}

// runPRGateMockServerMulti is runPRGateMockServer's AUR-537/B1 variant: it
// records EVERY commit status POSTed to /statuses/head-sha into *published,
// instead of overwriting a single pointer with only the last one. A
// provider-failure run with --check publishes BOTH aurumcode/review
// (checkContext, publishCheckStatus) and aurumcode/policy-gate
// (policyGateContext, publishPolicyGateStatus) to that same path in the
// same request cycle; B1 is exactly the defect of the first one silently
// reading as a false "success" -- a single-status mock cannot even see it.
func runPRGateMockServerMulti(t *testing.T, diffBody, repoConfig string, published *[]githubclient.CommitStatus, postedBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(repoConfig)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			if postedBody != nil {
				var payload struct {
					Body string `json:"body"`
				}
				buf := new(bytes.Buffer)
				_, _ = buf.ReadFrom(r.Body)
				_ = json.Unmarshal(buf.Bytes(), &payload)
				*postedBody = payload.Body
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			var status githubclient.CommitStatus
			_ = json.NewDecoder(r.Body).Decode(&status)
			*published = append(*published, status)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s (Accept=%s)", r.Method, r.URL.Path, r.Header.Get("Accept"))
		}
	}))
}

func statusByContext(statuses []githubclient.CommitStatus, context string) (githubclient.CommitStatus, bool) {
	for _, s := range statuses {
		if s.Context == context {
			return s, true
		}
	}
	return githubclient.CommitStatus{}, false
}

// assertCheckStatusNeverSucceedsOnProviderFailure is AUR-537 B1's own
// assertion, shared by both the transport-failure and budget-refusal tests,
// for both gate.inconclusive modes: aurumcode/review (checkContext) must
// read failure and name provider_failure, REGARDLESS of gate mode and
// REGARDLESS of --exigir-qualidade -- warn only ever softens
// aurumcode/policy-gate, never this legacy, grave-finding-only status.
// Before AUR-537 this context was simply absent for this run (fails closed
// for any ruleset that requires it); it must never flip to a false
// "success" now that AUR-537 makes the function keep going.
func assertCheckStatusNeverSucceedsOnProviderFailure(t *testing.T, published []githubclient.CommitStatus) {
	t.Helper()
	review, ok := statusByContext(published, checkContext)
	if !ok {
		t.Fatalf("no %q status published at all: %+v", checkContext, published)
	}
	if review.State != "failure" {
		t.Fatalf("%q status = %+v, want state failure -- a provider outage must never read success", checkContext, review)
	}
	if !strings.Contains(review.Description, gateReasonProviderFailure) {
		t.Fatalf("%q description = %q, want it naming %q", checkContext, review.Description, gateReasonProviderFailure)
	}
}

// TestAUR537ProviderFailureBlocksGate covers AC-001 and B1: with
// gate.inconclusive: block, a provider/transport failure during the actual
// model call (not a selection failure) publishes aurumcode/policy-gate as
// "failure" naming provider_failure AND aurumcode/review as "failure"
// naming provider_failure (never a false "success"), exits
// exitQualityNotReviewed, writes an audit record whose gate.decision names
// the failure, writes a SARIF document with executionSuccessful=false, and
// the published review body says the review did not run, with the verdict
// read as Inconclusive -- never approved.
func TestAUR537ProviderFailureBlocksGate(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: block\n", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
	}
	gate, ok := statusByContext(published, policyGateContext)
	if !ok || gate.State != "failure" {
		t.Fatalf("published statuses = %+v, want %q state failure", published, policyGateContext)
	}
	if !strings.Contains(gate.Description, gateReasonProviderFailure) {
		t.Fatalf("published status description = %q, want it naming %q", gate.Description, gateReasonProviderFailure)
	}
	assertCheckStatusNeverSucceedsOnProviderFailure(t, published)
	if !strings.Contains(postedBody, "the review did not run") {
		t.Fatalf("published review body does not say the review did not run: %s", postedBody)
	}
	if !strings.Contains(postedBody, "**Verdict:** Inconclusive") {
		t.Fatalf("published review body verdict does not read Inconclusive: %s", postedBody)
	}
	if strings.Contains(postedBody, "**Verdict:** Approve") {
		t.Fatalf("published review body verdict read Approve under a blocking, inconclusive gate: %s", postedBody)
	}
	if strings.Contains(stdout.String(), `"APPROVE"`) {
		t.Fatalf("published GitHub review action read APPROVE under a blocking, inconclusive gate: %s", stdout.String())
	}

	audit := readAUR537Audit(t, auditPath)
	if audit.Verdict == "approve" {
		t.Fatal("audit verdict must never be approve for a run that did not review")
	}
	if !strings.Contains(audit.Gate.Reason, gateReasonProviderFailure) {
		t.Fatalf("audit gate.reason=%q, want it naming %q", audit.Gate.Reason, gateReasonProviderFailure)
	}
	assertInconclusiveSARIF(t, readAUR537SARIF(t, sarifPath))
}

// TestAUR537ProviderFailureWarnsGate covers AC-002 and B1: with
// gate.inconclusive: warn, the identical provider/transport failure
// publishes aurumcode/policy-gate as "success" carrying a visible
// inconclusive alert -- never the "aprovado" word -- and exits 0, BUT
// aurumcode/review must still read failure naming provider_failure: warn
// only ever softens the policy-gate's own status, never the legacy
// grave-finding one. The audit record for the same run must read
// "inconclusive", in agreement with AC-001's own run reading "fail" only
// because that run's mode was "block".
func TestAUR537ProviderFailureWarnsGate(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: warn\n", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	gate, ok := statusByContext(published, policyGateContext)
	if !ok || gate.State != "success" {
		t.Fatalf("published statuses = %+v, want %q state success", published, policyGateContext)
	}
	if strings.Contains(gate.Description, gateStatusWordApproved) {
		t.Fatalf("%q description = %q, must never read %q for a run that did not review", policyGateContext, gate.Description, gateStatusWordApproved)
	}
	if !strings.Contains(gate.Description, gateStatusWordInconclusive) {
		t.Fatalf("%q description = %q, want the inconclusive word", policyGateContext, gate.Description)
	}
	assertCheckStatusNeverSucceedsOnProviderFailure(t, published)
	if !strings.Contains(postedBody, "the review did not run") {
		t.Fatalf("published review body does not say the review did not run: %s", postedBody)
	}
	if !strings.Contains(postedBody, "**Verdict:** Inconclusive") {
		t.Fatalf("published review body verdict does not read Inconclusive: %s", postedBody)
	}

	audit := readAUR537Audit(t, auditPath)
	if audit.Gate.Decision != "inconclusive" {
		t.Fatalf("audit gate.decision=%q, want inconclusive", audit.Gate.Decision)
	}
	assertInconclusiveSARIF(t, readAUR537SARIF(t, sarifPath))
}

// TestAUR537ProviderFailureExigirQualidadeStillInconclusive is N2: with
// --exigir-qualidade and gate.inconclusive: warn, a provider outage must
// still exit exitQualityNotReviewed (never 0) and never claim approval --
// --exigir-qualidade is a stricter floor, not a way to recover a review
// that did not happen. Pins that qualityDegraded is actually set on the
// providerFailed path (pr.go's providerFailed block): removing that
// assignment would let this run slip through as a clean, unflagged pass.
func TestAUR537ProviderFailureExigirQualidadeStillInconclusive(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: warn\n", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet:  true,
		publication:     "review",
		exigirQualidade: true,
	})
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d) under --exigir-qualidade; stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
	}
	if strings.Contains(postedBody, "**Verdict:** Approve") {
		t.Fatalf("published review body verdict read Approve despite --exigir-qualidade on a provider outage: %s", postedBody)
	}
	if !strings.Contains(postedBody, "**Verdict:** Inconclusive") {
		t.Fatalf("published review body verdict does not read Inconclusive: %s", postedBody)
	}
	assertCheckStatusNeverSucceedsOnProviderFailure(t, published)
}

// TestAUR537NoGateProviderFailureUnchanged covers AC-003: with no `gate:`
// key declared anywhere, the identical provider/transport failure must stay
// byte-identical to the command's behavior before this card -- exit 1, the
// same "all providers failed" diagnosis, no status at all published (the
// mock server fails the test on any unexpected request, so a status POST
// that should not happen is caught), and no audit/SARIF file written even
// though both paths were given.
func TestAUR537NoGateProviderFailureUnchanged(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != 1 {
		t.Fatalf("exit=%d, want 1 (unchanged without a gate); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "all providers failed") {
		t.Fatalf("stderr does not name the transport failure: %s", stderr.String())
	}
	if len(published) != 0 {
		t.Fatalf("a status was published with no gate declared: %+v", published)
	}
	if _, err := os.Stat(auditPath); err == nil {
		t.Fatal("audit record was written with no gate declared -- AC-003 requires byte-identical (no-op) behavior")
	}
	if _, err := os.Stat(sarifPath); err == nil {
		t.Fatal("SARIF was written with no gate declared -- AC-003 requires byte-identical (no-op) behavior")
	}
}

// TestAUR537MutationReturnsBeforeGate is MUT-001's own anchor at the unit
// level, independent of the shell acceptance script's AC-001-MUT-001: it
// pins that providerFailed actually prevents the early return once a gate
// is declared. If a future edit restores the unconditional early return on
// llm.ErrAllProvidersFailed, this test (and AC-001 above) both fail.
func TestAUR537MutationReturnsBeforeGate(t *testing.T) {
	var published []githubclient.CommitStatus
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: block\n", &published, nil)
	defer server.Close()
	deadProviderEnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
	})
	if _, ok := statusByContext(published, policyGateContext); code == 1 && !ok {
		t.Fatal("MUT-001: the function returned before the gate was ever reached despite a declared gate")
	}
	if _, ok := statusByContext(published, policyGateContext); !ok {
		t.Fatalf("published statuses = %+v, want %q present", published, policyGateContext)
	}
}

// TestAUR537BudgetExceededBlocksGate is N1: --limite's own pre-call refusal
// (llm.ErrBudgetExceeded) must route through the gate exactly like a
// transport failure does, under gate.inconclusive: block.
func TestAUR537BudgetExceededBlocksGate(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: block\n", &published, &postedBody)
	defer server.Close()
	tinyBudgetEnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		limiteSet:      true,
		limite:         "0.0000000001",
	})
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
	}
	gate, ok := statusByContext(published, policyGateContext)
	if !ok || gate.State != "failure" || !strings.Contains(gate.Description, gateReasonProviderFailure) {
		t.Fatalf("published statuses = %+v, want %q failure naming %q", published, policyGateContext, gateReasonProviderFailure)
	}
	assertCheckStatusNeverSucceedsOnProviderFailure(t, published)
	if !strings.Contains(postedBody, "the review did not run") {
		t.Fatalf("published review body does not say the review did not run: %s", postedBody)
	}
}

// TestAUR537BudgetExceededWarnsGate is N1's warn counterpart.
func TestAUR537BudgetExceededWarnsGate(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: warn\n", &published, &postedBody)
	defer server.Close()
	tinyBudgetEnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		limiteSet:      true,
		limite:         "0.0000000001",
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	gate, ok := statusByContext(published, policyGateContext)
	if !ok || gate.State != "success" || strings.Contains(gate.Description, gateStatusWordApproved) {
		t.Fatalf("published statuses = %+v, want %q success, never %q", published, policyGateContext, gateStatusWordApproved)
	}
	assertCheckStatusNeverSucceedsOnProviderFailure(t, published)
}

// TestAUR537BudgetExceededNoGateUnchanged is N1's AC-003 counterpart: with
// no gate declared, --limite's pre-call refusal keeps today's exact
// behavior -- the same exit code reportBudgetExceeded already returns, no
// status, no audit/SARIF.
func TestAUR537BudgetExceededNoGateUnchanged(t *testing.T) {
	var published []githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "", &published, &postedBody)
	defer server.Close()
	tinyBudgetEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		limiteSet:      true,
		limite:         "0.0000000001",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != 1 {
		t.Fatalf("exit=%d, want 1 (unchanged without a gate); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing to call the model") {
		t.Fatalf("stderr does not name the budget refusal: %s", stderr.String())
	}
	if len(published) != 0 {
		t.Fatalf("a status was published with no gate declared: %+v", published)
	}
	if _, err := os.Stat(auditPath); err == nil {
		t.Fatal("audit record was written with no gate declared -- AC-003 requires byte-identical (no-op) behavior")
	}
	if _, err := os.Stat(sarifPath); err == nil {
		t.Fatal("SARIF was written with no gate declared -- AC-003 requires byte-identical (no-op) behavior")
	}
}

// TestAUR537BudgetMutationReturnsBeforeGate is N1's own mutation anchor at
// the unit level for the --limite/ErrBudgetExceeded branch specifically.
func TestAUR537BudgetMutationReturnsBeforeGate(t *testing.T) {
	var published []githubclient.CommitStatus
	server := runPRGateMockServerMulti(t, simpleDiffAUR537, "gate:\n  inconclusive: block\n", &published, nil)
	defer server.Close()
	tinyBudgetEnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true,
		publication:    "review",
		limiteSet:      true,
		limite:         "0.0000000001",
	})
	if _, ok := statusByContext(published, policyGateContext); code == 1 && !ok {
		t.Fatal("MUT-001 (budget): the function returned before the gate was ever reached despite a declared gate")
	}
	if _, ok := statusByContext(published, policyGateContext); !ok {
		t.Fatalf("published statuses = %+v, want %q present", published, policyGateContext)
	}
}
