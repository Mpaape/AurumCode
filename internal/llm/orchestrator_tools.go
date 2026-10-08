package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

// ErrToolsUnsupported is returned by CompleteWithTools when no provider of
// the chain implements ToolCaller. A provider without tool calling is never
// sent a tool conversation: falling back to it would silently drop the
// tools the model was offered.
var ErrToolsUnsupported = errors.New("no provider in the chain can call tools")

// SupportsTools reports whether at least one provider of the chain can take
// a tool conversation, before any request is made.
func (o *Orchestrator) SupportsTools() bool {
	for _, p := range o.providers {
		if _, ok := AsToolCaller(p); ok {
			return true
		}
	}
	return false
}

// CompleteWithTools sends one round of a tool conversation. Each call is
// one round: the cost of the round is reserved before the provider is
// called and committed (or released) after, exactly like Complete, so a
// multi-round deliberation books the sum of its rounds and the ceiling is
// checked before every round, not once per conversation. Fallback only
// moves between providers that implement ToolCaller.
func (o *Orchestrator) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolSpec, opts Options) (ToolResponse, error) {
	estimateText := toolRoundText(messages, tools)
	tokensOut := opts.MaxTokens
	if tokensOut == 0 {
		tokensOut = defaultMaxTokens
	}
	var lastErr error
	tried := 0
	for _, provider := range o.providers {
		caller, ok := AsToolCaller(provider)
		if !ok {
			continue
		}
		tried++
		modelKey := ResolveModelKey(provider, opts)
		tokensIn := o.estimator.Estimate(provider, estimateText, modelKey)
		reservation, err := o.reserveRound(provider, tokensIn, tokensOut, modelKey)
		if err != nil {
			return ToolResponse{}, err
		}
		resp, err := runWithTimeout(ctx, func() (ToolResponse, error) {
			return caller.CompleteWithTools(messages, tools, opts)
		})
		if err != nil {
			reservation.Release()
			lastErr = fmt.Errorf("provider %s failed: %w", provider.Name(), err)
			continue
		}
		o.commitRound(reservation, resp.TokensIn, resp.TokensOut, modelKey)
		// AUR-526 AC-008: whatever the provider, a call without id or name,
		// or with arguments that are not an object, never reaches a tool.
		for _, c := range resp.ToolCalls {
			if err := ValidateToolCall(c); err != nil {
				return ToolResponse{}, fmt.Errorf("provider %s: %w", provider.Name(), err)
			}
		}
		return resp, nil
	}
	if tried == 0 {
		return ToolResponse{}, ErrToolsUnsupported
	}
	return ToolResponse{}, fmt.Errorf("%w: %w", ErrAllProvidersFailed, lastErr)
}

// reserveRound books one round's estimate against the tracker. A nil
// tracker reserves nothing (no cost control was configured).
func (o *Orchestrator) reserveRound(provider Provider, tokensIn, tokensOut int, modelKey string) (*cost.Reservation, error) {
	if o.tracker == nil {
		return nil, nil
	}
	res, ok := o.tracker.Reserve(tokensIn, tokensOut, modelKey)
	if !ok {
		return nil, fmt.Errorf("%w: insufficient budget for %s (model %q)", ErrBudgetExceeded, provider.Name(), modelKey)
	}
	return res, nil
}

// commitRound swaps a round's reservation for its actual cost.
func (o *Orchestrator) commitRound(reservation *cost.Reservation, tokensIn, tokensOut int, modelKey string) {
	if reservation == nil {
		return
	}
	if err := reservation.Commit(tokensIn, tokensOut, modelKey); err != nil {
		o.untracked.Add(1)
	}
}

// toolRoundText is the text a round's input estimate is taken on: the
// flattened conversation plus the tool definitions sent beside it.
func toolRoundText(messages []Message, tools []ToolSpec) string {
	parts := []string{FlattenMessages(messages)}
	for _, m := range messages {
		for _, c := range m.ToolCalls {
			parts = append(parts, c.Name, string(c.Arguments))
		}
	}
	for _, t := range tools {
		parts = append(parts, t.Name, t.Description, string(t.Parameters))
	}
	return strings.Join(parts, messageSeparator)
}

// runWithTimeout runs call under ctx, bounded by ProviderTimeout when ctx
// has no deadline. The call keeps running after a timeout but its answer
// is discarded.
func runWithTimeout[T any](ctx context.Context, call func() (T, error)) (T, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ProviderTimeout())
		defer cancel()
	}
	type result struct {
		value T
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := call()
		ch <- result{value: v, err: err}
	}()
	select {
	case r := <-ch:
		return r.value, r.err
	case <-ctx.Done():
		var zero T
		return zero, fmt.Errorf("request timeout: %w", ctx.Err())
	}
}

// responseSchemaName is the schema name sent when the caller set none.
const responseSchemaName = "response"

// ResponseSchemaNameOrDefault is the schema name sent with ResponseSchema.
func (o Options) ResponseSchemaNameOrDefault() string {
	if strings.TrimSpace(o.ResponseSchemaName) != "" {
		return o.ResponseSchemaName
	}
	return responseSchemaName
}
