package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR519EvaluateGateNoGateDeclared(t *testing.T) {
	d, err := evaluateGate(config.GateConfig{}, gateOriginPolicy, nil, nil, "")
	if err != nil {
		t.Fatalf("evaluateGate() error = %v", err)
	}
	if d.Active || d.Fail {
		t.Fatalf("evaluateGate() with no gate declared = %+v, want inactive and passing", d)
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
	d, err := evaluateGate(gate, gateOriginPolicy, dynamic, issues, "")
	if err != nil {
		t.Fatalf("evaluateGate() error = %v", err)
	}
	if !d.Active || !d.Fail {
		t.Fatalf("evaluateGate() = %+v, want active and failing", d)
	}
	if len(d.Lines) != 1 || !containsAll(d.Lines[0], "security#no-hardcoded-secrets", "No Hardcoded Secrets") {
		t.Fatalf("evaluateGate() lines = %v, want the skill/section named (AC-001)", d.Lines)
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
	d, err := evaluateGate(gate, gateOriginPolicy, dynamic, issues, "")
	if err != nil {
		t.Fatalf("evaluateGate() error = %v", err)
	}
	if d.Fail {
		t.Fatalf("evaluateGate() = %+v, want passing: a repo-origin finding must never fail a policy's gate", d)
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
	d, err := evaluateGate(block, gateOriginPolicy, nil, nil, "provider_failure")
	if err != nil {
		t.Fatalf("evaluateGate() error = %v", err)
	}
	if !d.Fail || !d.Inconclusive {
		t.Fatalf("evaluateGate() with inconclusive:block = %+v, want failing and inconclusive", d)
	}

	warn := config.GateConfig{Inconclusive: "warn"}
	d, err = evaluateGate(warn, gateOriginPolicy, nil, nil, "provider_failure")
	if err != nil {
		t.Fatalf("evaluateGate() error = %v", err)
	}
	if d.Fail {
		t.Fatalf("evaluateGate() with inconclusive:warn = %+v, want passing", d)
	}
	if !d.Inconclusive || len(d.Lines) == 0 {
		t.Fatalf("evaluateGate() with inconclusive:warn = %+v, want a visible alert", d)
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

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
