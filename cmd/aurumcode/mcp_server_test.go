package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// mcpCanary is an AWS-shaped canary the security pass flags and the
// redaction filter recognizes.
const mcpCanary = "AKIA" + "2Z3Y4X5W6V7U8T9S"

// mcpRepo is a repository whose change both violates a repository skill
// (cited by the model fixture) and adds a secret (found by the security
// pass), under a gate that fails on error.
func mcpRepo(t *testing.T) string {
	t.Helper()
	files := map[string][]byte{".aurumcode/skills/tamanho/SKILL.md": []byte(tamanhoSkill), "app.go": []byte("package demo\n")}
	head := map[string][]byte{".aurumcode/skills/tamanho/SKILL.md": []byte(tamanhoSkill), "app.go": []byte("package demo\n\nfunc Longa() {}\n\nfunc Conecta() string {\n\tdbPassword := \"" + mcpCanary + "\"\n\treturn dbPassword\n}\n")}
	dir := aur522Repo(t, files, head, "gate:\n  fail_on: [error]\n")
	aur522Fixture(t, `{"summary":"x","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"`+tamanhoRule+`","message":"Funcao Longa tem 160 linhas.","impact":"Funcao longa demais para revisar.","evidence":"As linhas adicionadas declaram Longa em app.go.","suggestion":"Divida em etapas nomeadas.","verification":"Conte as linhas da funcao."}]}`)
	return dir
}

// mcpSession runs `aurumcode mcp` in process over the given requests and
// returns every response, failing on any stdout line that is not JSON-RPC.
func mcpSession(t *testing.T, requests ...string) []map[string]any {
	t.Helper()
	in := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"teste","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + strings.Join(requests, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	if code := runMCP(nil, strings.NewReader(in), &stdout, &stderr, redaction.NewFilter()); code != 0 {
		t.Fatalf("mcp exit=%d stderr=%s", code, stderr.String())
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil || m["jsonrpc"] != "2.0" {
			t.Fatalf("stdout carries a line that is not JSON-RPC: %q", line)
		}
		out = append(out, m)
	}
	if len(out) != len(requests)+1 {
		t.Fatalf("got %d responses for %d requests", len(out), len(requests)+1)
	}
	return out[1:]
}

func mcpCall(id int, tool, args string) string {
	return `{"jsonrpc":"2.0","id":` + itoaTest(id) + `,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`
}

func itoaTest(i int) string { b, _ := json.Marshal(i); return string(b) }

func mcpAnswer(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", m)
	}
	return res["structuredContent"].(map[string]any)
}

// freshCache gives the next run its own review cache, so a verdict stored by
// an earlier run of the same test is never reused across the comparison.
func freshCache(t *testing.T) {
	t.Helper()
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
}

// AC-001: the agent's gate is the review session's gate: same exit, same
// report, same blocking findings, same decision as `review --base` on the
// same diff.
func TestMCPGateIsTheReviewSessionGate(t *testing.T) {
	mcpRepo(t)
	freshCache(t)
	audit := filepath.Join(t.TempDir(), "audit.json")
	var cliOut, cliErr bytes.Buffer
	cliExit := runReview([]string{"--base", "HEAD~1", "--seguranca", "--exigir-qualidade", "--auditoria", audit}, &cliOut, &cliErr, redaction.NewFilter())
	if cliExit != exitFindings {
		t.Fatalf("fixture review exit=%d, want %d\n%s%s", cliExit, exitFindings, cliOut.String(), cliErr.String())
	}
	freshCache(t)
	msgs := mcpSession(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, mcpCall(2, "aurum_review", `{"base":"HEAD~1"}`))
	freshCache(t)
	gate := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_gate", `{"base":"HEAD~1"}`))[0])
	tools := msgs[0]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 4 {
		t.Fatalf("tools/list lists %d tools, want 4", len(tools))
	}
	review := mcpAnswer(t, msgs[1])
	for name, a := range map[string]map[string]any{"aurum_gate": gate, "aurum_review": review} {
		if a["decision"] != "fail" || a["exit_code"] != float64(cliExit) {
			t.Fatalf("%s decision=%v exit=%v, review --base exit=%d", name, a["decision"], a["exit_code"], cliExit)
		}
	}
	// The answer passes the redaction filter as a whole; the CLI's stdout
	// is redacted field by field, so the same filter is applied to it.
	if want := redaction.NewFilter().Redact(cliOut.String()); review["report"] != want {
		t.Fatalf("aurum_review report differs from review --base stdout:\n--- mcp\n%v\n--- cli\n%s", review["report"], want)
	}
	var record struct {
		BlockingFindings []struct {
			RuleID string `json:"rule_id"`
			Path   string `json:"path"`
			Line   int    `json:"line"`
		} `json:"blocking_findings"`
	}
	raw, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	var want, got []string
	for _, b := range record.BlockingFindings {
		want = append(want, fmt.Sprintf("%s@%s:%d", b.RuleID, b.Path, b.Line))
	}
	for _, b := range gate["blocking_findings"].([]any) {
		bm := b.(map[string]any)
		got = append(got, fmt.Sprintf("%s@%s:%v", bm["rule_id"], bm["file"], bm["line"]))
	}
	if len(want) < 2 || !sameSet(want, got) {
		t.Fatalf("blocking findings differ: mcp %v, review --base audit %v", got, want)
	}
}

func sameSet(a, b []string) bool {
	count := map[string]int{}
	for _, s := range a {
		count[s]++
	}
	for _, s := range b {
		count[s]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return len(a) == len(b)
}

// AC-003: no answer carries the secret of the diff; an invalid argument is
// refused before a review runs; a provider that fails is inconclusive.
func TestMCPAnswersAreRedactedAndFailClosed(t *testing.T) {
	mcpRepo(t)
	freshCache(t)
	msgs := mcpSession(t, mcpCall(1, "aurum_review", `{"base":"HEAD~1"}`), mcpCall(2, "aurum_gate", `{"base":"HEAD~1"}`), mcpCall(3, "aurum_gate", `{"base":"--politica=/tmp"}`), mcpCall(4, "aurum_gate", `{"base":"HEAD~1","fail_on":"none"}`))
	sawSecretFinding := false
	for i, m := range msgs[:2] {
		raw, _ := json.Marshal(m)
		if strings.Contains(string(raw), mcpCanary) {
			t.Fatalf("answer %d leaks the canary of the diff (%d bytes)", i+1, len(raw))
		}
		for _, f := range mcpAnswer(t, m)["findings"].([]any) {
			if rule := f.(map[string]any)["rule_id"]; rule == "security/hardcoded-secret" || rule == "analysis/hardcoded-secret" {
				sawSecretFinding = true
			}
		}
	}
	if !sawSecretFinding {
		raw, _ := json.Marshal(msgs[0])
		t.Fatalf("the security pass did not report the secret: the canary proves nothing: %s", raw)
	}
	for i, m := range msgs[2:] {
		if e, ok := m["error"].(map[string]any); !ok || e["code"] != float64(-32602) {
			t.Fatalf("invalid call %d was not refused: %v", i+3, m)
		}
	}

	t.Setenv("AURUMCODE_LLM_FIXTURE", filepath.Join(t.TempDir(), "missing.json"))
	failed := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_gate", `{"base":"HEAD~1"}`))[0])
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	none := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_gate", `{"base":"HEAD~1"}`))[0])
	for name, a := range map[string]map[string]any{"provider failed": failed, "no provider": none} {
		if a["decision"] != "inconclusive" {
			t.Errorf("%s: decision %v, want inconclusive (%v)", name, a["decision"], a["reason"])
		}
	}
}

// AC-002: under a central policy, aurum_rules lists the policy's skills and
// the repository's; no tool declares a parameter that could switch a rule
// off or name another policy.
func TestMCPRulesUnderCentralPolicy(t *testing.T) {
	mcpRepo(t)
	policy := t.TempDir()
	polSkill := "---\nname: politica-org\npaths: [\"**\"]\n---\nRegra da organizacao.\n\n## ORG-001 Sem TODO\nseverity: error\nNada de TODO.\n"
	for name, data := range map[string]string{".aurumcode/config.yml": "gate:\n  fail_on: [error]\n", ".aurumcode/skills/politica-org/SKILL.md": polSkill} {
		p := filepath.Join(policy, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AURUMCODE_POLICY", policy)
	msgs := mcpSession(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, mcpCall(2, "aurum_rules", `{"paths":["app.go"]}`))
	layers := map[string]bool{}
	rules := mcpAnswer(t, msgs[1])
	for _, s := range rules["skills"].([]any) {
		layers[s.(map[string]any)["layer"].(string)] = true
	}
	if !layers["policy"] || !layers["repository"] {
		t.Fatalf("aurum_rules does not list both layers: %v", rules)
	}
	allowed := map[string][]string{"aurum_review": {"base"}, "aurum_gate": {"base"}, "aurum_rules": {"paths"}, "aurum_explain": {"finding_id"}}
	for _, tool := range msgs[0]["result"].(map[string]any)["tools"].([]any) {
		tm := tool.(map[string]any)
		schema := tm["inputSchema"].(map[string]any)
		var props []string
		for k := range schema["properties"].(map[string]any) {
			props = append(props, k)
		}
		if !reflect.DeepEqual(props, allowed[tm["name"].(string)]) || schema["additionalProperties"] != false {
			t.Errorf("%s declares %v (additionalProperties=%v)", tm["name"], props, schema["additionalProperties"])
		}
	}
}

// An agent that asks before committing compares HEAD with itself: an empty
// change is never a pass.
func TestMCPEmptyChangeIsNeverAPass(t *testing.T) {
	mcpRepo(t)
	freshCache(t)
	a := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_gate", `{"base":"HEAD"}`))[0])
	if a["decision"] != "inconclusive" || a["reason"] != "empty_change" {
		t.Fatalf("an empty change was answered %v (%v), want inconclusive (empty_change)", a["decision"], a["reason"])
	}
}
