package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// ContextProvider is the extension seam AUR-468 (skills), AUR-469 (MCP)
// and AUR-470 (RAG) implement. Every source of context injected into the
// review call -- a file, a skill package, an MCP tool, a repository
// index -- is, from the engine's point of view, exactly this: a named
// source of free text, scoped to the files under review.
//
// A provider's returned text is UNTRUSTED DATA (see package doc). It can
// only ever become background material appended to the outbound model
// prompt (BuildContextBlock); it is never inspected for directives, so a
// provider cannot enable/disable a rule, change a finding's severity,
// loosen --fail-on, disable secret redaction, or change the cost cap --
// nothing in this package's API even accepts provider text at the call
// sites that decide those five things.
//
// Provide takes a ctx that BuildContextBlock bounds to ProviderTimeout: a
// provider that hangs (an unreachable MCP server, a stalled RAG index)
// must fail the review loudly within that bound, never hang it
// indefinitely. The text a provider returns has no size ceiling, whether
// it comes from an external source (an MCP server, a retrieval index) or
// from a CuratedProvider: it reaches the model whole, never truncated nor
// refused by size (issue #179; the rule catalog, in
// internal/prompt.ValidateRuleCatalog, is likewise never cut).
type ContextProvider interface {
	// Name identifies this provider in the rendered prompt and in
	// evidence/diagnostics. Stable across calls.
	Name() string
	// Provide returns free-text background for the given changed paths.
	// Returning "" (with a nil error) means "nothing to contribute for
	// this review" -- the zero-config path requires every provider to
	// answer this way when it finds nothing, so a review with no matching
	// file and no repository prompt stays byte-identical to a review with
	// zero providers registered at all. Provide must respect ctx
	// cancellation/deadline promptly: BuildContextBlock races it against
	// ProviderTimeout regardless of whether the provider itself honors
	// ctx, but a provider that does honor it (an HTTP call, an MCP
	// request) frees its own resources instead of leaking a goroutine
	// past the timeout.
	Provide(ctx context.Context, changedPaths []string) (string, error)
}

// ProviderWarning is a sanitized, user-facing notice that one optional
// context source was unavailable. The review remains valid without that
// source; callers must surface every warning instead of silently dropping it.
type ProviderWarning struct {
	Provider string
	Reason   string
}

// CuratedProvider is implemented by a provider that reads the repository's
// or the policy's own curated files: prompt, instructions, skills, docs.
// It marks where the contribution comes from; like every provider, it
// reaches the model whole (issue #179), and a prompt too long for the
// model makes the review split, never refuse a source.
type CuratedProvider interface {
	ContextProvider
	// Curated marks the provider; it has no behavior of its own.
	Curated()
}

// ProviderTimeout bounds a single ContextProvider.Provide call.
// AUR-469 (MCP) is the case this exists for: an external tool call that
// hangs must not hang the review it was supposed to inform. A provider
// that needs longer for a specific, known-slow operation is a later
// card's decision to make explicit (e.g. its own internal caching), not a
// reason to raise this shared ceiling.
const ProviderTimeout = 10 * time.Second

// The rendered block's header -- this material is informational, and the
// five decisions it names are made elsewhere -- and its source/contribution
// layout live in the review prompt template (internal/prompt's
// repository_context slot), the single source of every prompt section.

// callProviderBounded runs p.Provide under ProviderTimeout. A provider
// that does not answer in time or errors is a loud error identifying which
// provider caused it -- never a silent empty contribution, because that
// would be indistinguishable from "this provider legitimately had
// nothing to say" (the zero-config signal every other caller relies on).
func callProviderBounded(ctx context.Context, p ContextProvider, changedPaths []string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, ProviderTimeout)
	defer cancel()

	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		text, err := p.Provide(cctx, changedPaths)
		ch <- result{text: text, err: err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return "", fmt.Errorf("context provider %q: %w", p.Name(), r.err)
		}
		return r.text, nil
	case <-cctx.Done():
		return "", fmt.Errorf("context provider %q did not answer within %s: %w", p.Name(), ProviderTimeout, cctx.Err())
	}
}

// BuildContextBlock queries every provider in order (each bounded by
// callProviderBounded) and renders their non-empty contributions into one
// block. Provider failures are returned as warnings and do not discard the
// rest of the review; no contribution is refused by its size.
//
// Redaction runs twice. Each non-empty contribution is passed through the
// same AUR-009 filter before it is placed in the block, and the concatenated
// block is passed through the filter once more afterwards. The second pass is
// the one that closes the split-secret hole: a registered secret cut across
// two providers is not visible to either per-contribution pass (each half is
// harmless alone) but is visible in the assembled block. The per-contribution
// pass is kept on purpose, not replaced -- defense in depth, so a secret is
// contained at its own source even if the assembled pass is ever skipped or
// restructured. The block is redacted exactly once, not on every partial
// concatenation, so the extra cost stays proportional to the single final
// block. The source names are listed separately so the redaction input can
// preserve a plain newline boundary between contributions.
//
// Returns "" when no provider had anything to contribute -- the exact
// zero-config signal WrapProvider uses to leave the base LLM provider
// completely unwrapped.
func BuildContextBlock(ctx context.Context, providers []ContextProvider, changedPaths []string, filter *redaction.Filter) (string, error) {
	block, _, err := BuildContextBlockWithWarnings(ctx, providers, changedPaths, filter)
	return block, err
}

// BuildContextBlockWithWarnings is the warning-aware form used by the CLI.
// Provider failures are recoverable because context is advisory; malformed
// repository configuration is still caught earlier by config.Load.
func BuildContextBlockWithWarnings(ctx context.Context, providers []ContextProvider, changedPaths []string, filter *redaction.Filter) (string, []ProviderWarning, error) {
	var contributions []string
	var names []string
	var warnings []ProviderWarning
	for _, p := range providers {
		text, err := callProviderBounded(ctx, p, changedPaths)
		if err != nil {
			warning := ProviderWarning{Provider: p.Name(), Reason: err.Error()}
			if filter != nil {
				warning.Provider = filter.Redact(warning.Provider)
				warning.Reason = filter.Redact(warning.Reason)
			}
			warnings = append(warnings, warning)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if filter != nil {
			text = filter.Redact(text)
		}
		contributions = append(contributions, text)
		names = append(names, p.Name())
	}
	if len(contributions) == 0 {
		return "", warnings, nil
	}

	assembled := strings.Join(contributions, "\n")
	if filter != nil {
		assembled = filter.Redact(assembled)
	}

	return prompt.RenderRepositoryContext(names, assembled), warnings, nil
}
