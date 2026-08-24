package config

import (
	"context"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// contextInjectingProvider decorates an llm.Provider so every prompt it
// forwards carries the assembled, clearly-labeled, redacted context
// block appended after the reviewer's own prompt.
//
// THE COST-CEILING FIX: Tokens is overridden, not merely forwarded. The
// orchestrator's pre-flight budget check (internal/llm.Orchestrator.
// Complete, via its Estimator) calls provider.Tokens(prompt) on the
// SHORT, unexpanded prompt it was given -- Complete's own expansion
// happens one step later, inside THIS type's Complete below, which the
// budget check never sees. A wrapper that only forwarded Tokens (this
// card's first cut) let the orchestrator estimate cost against a prompt
// smaller than the one actually sent: a large provider contribution
// (a big repository prompt today; MCP or RAG content, larger still, once
// AUR-469/470 plug in) passed the ceiling check for free and was then
// sent in full. Overriding Tokens to always account for the block --
// regardless of what the literal input argument is -- makes the estimate
// match what Complete will actually transmit, so Reserve's ceiling check
// sees the true size before a single byte leaves the process.
type contextInjectingProvider struct {
	llm.Provider
	block string // never empty: WrapProvider does not construct this type otherwise
}

func (p *contextInjectingProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	return p.Provider.Complete(prompt+"\n\n"+p.block, opts)
}

// Tokens reports the token count of the EXPANDED prompt (input+block),
// the same expansion Complete performs, so the orchestrator's pre-flight
// Reserve is never budgeting against a smaller prompt than the one that
// actually gets sent.
func (p *contextInjectingProvider) Tokens(input string) (int, error) {
	return p.Provider.Tokens(input + "\n\n" + p.block)
}

// WrapProvider composes providers' contributions for changedPaths into
// one redacted block (BuildContextBlock, which applies the same AUR-009
// filter internal/review.Reviewer runs over the diff -- including a second
// pass after contributions are assembled) and returns an llm.Provider that
// appends that block to every outbound prompt before forwarding to base,
// with its Tokens accounting adjusted to match (see the type doc above).
//
// THE ZERO-CONFIG GUARANTEE: when providers is empty, or every provider
// contributes nothing for changedPaths (no .aurumcode/prompt.md, no
// matching .aurumcode/instructions/*.md -- the case with no repository
// files at all), WrapProvider returns base UNCHANGED: the exact same
// llm.Provider value, not a zero-effect wrapper around it. A caller that
// sends a prompt through the returned value therefore calls base's own
// Complete directly, with the exact same argument bytes, so the review's
// outbound request -- and everything downstream of it, cost accounting
// included -- is provably identical to what runs with no config package
// involved at all.
func WrapProvider(ctx context.Context, base llm.Provider, providers []ContextProvider, changedPaths []string, filter *redaction.Filter) (llm.Provider, error) {
	wrapped, _, err := WrapProviderWithWarnings(ctx, base, providers, changedPaths, filter)
	return wrapped, err
}

// WrapProviderWithWarnings composes optional context and returns the
// recoverable provider failures that the caller must announce. A failed
// provider is omitted from the block; a hard contribution-limit error still
// returns an error because silently sending a truncated context is unsafe.
func WrapProviderWithWarnings(ctx context.Context, base llm.Provider, providers []ContextProvider, changedPaths []string, filter *redaction.Filter) (llm.Provider, []ProviderWarning, error) {
	block, warnings, err := BuildContextBlockWithWarnings(ctx, providers, changedPaths, filter)
	if err != nil {
		return nil, warnings, err
	}
	if block == "" {
		return base, warnings, nil
	}
	return &contextInjectingProvider{Provider: base, block: block}, warnings, nil
}
