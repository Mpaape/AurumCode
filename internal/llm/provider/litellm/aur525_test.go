package litellm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// captureServer records the raw request body and answers with reply.
func captureServer(t *testing.T, reply string, got *[]byte, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if got != nil {
			*got = body
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
}

const toolCallReply = `{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
"tool_calls":[{"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"a/b.go\",\"start\":3}"}}]}}],
"usage":{"prompt_tokens":11,"completion_tokens":7}}`

var readTool = llm.ToolSpec{
	Name:        "file_read",
	Description: "read a file of the revision under review",
	Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
}

// AC-001: tools go out in the OpenAI format and tool_calls come back intact.
func TestAUR525RequestCarriesToolsAndReturnsCalls(t *testing.T) {
	var body []byte
	srv := captureServer(t, toolCallReply, &body, nil)
	defer srv.Close()

	p := NewProvider("k", srv.URL, "m")
	resp, err := p.CompleteWithTools([]llm.Message{{Role: llm.RoleUser, Content: "review"}}, []llm.ToolSpec{readTool}, llm.Options{})
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}

	var req struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if len(req.Tools) != 1 || req.Tools[0].Type != "function" || req.Tools[0].Function.Name != "file_read" ||
		req.Tools[0].Function.Description != readTool.Description {
		t.Fatalf("tools not sent in OpenAI format: %s", body)
	}
	if string(req.Tools[0].Function.Parameters) != string(readTool.Parameters) {
		t.Fatalf("parameters schema altered: %s", req.Tools[0].Function.Parameters)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(resp.ToolCalls))
	}
	c := resp.ToolCalls[0]
	if c.ID != "call_1" || c.Name != "file_read" || string(c.Arguments) != `{"path":"a/b.go","start":3}` {
		t.Fatalf("tool call altered: %+v (%s)", c, c.Arguments)
	}
	if resp.FinishReason != "tool_calls" || resp.TokensIn != 11 || resp.TokensOut != 7 || resp.Text != "" {
		t.Fatalf("response metadata wrong: %+v", resp.Response)
	}
}

// AC-002: a multi-message conversation keeps order, ids and null content.
func TestAUR525ConversationSerializesInOrderWithIDs(t *testing.T) {
	var body []byte
	srv := captureServer(t, `{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"done"}}]}`, &body, nil)
	defer srv.Close()

	conv := []llm.Message{
		{Role: llm.RoleUser, Content: "review this"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_9", Name: "file_read", Arguments: json.RawMessage(`{"path":"x.go"}`)}}},
		{Role: llm.RoleTool, ToolCallID: "call_9", Content: "package x"},
	}
	p := NewProvider("k", srv.URL, "m")
	resp, err := p.CompleteWithTools(conv, []llm.ToolSpec{readTool}, llm.Options{System: "sys"})
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}
	if resp.Text != "done" {
		t.Fatalf("text = %q", resp.Text)
	}

	var req struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	roles := []string{"system", "user", "assistant", "tool"}
	if len(req.Messages) != len(roles) {
		t.Fatalf("want %d messages, got %d: %s", len(roles), len(req.Messages), body)
	}
	for i, role := range roles {
		if string(req.Messages[i]["role"]) != `"`+role+`"` {
			t.Fatalf("message %d role = %s, want %s", i, req.Messages[i]["role"], role)
		}
	}
	if string(req.Messages[2]["content"]) != "null" {
		t.Fatalf("tool-only assistant turn must carry null content, got %s", req.Messages[2]["content"])
	}
	var calls []wireToolCall
	if err := json.Unmarshal(req.Messages[2]["tool_calls"], &calls); err != nil || len(calls) != 1 {
		t.Fatalf("assistant tool_calls missing: %s", req.Messages[2]["tool_calls"])
	}
	if calls[0].ID != "call_9" || calls[0].Function.Arguments != `{"path":"x.go"}` {
		t.Fatalf("assistant tool call altered: %+v", calls[0])
	}
	if string(req.Messages[3]["tool_call_id"]) != `"call_9"` {
		t.Fatalf("tool result must answer call_9, got %s", req.Messages[3]["tool_call_id"])
	}
	if string(req.Messages[3]["content"]) != `"package x"` {
		t.Fatalf("tool result content = %s", req.Messages[3]["content"])
	}
}

// AC-003: malformed arguments are a typed error, never a partial call.
func TestAUR525MalformedArgumentsIsTypedError(t *testing.T) {
	for name, args := range map[string]string{
		"truncated":  `{\"path\":`,
		"not-object": `[1,2]`,
		"null":       `null`,
	} {
		t.Run(name, func(t *testing.T) {
			reply := `{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
"tool_calls":[{"id":"call_2","type":"function","function":{"name":"code_search","arguments":"` + args + `"}}]}}]}`
			srv := captureServer(t, reply, nil, nil)
			defer srv.Close()

			resp, err := NewProvider("k", srv.URL, "m").CompleteWithTools(
				[]llm.Message{{Role: llm.RoleUser, Content: "x"}}, []llm.ToolSpec{readTool}, llm.Options{})
			if !errors.Is(err, llm.ErrMalformedToolArguments) {
				t.Fatalf("want ErrMalformedToolArguments, got %v", err)
			}
			var typed *llm.MalformedToolCallError
			if !errors.As(err, &typed) || typed.ID != "call_2" || typed.Name != "code_search" {
				t.Fatalf("error does not name the call: %v", err)
			}
			if len(resp.ToolCalls) != 0 {
				t.Fatalf("partial call returned: %+v", resp.ToolCalls)
			}
		})
	}
}

type textOnlyProvider struct{}

func (textOnlyProvider) Complete(string, llm.Options) (llm.Response, error) {
	return llm.Response{}, nil
}
func (textOnlyProvider) Tokens(string) (int, error) { return 0, nil }
func (textOnlyProvider) Name() string               { return "text-only" }

// AC-004: support is detectable before any request.
func TestAUR525ToolSupportDetectableBeforeCall(t *testing.T) {
	var calls int32
	srv := captureServer(t, toolCallReply, nil, &calls)
	defer srv.Close()

	if _, ok := llm.AsToolCaller(NewProvider("k", srv.URL, "m")); !ok {
		t.Fatal("litellm provider must report tool support")
	}
	if _, ok := llm.AsToolCaller(textOnlyProvider{}); ok {
		t.Fatal("a text-only provider must not report tool support")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("detection made %d request(s)", calls)
	}
}

// AC-005: Complete without tools sends exactly the request it sent before.
func TestAUR525CompleteRequestUnchanged(t *testing.T) {
	var body []byte
	srv := captureServer(t, `{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`, &body, nil)
	defer srv.Close()

	resp, err := NewProvider("k", srv.URL, "m").Complete("P", llm.Options{System: "S", MaxTokens: 5, JSONMode: true})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	const golden = `{"model":"m","messages":[{"role":"system","content":"S"},{"role":"user","content":"P"}],"max_tokens":5,"response_format":{"type":"json_object"}}`
	if string(body) != golden {
		t.Fatalf("Complete request changed:\n got %s\nwant %s", body, golden)
	}
	if resp.Text != "ok" {
		t.Fatalf("text = %q", resp.Text)
	}
}
