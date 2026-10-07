package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeGateway struct {
	calls    int
	rulesErr error
	outcome  SessionOutcome
	err      error
	delay    time.Duration
	rules    RuleSet
}

func (f *fakeGateway) Review(ctx context.Context, _ ReviewRequest) (SessionOutcome, error) {
	f.calls++
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
		}
	}
	return f.outcome, f.err
}

func (f *fakeGateway) Rules(context.Context, []string) (RuleSet, error) {
	f.calls++
	return f.rules, f.rulesErr
}

type canaryRedactor struct{ canary string }

func (r canaryRedactor) Redact(s string) string { return strings.ReplaceAll(s, r.canary, "[REDACTED]") }

// exchange sends lines and returns every response line, decoded.
func exchange(t *testing.T, s *Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var msgs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("stdout line is not JSON-RPC: %q", l)
		}
		if m["jsonrpc"] != "2.0" {
			t.Fatalf("not a JSON-RPC 2.0 message: %q", l)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func call(id int, tool, args string) string {
	return `{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func structured(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", m)
	}
	sc, ok := res["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("no structuredContent: %v", res)
	}
	return sc
}

func TestInitializeListAndNotifications(t *testing.T) {
	s := New(Options{Gateway: &fakeGateway{}, Version: "test"})
	msgs := exchange(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
	)
	if len(msgs) != 3 {
		t.Fatalf("a notification was answered or a request was not: %d responses", len(msgs))
	}
	init := msgs[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocol version not negotiated: %v", init["protocolVersion"])
	}
	tools := msgs[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		tm := tool.(map[string]any)
		names[tm["name"].(string)] = true
		schema := tm["inputSchema"].(map[string]any)
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Errorf("%s schema is not a closed object: %v", tm["name"], schema)
		}
		if tm["annotations"].(map[string]any)["readOnlyHint"] != true {
			t.Errorf("%s is not declared read only", tm["name"])
		}
	}
	for _, want := range []string{ToolReview, ToolGate, ToolRules, ToolExplain} {
		if !names[want] {
			t.Errorf("tools/list lacks %s", want)
		}
	}
}

func TestProtocolErrors(t *testing.T) {
	s := New(Options{Gateway: &fakeGateway{}})
	msgs := exchange(t, s,
		`not json`,
		`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"aurum_nope","arguments":{}}}`,
	)
	want := []float64{codeParseError, codeInvalidRequest, codeMethodNotFound, codeInvalidParams}
	if len(msgs) != len(want) {
		t.Fatalf("got %d responses", len(msgs))
	}
	for i, m := range msgs {
		e, ok := m["error"].(map[string]any)
		if !ok || e["code"] != want[i] {
			t.Errorf("message %d: want error %v, got %v", i, want[i], m)
		}
	}
}

func TestInvalidArgumentsRefusedBeforeRunning(t *testing.T) {
	gw := &fakeGateway{}
	s := New(Options{Gateway: gw})
	msgs := exchange(t, s,
		call(1, ToolGate, `{"base":"--output=/tmp/x"}`),
		call(2, ToolGate, `{"base":"main","politica":"/tmp/other"}`),
		call(3, ToolReview, `{"base":"main","disable_rules":["x"]}`),
		call(4, ToolGate, `{}`),
		call(5, ToolRules, `{"paths":["../etc/passwd"]}`),
		call(6, ToolRules, `{"paths":[]}`),
		call(7, ToolExplain, `{"finding_id":"zz"}`),
		call(8, ToolGate, `{"base":"main..evil"}`),
	)
	for i, m := range msgs {
		e, ok := m["error"].(map[string]any)
		if !ok || e["code"] != float64(codeInvalidParams) {
			t.Errorf("call %d was not refused as invalid params: %v", i+1, m)
		}
	}
	if gw.calls != 0 {
		t.Errorf("the gateway ran %d time(s) for invalid arguments", gw.calls)
	}
}

func TestDecisionNeverPassesWhenInconclusive(t *testing.T) {
	cases := []struct {
		name string
		o    SessionOutcome
		err  error
		want Decision
	}{
		{"clean", SessionOutcome{Exit: 0, ChangedFiles: 1}, nil, DecisionPass},
		{"empty change", SessionOutcome{Exit: 0}, nil, DecisionInconclusive},
		{"violation", SessionOutcome{Exit: 3}, nil, DecisionFail},
		{"blocked", SessionOutcome{Exit: 1, Blocking: []Finding{{File: "a.go", Line: 1}}}, nil, DecisionFail},
		{"provider", SessionOutcome{Exit: 1, InconclusiveReason: "provider_failure"}, nil, DecisionInconclusive},
		{"inconclusive exit 0", SessionOutcome{Exit: 0, ChangedFiles: 1, InconclusiveReason: "partial_coverage"}, nil, DecisionInconclusive},
		{"usage", SessionOutcome{Exit: 2}, nil, DecisionInconclusive},
		{"session error", SessionOutcome{}, errors.New("boom"), DecisionInconclusive},
	}
	for _, c := range cases {
		s := New(Options{Gateway: &fakeGateway{outcome: c.o, err: c.err}})
		msgs := exchange(t, s, call(1, ToolGate, `{"base":"main"}`))
		if got := structured(t, msgs[0])["decision"]; got != string(c.want) {
			t.Errorf("%s: decision %v, want %s", c.name, got, c.want)
		}
	}
}

func TestTimeoutIsInconclusive(t *testing.T) {
	s := New(Options{Gateway: &fakeGateway{outcome: SessionOutcome{Exit: 0}, delay: time.Second}, Timeout: 20 * time.Millisecond})
	msgs := exchange(t, s, call(1, ToolGate, `{"base":"main"}`))
	sc := structured(t, msgs[0])
	if sc["decision"] != string(DecisionInconclusive) || sc["reason"] != reasonTimeout {
		t.Errorf("timeout answered %v", sc)
	}
}

func TestEveryAnswerIsRedacted(t *testing.T) {
	canary := "AKIA" + "ABCDEFGHIJKLMNOP"
	o := SessionOutcome{
		Exit:        3,
		Findings:    []Finding{{File: "app.go", Line: 3, Severity: "error", RuleID: "security/hardcoded-secret", Origin: "security", Message: "key " + canary, Evidence: canary}},
		Report:      "report " + canary,
		Diagnostics: "stderr " + canary,
		GateLines:   []string{"gate " + canary},
	}
	s := New(Options{Gateway: &fakeGateway{outcome: o}, Redactor: canaryRedactor{canary}})
	msgs := exchange(t, s, call(1, ToolReview, `{"base":"main"}`), call(2, ToolGate, `{"base":"main"}`))
	id := structured(t, msgs[1])["findings"].([]any)[0].(map[string]any)["id"].(string)
	msgs = append(msgs, exchange(t, s, call(3, ToolExplain, `{"finding_id":"`+id+`"}`))...)
	for i, m := range msgs {
		raw, _ := json.Marshal(m)
		if strings.Contains(string(raw), canary) {
			t.Errorf("answer %d leaks the canary (%d bytes)", i+1, len(raw))
		}
		if !strings.Contains(string(raw), "[REDACTED]") {
			t.Errorf("answer %d carries no redaction marker", i+1)
		}
	}
}

// A protocol error passes the same redaction as an answer: a skill or
// configuration error that quotes a secret never reaches stdout raw.
func TestProtocolErrorsAreRedacted(t *testing.T) {
	canary := "AKIA" + "QRSTUVWXYZ234567"
	gw := &fakeGateway{rulesErr: errors.New("loading .aurumcode/skills/x/SKILL.md: line 3: token " + canary)}
	s := New(Options{Gateway: gw, Redactor: canaryRedactor{canary}})
	msgs := exchange(t, s, call(1, ToolRules, `{"paths":["app.go"]}`), call(2, ToolGate, `{"base":"main","x`+canary+`":1}`))
	for i, m := range msgs {
		e, ok := m["error"].(map[string]any)
		if !ok {
			t.Fatalf("call %d did not fail", i+1)
		}
		raw, _ := json.Marshal(m)
		if strings.Contains(string(raw), canary) || !strings.Contains(e["message"].(string), "[REDACTED]") {
			t.Errorf("error %d is not redacted (%d bytes)", i+1, len(raw))
		}
	}
}
