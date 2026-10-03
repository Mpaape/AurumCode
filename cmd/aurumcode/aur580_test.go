package main

// Deliberation through the real command: the model asks for an optional
// scanner when the change is large and not when it is small; the scan's
// findings count in the gate with their origin; exceeding a limit is
// inconclusive with nothing published; a required scanner is never offered;
// a missing binary is the scan's inconclusive reason; the transcript is in
// the audit with redacted arguments.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const aur580Approve = `{"verdict":"approve","strengths":[],"issues":[],"suggestions":[%s],"ci_analysis":[],"test_plan":[],"limitations":[],"summary":"parecer"}`

// aur580Fixture writes a tool-calling fixture: toolCalls is the JSON list
// of tool cases; the answer cites the scan when its result is in the
// conversation.
func aur580Fixture(t *testing.T, toolCalls string) string {
	t.Helper()
	cited := strings.Replace(aur580Approve, "%s", `{"title":"Evidencia do scanner","description":"o scanner fakescan confirmou fake leak em app.go:4"}`, 1)
	plain := strings.Replace(aur580Approve, "%s", `{"title":"Sem varredura","description":"sem varredura opcional"}`, 1)
	body := `{"aurumcode_fixture":{"tool_calls":` + toolCalls + `,"cases":[{"prompt_contains":"fakescan: 1 achado","response":` + cited + `}],"default":` + plain + `}}`
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// aur580Setup builds the repository, registers fakescan (one error finding
// on app.go:4, or a missing binary) and writes the configuration.
func aur580Setup(t *testing.T, config string, binary string) *int {
	t.Helper()
	aur579Repo(t)
	invoked := 0
	registerFake(t, scanner.Engine{Scanner: fakeEngine{name: "fakescan", invoked: &invoked, binary: binary, report: scanner.Report{Complete: true, Findings: []scanner.Finding{
		{Path: "app.go", Line: 4, Side: "RIGHT", RuleID: "fakescan:leak", Severity: "error", Message: "fake leak"},
	}}}})
	writeRepoConfig(t, config)
	return &invoked
}

const aur580Config = "deliberation:\n  enabled: true\n  max_rounds: 3\nquality_gates:\n  scanners:\n    - engine: fakescan\n"

type aur580Audit struct {
	Blocking []struct {
		RuleID string `json:"rule_id"`
		Origin string `json:"origin"`
	} `json:"blocking_findings"`
	Deliberation *struct {
		Offered      []string `json:"offered"`
		Requested    []string `json:"requested"`
		NotRequested []string `json:"not_requested"`
		Calls        []struct {
			Tool      string `json:"tool"`
			Arguments string `json:"arguments"`
			Status    string `json:"status"`
		} `json:"calls"`
	} `json:"deliberation"`
}

func aur580Run(t *testing.T, args ...string) (int, string, string, aur580Audit) {
	t.Helper()
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, out, errOut := aur579Review(t, nil, append([]string{"--auditoria", audit}, args...)...)
	var rec aur580Audit
	if data, err := os.ReadFile(audit); err == nil {
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatalf("audit is not JSON: %v\n%s", err, data)
		}
	}
	return code, out, errOut, rec
}

// AC-001: a large diff makes the fixture ask for the scanner; the scan runs
// once, its result goes back to the model, the answer cites it, and the
// finding fails the scanner gate with its origin.
func TestAUR580ModelAsksForTheScannerOnALargeDiff(t *testing.T) {
	invoked := aur580Setup(t, aur580Config, "")
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"lines_above":1,"tool":"scanner_fakescan"}]`))
	code, out, errOut, rec := aur580Run(t)
	if code != exitFindings || *invoked != 1 {
		t.Fatalf("exit=%d invoked=%d, want the requested scan to run once and fail the gate; stderr=%s", code, *invoked, errOut)
	}
	if !strings.Contains(out, "o scanner fakescan confirmou fake leak") {
		t.Errorf("the answer does not cite the tool result:\n%s", out)
	}
	if !strings.Contains(errOut, "pedidas [scanner_fakescan]") {
		t.Errorf("stderr lacks the decision:\n%s", errOut)
	}
	if len(rec.Blocking) != 1 || rec.Blocking[0].Origin != "fakescan" {
		t.Errorf("blocking findings = %+v, want fakescan:leak with origin fakescan", rec.Blocking)
	}
	if rec.Deliberation == nil || len(rec.Deliberation.Requested) != 1 || rec.Deliberation.Calls[0].Status != "executed" {
		t.Fatalf("audit deliberation = %+v", rec.Deliberation)
	}
	sent, _ := os.ReadFile(capture)
	for _, want := range []string{"## Available tools", "`scanner_fakescan` (custo:", "`codebase_context` (custo:"} {
		if !strings.Contains(string(sent), want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
}

// AC-001: a small diff: the fixture does not ask, the scanner never runs,
// and the audit records that it was offered and not requested.
func TestAUR580ModelDoesNotAskOnASmallDiff(t *testing.T) {
	invoked := aur580Setup(t, aur580Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"lines_above":50,"tool":"scanner_fakescan"}]`))
	code, _, errOut, rec := aur580Run(t)
	if code != 0 || *invoked != 0 {
		t.Fatalf("exit=%d invoked=%d, want no scan and a clean gate; stderr=%s", code, *invoked, errOut)
	}
	if rec.Deliberation == nil || len(rec.Deliberation.Requested) != 0 || !contains(rec.Deliberation.NotRequested, "scanner_fakescan") {
		t.Fatalf("audit deliberation = %+v, want scanner_fakescan not requested", rec.Deliberation)
	}
}

// AC-002: a model that keeps asking exceeds max_rounds: exit 1, the motive
// is named, nothing of its answer is published and no audit is written.
func TestAUR580RoundsExceededIsInconclusiveAndUnpublished(t *testing.T) {
	aur580Setup(t, strings.Replace(aur580Config, "max_rounds: 3", "max_rounds: 2", 1), "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"tool":"codebase_context","arguments":{"path":"app.go"},"every_round":true}]`))
	code, out, errOut, rec := aur580Run(t)
	if code == 0 || !strings.Contains(errOut, "inconclusivo (deliberation_limit:max_rounds)") {
		t.Fatalf("exit=%d, want non-zero with the deliberation_limit motive; stderr=%s", code, errOut)
	}
	if strings.Contains(out, "sem varredura opcional") || strings.Contains(out, "Verdict") || rec.Deliberation != nil {
		t.Fatalf("a deliberation over its rounds published a verdict:\nstdout=%s", out)
	}
}

// AC-002: the token ceiling is a limit too.
func TestAUR580CostExceededIsInconclusive(t *testing.T) {
	aur580Setup(t, strings.Replace(aur580Config, "max_rounds: 3", "max_rounds: 3\n  max_cost_tokens: 10", 1), "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"lines_above":1,"tool":"scanner_fakescan"}]`))
	code, out, errOut, _ := aur580Run(t)
	if code == 0 || !strings.Contains(errOut, "deliberation_limit:max_cost_tokens") || strings.Contains(out, "Verdict") {
		t.Fatalf("exit=%d, want the cost limit to stop the review unpublished; stderr=%s", code, errOut)
	}
}

// AC-004: a required scanner runs before the model and is never offered.
func TestAUR580RequiredScannerIsNeverOptional(t *testing.T) {
	invoked := aur580Setup(t, aur580Config+"      required: true\n", "")
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"lines_above":1,"tool":"scanner_fakescan"}]`))
	code, _, errOut, rec := aur580Run(t)
	if code != exitFindings || *invoked != 1 {
		t.Fatalf("exit=%d invoked=%d, want the required scan to run once before the model; stderr=%s", code, *invoked, errOut)
	}
	sent, _ := os.ReadFile(capture)
	if strings.Contains(string(sent), "scanner_fakescan") || contains(rec.Deliberation.Offered, "scanner_fakescan") {
		t.Fatalf("a required scanner was offered as optional")
	}
}

// AC-004/AC-005: invalid arguments are refused before running; the audit
// records the refusal with the arguments redacted.
func TestAUR580InvalidArgumentsRefusedAndRedactedInTheAudit(t *testing.T) {
	aur580Setup(t, aur580Config, "")
	t.Setenv("AURUM_SECRET_CANARY", "canary-token-580-xyz")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"tool":"codebase_context","arguments":{"path":5}},{"tool":"codebase_context","arguments":{"path":"canary-token-580-xyz"}}]`))
	code, _, errOut, rec := aur580Run(t)
	if code != 0 {
		t.Fatalf("exit=%d; stderr=%s", code, errOut)
	}
	if rec.Deliberation == nil || len(rec.Deliberation.Calls) != 2 {
		t.Fatalf("audit deliberation = %+v", rec.Deliberation)
	}
	if c := rec.Deliberation.Calls[0]; c.Status != "refused" {
		t.Errorf("a call with a numeric path was not refused: %+v", c)
	}
	if c := rec.Deliberation.Calls[1]; c.Status != "failed" || strings.Contains(c.Arguments, "canary-token-580-xyz") {
		t.Errorf("a path outside the diff must fail and its argument be redacted: %+v", c)
	}
}

// A requested scanner whose binary is missing follows the one rule: the
// scan is inconclusive and the gate fails under block.
func TestAUR580MissingBinaryIsInconclusive(t *testing.T) {
	aur580Setup(t, aur580Config+"gate:\n  fail_on: [error]\n  inconclusive: block\n  sources: [fakescan]\n", "aurumcode-absent-scanner-580")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"lines_above":1,"tool":"scanner_fakescan"}]`))
	code, _, errOut, _ := aur580Run(t)
	if code != 1 || !strings.Contains(errOut, "fakescan_unavailable") {
		t.Fatalf("exit=%d, want the missing binary inconclusive under block; stderr=%s", code, errOut)
	}
}

// Without a tool-capable provider the deferred scanners run as before.
func TestAUR580DeferredScannerRunsWhenToolsCannotBeOffered(t *testing.T) {
	invoked := aur580Setup(t, aur580Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, "[]"))
	code, _, errOut, rec := aur580Run(t)
	if code != exitFindings || *invoked != 1 || rec.Deliberation != nil {
		t.Fatalf("exit=%d invoked=%d deliberation=%v, want the scanner run as before; stderr=%s", code, *invoked, rec.Deliberation, errOut)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
