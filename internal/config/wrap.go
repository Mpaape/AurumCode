package config

import "github.com/Mpaape/AurumCode/internal/llm"

// contextInjectingProvider decorates an llm.Provider so every prompt it
// forwards carries the assembled, clearly-labeled context block appended
// after the reviewer's own prompt. The embedded llm.Provider forwards
// Tokens and Name unchanged; only Complete is overridden, and even there
// only the prompt argument changes -- opts, the error handling and the
// response are passed through untouched.
type contextInjectingProvider struct {
	llm.Provider
	block string // never empty: WrapProvider does not construct this type otherwise
}

func (p *contextInjectingProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	return p.Provider.Complete(prompt+"\n\n"+p.block, opts)
}

// WrapProvider composes providers' contributions for changedPaths into one
// block (BuildContextBlock) and returns an llm.Provider that appends it to
// every outbound prompt before forwarding to base.
//
// THE ZERO-CONFIG GUARANTEE: when providers is empty, or every provider
// contributes nothing for changedPaths (no .aurumcode/prompt.md, no
// matching .aurumcode/instructions/*.md -- the case with no repository
// files at all), WrapProvider returns base UNCHANGED: the exact same
// llm.Provider value, not a zero-effect wrapper around it. A caller that
// sends a prompt through the returned value therefore calls base's own
// Complete directly, with the exact same argument bytes, so the review's
// outbound request -- and everything downstream of it -- is provably
// identical to what runs with no config package involved at all.
func WrapProvider(base llm.Provider, providers []ContextProvider, changedPaths []string) (llm.Provider, error) {
	block, err := BuildContextBlock(providers, changedPaths)
	if err != nil {
		return nil, err
	}
	if block == "" {
		return base, nil
	}
	return &contextInjectingProvider{Provider: base, block: block}, nil
}
