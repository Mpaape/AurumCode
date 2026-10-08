package main

import (
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

// profileToolProvider is profileProvider over a provider that calls tools
// (AUR-526 AC-007): the tool conversation carries the same profile prefix,
// at the head of its first message, so llm.AsToolCaller stays true through
// the decorator without skipping the prefix. It exists only when the base
// provider is itself a llm.ToolCaller.
type profileToolProvider struct {
	profileProvider
	caller llm.ToolCaller
}

// CompleteWithTools implements llm.ToolCaller.
func (p profileToolProvider) CompleteWithTools(messages []llm.Message, tools []llm.ToolSpec, opts llm.Options) (llm.ToolResponse, error) {
	out := append([]llm.Message(nil), messages...)
	if len(out) == 0 {
		out = append(out, llm.Message{Role: llm.RoleUser})
	}
	out[0].Content = p.prefix() + out[0].Content
	return p.caller.CompleteWithTools(out, tools, opts)
}

// newProfileProvider decorates base with profile, keeping its tool
// capability visible when it has one.
func newProfileProvider(base llm.Provider, profile reviewprofile.Profile) llm.Provider {
	decorated := profileProvider{base: base, profile: profile}
	if caller, ok := llm.AsToolCaller(base); ok {
		return profileToolProvider{profileProvider: decorated, caller: caller}
	}
	return decorated
}
