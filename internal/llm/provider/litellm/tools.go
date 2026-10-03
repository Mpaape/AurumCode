package litellm

import (
	"encoding/json"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// Tool calling over the OpenAI chat-completions wire format (AUR-525).
// Complete keeps its own request shape untouched; only CompleteWithTools
// sends these types.

type toolRequest struct {
	Model          string          `json:"model,omitempty"`
	Messages       []toolMessage   `json:"messages"`
	Tools          []wireTool      `json:"tools,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// toolMessage keeps content as a pointer: an assistant turn that only calls
// tools carries "content": null on the wire.
type toolMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireCallFunction `json:"function"`
}

// wireCallFunction.Arguments is a JSON-encoded string, as the format requires.
type wireCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      toolMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage usage `json:"usage"`
}

// CompleteWithTools sends a multi-message conversation with tool definitions
// and returns the model's text and/or tool calls. Implements llm.ToolCaller.
func (p *Provider) CompleteWithTools(messages []llm.Message, tools []llm.ToolSpec, opts llm.Options) (llm.ToolResponse, error) {
	reqBody := toolRequest{
		Model:          p.ResolveModel(opts),
		MaxTokens:      opts.MaxTokens,
		ResponseFormat: answerFormat(opts),
	}
	if opts.System != "" {
		system := opts.System
		reqBody.Messages = append(reqBody.Messages, toolMessage{Role: llm.RoleSystem, Content: &system})
	}
	for _, m := range messages {
		wm, err := toWireMessage(m)
		if err != nil {
			return llm.ToolResponse{}, err
		}
		reqBody.Messages = append(reqBody.Messages, wm)
	}
	for _, t := range tools {
		if t.Name == "" {
			return llm.ToolResponse{}, fmt.Errorf("tool definition without a name")
		}
		reqBody.Tools = append(reqBody.Tools, wireTool{
			Type:     "function",
			Function: wireFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		})
	}

	body, err := p.post(reqBody)
	if err != nil {
		return llm.ToolResponse{}, err
	}

	var completion toolCompletionResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return llm.ToolResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return llm.ToolResponse{}, fmt.Errorf("no choices in response")
	}

	choice := completion.Choices[0]
	out := llm.ToolResponse{Response: llm.Response{
		TokensIn:     completion.Usage.PromptTokens,
		TokensOut:    completion.Usage.CompletionTokens,
		Model:        completion.Model,
		FinishReason: choice.FinishReason,
	}}
	if choice.Message.Content != nil {
		out.Text = *choice.Message.Content
	}
	for _, c := range choice.Message.ToolCalls {
		if err := llm.ValidateToolArguments([]byte(c.Function.Arguments)); err != nil {
			return llm.ToolResponse{}, &llm.MalformedToolCallError{ID: c.ID, Name: c.Function.Name}
		}
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:        c.ID,
			Name:      c.Function.Name,
			Arguments: json.RawMessage(c.Function.Arguments),
		})
	}
	return out, nil
}

func toWireMessage(m llm.Message) (toolMessage, error) {
	switch m.Role {
	case llm.RoleSystem, llm.RoleUser:
		content := m.Content
		return toolMessage{Role: m.Role, Content: &content}, nil
	case llm.RoleAssistant:
		wm := toolMessage{Role: m.Role}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			content := m.Content
			wm.Content = &content
		}
		for _, c := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:       c.ID,
				Type:     "function",
				Function: wireCallFunction{Name: c.Name, Arguments: string(c.Arguments)},
			})
		}
		return wm, nil
	case llm.RoleTool:
		if m.ToolCallID == "" {
			return toolMessage{}, fmt.Errorf("tool result message without tool_call_id")
		}
		content := m.Content
		return toolMessage{Role: m.Role, Content: &content, ToolCallID: m.ToolCallID}, nil
	default:
		return toolMessage{}, fmt.Errorf("unknown message role %q", m.Role)
	}
}
