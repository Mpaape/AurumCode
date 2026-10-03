package main

import (
	"context"
	"encoding/json"
	igate "github.com/Mpaape/AurumCode/internal/gate"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR519EvaluateGateNoGateDeclared(t *testing.T) {
	d, err := igate.EvaluateGate(config.GateConfig{}, gateOriginPolicy, nil, nil, "", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if d.Active || d.Fail {
		t.Fatalf("igate.EvaluateGate() with no gate declared = %+v, want inactive and passing", d)
	}
}

// TestAUR519EvaluateGateSeverityBreach proves AC-001: a finding at or
// above the policy's fail_on threshold, citing a policy skill section,
// fails the gate and names the skill and section.
func TestAUR519EvaluateGateSeverityBreach(t *testing.T) {
	gate := config.GateConfig{FailOn: []string{"high"}}
	dynamic := map[string]review.Rule{
		"security#no-hardcoded-secrets": {ID: "security#no-hardcoded-secrets", Title: "No Hardcoded Secrets", Origin: gateOriginPolicy},
	}
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "error", RuleID: "security#no-hardcoded-secrets", Message: "leak"},
	}
	d, err := igate.EvaluateGate(gate, gateOriginPolicy, dynamic, issues, "", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if !d.Active || !d.Fail {
		t.Fatalf("igate.EvaluateGate() = %+v, want active and failing", d)
	}
	if len(d.Lines) != 1 || !containsAll(d.Lines[0], "security#no-hardcoded-secrets", "No Hardcoded Secrets") {
		t.Fatalf("igate.EvaluateGate() lines = %v, want the skill/section named (AC-001)", d.Lines)
	}
}

// TestAUR519EvaluateGateRepoOriginNeverFails proves AC-005: a finding
// citing a REPO skill section never fails the gate when a central policy
// is active (acceptedOrigin == policy), even above the threshold.
func TestAUR519EvaluateGateRepoOriginNeverFails(t *testing.T) {
	gate := config.GateConfig{FailOn: []string{"high"}}
	dynamic := map[string]review.Rule{
		"convencao#estilo": {ID: "convencao#estilo", Title: "Estilo do repositório", Origin: gateOriginRepo},
	}
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "error", RuleID: "convencao#estilo", Message: "estilo"},
	}
	d, err := igate.EvaluateGate(gate, gateOriginPolicy, dynamic, issues, "", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if d.Fail {
		t.Fatalf("igate.EvaluateGate() = %+v, want passing: a repo-origin finding must never fail a policy's gate", d)
	}
}

// TestAUR519EvaluateGateInconclusiveBlockAndWarn proves AC-003/AC-004/
// AC-008: with inconclusive: block an inconclusive run fails the gate;
// with inconclusive: warn it passes but still reports the alert. This is
// also MUT-001's target: a mutation that treats a provider failure as "no
// findings" must still be caught here, by the caller passing a non-empty
// inconclusiveReason regardless of how many issues ended up in the slice.
func TestAUR519EvaluateGateInconclusiveBlockAndWarn(t *testing.T) {
	block := config.GateConfig{Inconclusive: "block"}
	d, err := igate.EvaluateGate(block, gateOriginPolicy, nil, nil, "provider_failure", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if !d.Fail || !d.Inconclusive {
		t.Fatalf("igate.EvaluateGate() with inconclusive:block = %+v, want failing and inconclusive", d)
	}

	warn := config.GateConfig{Inconclusive: "warn"}
	d, err = igate.EvaluateGate(warn, gateOriginPolicy, nil, nil, "provider_failure", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if d.Fail {
		t.Fatalf("igate.EvaluateGate() with inconclusive:warn = %+v, want passing", d)
	}
	if !d.Inconclusive || len(d.Lines) == 0 {
		t.Fatalf("igate.EvaluateGate() with inconclusive:warn = %+v, want a visible alert", d)
	}
}

func TestAUR519MergedRuleCatalogIDs(t *testing.T) {
	builtin := []string{"security/sql-injection", "quality/dead-code"}
	dynamic := map[string]review.Rule{"security#no-hardcoded-secrets": {ID: "security#no-hardcoded-secrets"}}
	ids := mergedRuleCatalogIDs(builtin, dynamic)
	want := []string{"quality/dead-code", "security#no-hardcoded-secrets", "security/sql-injection"}
	if len(ids) != len(want) {
		t.Fatalf("mergedRuleCatalogIDs() = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("mergedRuleCatalogIDs() = %v, want %v", ids, want)
		}
	}
}

// TestAUR519EvaluateGateWarnStillFailsOnBreach is B1's regression: an
// inconclusive run (e.g. AUR-476 partial coverage from an ignored/oversized
// file on the same PR) under gate.inconclusive: warn must NOT skip the
// severity threshold loop. Before this fix, evaluateGate returned as soon
// as inconclusiveReason was non-empty, regardless of mode, so a policy
// breach riding alongside a merely-partial review silently passed (exit
// 0) instead of failing. It must still Fail (and Breach must be true, so
// the caller returns exitFindings, not exitQualityNotReviewed).
func TestAUR519EvaluateGateWarnStillFailsOnBreach(t *testing.T) {
	gate := config.GateConfig{FailOn: []string{"high"}, Inconclusive: "warn"}
	dynamic := map[string]review.Rule{
		"security#no-hardcoded-secrets": {ID: "security#no-hardcoded-secrets", Title: "No Hardcoded Secrets", Origin: gateOriginPolicy, Severity: "error"},
	}
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "error", RuleID: "security#no-hardcoded-secrets", Message: "leak"},
	}
	d, err := igate.EvaluateGate(gate, gateOriginPolicy, dynamic, issues, "partial_coverage", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if !d.Fail || !d.Breach {
		t.Fatalf("igate.EvaluateGate() = %+v, want Fail and Breach: a real breach must close the gate even under inconclusive:warn", d)
	}
	if !d.Inconclusive {
		t.Fatalf("igate.EvaluateGate() = %+v, want Inconclusive still reported alongside the breach", d)
	}
}

// TestAUR519EvaluateGateRuleSeverityFloorsModel is B4's regression: the
// model's own issue.Severity is untrusted (the reviewed diff could contain
// a prompt-injection line asking the model to under-report), so the cited
// dynamic rule's own declared severity must floor the comparison. Here the
// model reports "info" for a rule whose skill section declared
// "severity: error" -- the breach must still fire at a "high"/error
// threshold.
func TestAUR519EvaluateGateRuleSeverityFloorsModel(t *testing.T) {
	gate := config.GateConfig{FailOn: []string{"high"}}
	dynamic := map[string]review.Rule{
		"security#no-hardcoded-secrets": {ID: "security#no-hardcoded-secrets", Title: "No Hardcoded Secrets", Origin: gateOriginPolicy, Severity: "error"},
	}
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 1, Severity: "info", RuleID: "security#no-hardcoded-secrets", Message: "leak, downgraded by the model"},
	}
	d, err := igate.EvaluateGate(gate, gateOriginPolicy, dynamic, issues, "", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if !d.Fail || !d.Breach {
		t.Fatalf("igate.EvaluateGate() = %+v, want Fail: the rule's own \"severity: error\" must floor the model's downgraded \"info\"", d)
	}
}

// TestAUR519EvaluateGateFailOnWithoutInconclusiveNeverApproves is B5's
// regression: gate.fail_on declared with NO gate.inconclusive key at all
// (the policy never opted into block/warn) must still never let an
// inconclusive run report success as if it were a clean approval -- it is
// Inconclusive (so the caller's verdict/status logic can say so) but not
// Fail (absent inconclusive stays warn-equivalent: visible, never
// blocking).
func TestAUR519EvaluateGateFailOnWithoutInconclusiveNeverApproves(t *testing.T) {
	gate := config.GateConfig{FailOn: []string{"high"}}
	d, err := igate.EvaluateGate(gate, gateOriginPolicy, nil, nil, "degraded_parse", nil, "", time.Now())
	if err != nil {
		t.Fatalf("igate.EvaluateGate() error = %v", err)
	}
	if d.Fail {
		t.Fatalf("igate.EvaluateGate() = %+v, want not Fail: absent gate.inconclusive stays warn-equivalent", d)
	}
	if !d.Inconclusive || len(d.Lines) == 0 {
		t.Fatalf("igate.EvaluateGate() = %+v, want Inconclusive with a visible reason", d)
	}

	// The published status must say "inconclusiva", never "aprovado".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head":
			var status githubclient.CommitStatus
			_ = json.NewDecoder(r.Body).Decode(&status)
			publishedStatusForTest = status
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := githubclient.NewClientWithBaseURL("test-token", server.URL)
	var out, errOut strings.Builder
	exit := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", d, 1)
	if exit != 0 {
		t.Fatalf("publishPolicyGateStatus() exit = %d, want 0 (inconclusive-without-block never blocks)", exit)
	}
	if strings.Contains(publishedStatusForTest.Description, "aprovado") {
		t.Fatalf("status description = %q, must never claim approval on an inconclusive run", publishedStatusForTest.Description)
	}
	if !strings.Contains(publishedStatusForTest.Description, "inconclusiva") || publishedStatusForTest.State != "success" {
		t.Fatalf("status = %+v, want state success and a description naming the review as inconclusive", publishedStatusForTest)
	}
}

// publishedStatusForTest is a tiny package-level scratch var the httptest
// handler above writes into -- simplest way to inspect the one status this
// test's own server receives without a second channel/mutex for a
// single-threaded test body.
var publishedStatusForTest githubclient.CommitStatus

// TestAUR519MergeDynamicRulesPolicyWins is CR-TRUST-001's precedence proof:
// on an id collision between the policy's and the repository's own skill
// sections, the policy's rule must always win -- a repository must never
// be able to shadow a policy's rule id with a lower-trust, repo-origin
// copy and quietly remove it from the gate's accepted set.
func TestAUR519MergeDynamicRulesPolicyWins(t *testing.T) {
	policy := map[string]review.Rule{"security#x": {ID: "security#x", Title: "Policy version", Origin: gateOriginPolicy, Severity: "error"}}
	repo := map[string]review.Rule{"security#x": {ID: "security#x", Title: "Repo version", Origin: gateOriginRepo, Severity: "info"}}
	merged := mergeDynamicRules(policy, repo)
	got, ok := merged["security#x"]
	if !ok || got.Origin != gateOriginPolicy || got.Title != "Policy version" {
		t.Fatalf("mergeDynamicRules() = %+v, want the policy's own rule to win the collision", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
