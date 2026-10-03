package llm

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Tool calling (AUR-525). A provider that can let the model request tools
// implements ToolCaller in addition to Provider. Nothing here executes a
// tool: the caller runs whatever the model asks for and sends the result back
// as a RoleTool message. Callers detect support with AsToolCaller BEFORE the
// call, so a provider without tools is never mistaken for a model that simply
// chose not to call any.

// Message roles used in a tool conversation.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolSpec describes one tool the model may call. Parameters is a JSON Schema
// object, carried verbatim to the provider.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall is one call the model asked for. Arguments is the JSON object the
// model produced; it is always valid JSON (a malformed one is an error, never
// a partial call).
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Message is one turn of a multi-message conversation. An assistant message
// may carry ToolCalls; a RoleTool message answers exactly one call and names
// it in ToolCallID.
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolResponse is a completion that may contain tool calls instead of, or in
// addition to, text.
type ToolResponse struct {
	Response
	ToolCalls []ToolCall
}

// ToolCaller is implemented by providers that support tool calling.
type ToolCaller interface {
	CompleteWithTools(messages []Message, tools []ToolSpec, opts Options) (ToolResponse, error)
}

// AsToolCaller reports whether provider supports tool calling, before any
// request is made.
func AsToolCaller(provider Provider) (ToolCaller, bool) {
	return As[ToolCaller](provider)
}

// ErrMalformedToolArguments is returned (wrapped in a *MalformedToolCallError)
// when the model asks for a tool with arguments that are not a JSON object.
var ErrMalformedToolArguments = errors.New("malformed tool call arguments")

// MalformedToolCallError names the call whose arguments could not be used.
type MalformedToolCallError struct {
	ID   string
	Name string
}

func (e *MalformedToolCallError) Error() string {
	return fmt.Sprintf("%v: tool %q (call %q)", ErrMalformedToolArguments, e.Name, e.ID)
}

func (e *MalformedToolCallError) Unwrap() error { return ErrMalformedToolArguments }

// ValidateToolArguments returns nil when raw is a JSON object.
func ValidateToolArguments(raw []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return ErrMalformedToolArguments
	}
	return nil
}
