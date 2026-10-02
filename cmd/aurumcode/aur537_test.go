package main

// AUR-537 end-to-end behavior proof: a provider/transport failure on --pr
// (every configured provider failed the call, llm.ErrAllProvidersFailed)
// now routes through the AUR-519 policy gate instead of returning before it
// is ever reached -- driven through the real runPRReview with the same
// httptest GitHub mock aur519_e2e_test.go's --pr table already uses
// (runPRGateMockServer/setPRGateEnv), but with no AURUMCODE_LLM_FIXTURE and
// a dead LLM_BASE_URL instead, so the model is actually called and actually
// fails, rather than failing earlier at provider selection.

import (
	"encoding/json"
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

// TestAUR537ProviderFailureBlocksGate covers AC-001: with gate.inconclusive:
// block, a provider/transport failure during the actual model call (not a
// selection failure) publishes aurumcode/policy-gate as "failure" naming
// provider_failure, exits exitQualityNotReviewed, writes an audit record
// whose gate.decision names the failure, writes a SARIF document with
// executionSuccessful=false, and the published review body says the review
// did not run -- never approved.
func TestAUR537ProviderFailureBlocksGate(t *testing.T) {
	var published githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServer(t, simpleDiffAUR537, "gate:\n  inconclusive: block\n", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
	}
	if published.Context != policyGateContext || published.State != "failure" {
		t.Fatalf("published status = %+v, want context %q state failure", published, policyGateContext)
	}
	if !strings.Contains(published.Description, gateReasonProviderFailure) {
		t.Fatalf("published status description = %q, want it naming %q", published.Description, gateReasonProviderFailure)
	}
	if !strings.Contains(postedBody, "the review did not run") {
		t.Fatalf("published review body does not say the review did not run: %s", postedBody)
	}
	if strings.Contains(postedBody, "**Verdict:** Approve") {
		t.Fatalf("published review body verdict read Approve under a blocking, inconclusive gate: %s", postedBody)
	}
	if strings.Contains(stdout.String(), `"APPROVE"`) {
		t.Fatalf("published GitHub review action read APPROVE under a blocking, inconclusive gate: %s", stdout.String())
	}

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit aur537Audit
	if err := json.Unmarshal(rawAudit, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, rawAudit)
	}
	if audit.Verdict == "approve" {
		t.Fatal("audit verdict must never be approve for a run that did not review")
	}
	if !strings.Contains(audit.Gate.Reason, gateReasonProviderFailure) {
		t.Fatalf("audit gate.reason=%q, want it naming %q", audit.Gate.Reason, gateReasonProviderFailure)
	}

	rawSARIF, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	var doc aur537SARIF
	if err := json.Unmarshal(rawSARIF, &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, rawSARIF)
	}
	if len(doc.Runs) != 1 || len(doc.Runs[0].Invocations) != 1 {
		t.Fatalf("SARIF runs/invocations shape = %+v", doc.Runs)
	}
	if doc.Runs[0].Invocations[0].ExecutionSuccessful {
		t.Fatal("SARIF executionSuccessful must be false for a run that did not review")
	}
}

// TestAUR537ProviderFailureWarnsGate covers AC-002: with gate.inconclusive:
// warn, the identical provider/transport failure publishes
// aurumcode/policy-gate as "success" carrying a visible inconclusive alert
// -- never the "aprovado" word -- and exits 0. The audit record for the
// same run must read "inconclusive", in agreement with AC-001's own run
// reading "fail" only because that run's mode was "block".
func TestAUR537ProviderFailureWarnsGate(t *testing.T) {
	var published githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServer(t, simpleDiffAUR537, "gate:\n  inconclusive: warn\n", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if published.Context != policyGateContext || published.State != "success" {
		t.Fatalf("published status = %+v, want context %q state success", published, policyGateContext)
	}
	if strings.Contains(published.Description, gateStatusWordApproved) {
		t.Fatalf("published status description = %q, must never read %q for a run that did not review", published.Description, gateStatusWordApproved)
	}
	if !strings.Contains(published.Description, gateStatusWordInconclusive) {
		t.Fatalf("published status description = %q, want the inconclusive word", published.Description)
	}
	if !strings.Contains(postedBody, "the review did not run") {
		t.Fatalf("published review body does not say the review did not run: %s", postedBody)
	}

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit aur537Audit
	if err := json.Unmarshal(rawAudit, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, rawAudit)
	}
	if audit.Gate.Decision != "inconclusive" {
		t.Fatalf("audit gate.decision=%q, want inconclusive", audit.Gate.Decision)
	}

	rawSARIF, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	var doc aur537SARIF
	if err := json.Unmarshal(rawSARIF, &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, rawSARIF)
	}
	if len(doc.Runs) != 1 || len(doc.Runs[0].Invocations) != 1 || doc.Runs[0].Invocations[0].ExecutionSuccessful {
		t.Fatalf("SARIF executionSuccessful must be false for a run that did not review: %+v", doc.Runs)
	}
}

// TestAUR537NoGateProviderFailureUnchanged covers AC-003: with no `gate:`
// key declared anywhere, the identical provider/transport failure must stay
// byte-identical to the command's behavior before this card -- exit 1, the
// same "all providers failed" diagnosis, no aurumcode/policy-gate status
// published (the mock server fails the test on any unexpected request, so
// a status POST that should not happen is caught), and no audit/SARIF file
// written even though both paths were given.
func TestAUR537NoGateProviderFailureUnchanged(t *testing.T) {
	var published githubclient.CommitStatus
	var postedBody string
	server := runPRGateMockServer(t, simpleDiffAUR537, "", &published, &postedBody)
	defer server.Close()
	deadProviderEnv(t, server)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
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
	if published.Context != "" {
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
	var published githubclient.CommitStatus
	server := runPRGateMockServer(t, simpleDiffAUR537, "gate:\n  inconclusive: block\n", &published, nil)
	defer server.Close()
	deadProviderEnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	if code == 1 && published.Context == "" {
		t.Fatal("MUT-001: the function returned before the gate was ever reached despite a declared gate")
	}
	if published.Context != policyGateContext {
		t.Fatalf("published status context = %q, want %q", published.Context, policyGateContext)
	}
}
