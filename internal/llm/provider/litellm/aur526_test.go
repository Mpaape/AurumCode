package litellm

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// AC-008, incoming: a returned tool call without id is refused whole.
func TestAUR526IncomingCallWithoutIDIsRefused(t *testing.T) {
	reply := `{"model":"m","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
"tool_calls":[{"id":"","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]}}]}`
	srv := captureServer(t, reply, nil, nil)
	defer srv.Close()
	resp, err := NewProvider("k", srv.URL, "m").CompleteWithTools([]llm.Message{{Role: llm.RoleUser, Content: "x"}}, []llm.ToolSpec{readTool}, llm.Options{})
	if !errors.Is(err, llm.ErrMalformedToolArguments) || len(resp.ToolCalls) != 0 {
		t.Fatalf("err=%v calls=%+v, want the id-less call refused", err, resp.ToolCalls)
	}
}

// AC-008, outgoing: an assistant tool call without id or with non-object
// arguments is never sent.
func TestAUR526OutgoingInvalidCallIsNeverSent(t *testing.T) {
	for name, call := range map[string]llm.ToolCall{
		"no id":    {Name: "read_file", Arguments: json.RawMessage(`{}`)},
		"bad args": {ID: "c1", Name: "read_file", Arguments: json.RawMessage(`"x"`)},
	} {
		t.Run(name, func(t *testing.T) {
			var calls int32
			srv := captureServer(t, toolCallReply, nil, &calls)
			defer srv.Close()
			msgs := []llm.Message{{Role: llm.RoleUser, Content: "x"}, {Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{call}}, {Role: llm.RoleTool, ToolCallID: "c1", Content: "r"}}
			if _, err := NewProvider("k", srv.URL, "m").CompleteWithTools(msgs, []llm.ToolSpec{readTool}, llm.Options{}); err == nil {
				t.Fatal("an invalid outgoing tool call was accepted")
			}
			if calls != 0 {
				t.Fatalf("the invalid conversation reached the provider (%d request(s))", calls)
			}
		})
	}
}
