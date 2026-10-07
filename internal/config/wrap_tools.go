package config

import "github.com/Mpaape/AurumCode/internal/llm"

// contextInjectingToolProvider is contextInjectingProvider over a provider
// that calls tools (AUR-526 AC-007): the tool conversation carries the same
// block, appended to the first user message, so llm.AsToolCaller stays
// true through the decorator without letting a caller skip the injection.
// It exists only when the wrapped provider is itself a llm.ToolCaller, so a
// provider without tools never looks capable through it.
type contextInjectingToolProvider struct {
	*contextInjectingProvider
	caller llm.ToolCaller
}

// CompleteWithTools implements llm.ToolCaller.
func (p *contextInjectingToolProvider) CompleteWithTools(messages []llm.Message, tools []llm.ToolSpec, opts llm.Options) (llm.ToolResponse, error) {
	return p.caller.CompleteWithTools(appendToFirstUser(messages, "\n\n"+p.block), tools, opts)
}

// withToolCalling keeps base's tool capability visible through wrapped.
func withToolCalling(base llm.Provider, wrapped *contextInjectingProvider) llm.Provider {
	if caller, ok := llm.AsToolCaller(base); ok {
		return &contextInjectingToolProvider{contextInjectingProvider: wrapped, caller: caller}
	}
	return wrapped
}

// appendToFirstUser copies messages with suffix appended to the first user
// message (or a new user message when there is none).
func appendToFirstUser(messages []llm.Message, suffix string) []llm.Message {
	out := append([]llm.Message(nil), messages...)
	for i := range out {
		if out[i].Role == llm.RoleUser {
			out[i].Content += suffix
			return out
		}
	}
	return append(out, llm.Message{Role: llm.RoleUser, Content: suffix})
}
