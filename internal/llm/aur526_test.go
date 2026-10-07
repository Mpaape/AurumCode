package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// AC-008: a tool call without id or name, or with arguments that are not
// a JSON object, is refused.
func TestAUR526ValidateToolCall(t *testing.T) {
	if err := ValidateToolCall(ToolCall{ID: "c", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}); err != nil {
		t.Fatalf("a well-formed call was refused: %v", err)
	}
	for name, call := range map[string]ToolCall{
		"no id":      {Name: "read_file", Arguments: json.RawMessage(`{}`)},
		"no name":    {ID: "c", Arguments: json.RawMessage(`{}`)},
		"array args": {ID: "c", Name: "read_file", Arguments: json.RawMessage(`[1]`)},
		"no args":    {ID: "c", Name: "read_file"},
	} {
		var typed *MalformedToolCallError
		if err := ValidateToolCall(call); !errors.As(err, &typed) || !errors.Is(err, ErrMalformedToolArguments) {
			t.Errorf("%s: err = %v, want a MalformedToolCallError", name, err)
		}
	}
}

// AC-008 at the orchestrator: whatever provider returned it, an id-less
// call is refused before any tool can run.
func TestAUR526OrchestratorRefusesIDLessCall(t *testing.T) {
	p := &toolProvider{mockProvider: mockProvider{name: "tools"}, rounds: []ToolResponse{
		{ToolCalls: []ToolCall{{Name: "read_file", Arguments: json.RawMessage(`{}`)}}},
	}}
	resp, err := NewOrchestrator(p, nil, nil).CompleteWithTools(context.Background(), SystemUserMessages("s", "u"), nil, Options{})
	if !errors.Is(err, ErrMalformedToolArguments) || len(resp.ToolCalls) != 0 {
		t.Fatalf("err=%v calls=%+v", err, resp.ToolCalls)
	}
}
