package llm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

var (
	// ErrBudgetExceeded indicates the request exceeds available budget
	ErrBudgetExceeded = errors.New("budget exceeded")

	// ErrNoProviders indicates no providers are available
	ErrNoProviders = errors.New("no providers available")

	// ErrAllProvidersFailed indicates all providers in the chain failed
	ErrAllProvidersFailed = errors.New("all providers failed")
)

// defaultProviderTimeout bounds a single provider call when the caller's
// context carries no deadline of its own.
const defaultProviderTimeout = 60 * time.Second

// defaultMaxTokens is the output estimate used for the budget check when the
// caller does not cap the response.
const defaultMaxTokens = 1000

// Orchestrator manages LLM provider chains with fallback and budget enforcement
type Orchestrator struct {
	providers []Provider
	tracker   *cost.Tracker
	estimator *Estimator
	untracked atomic.Int64
}

// NewOrchestrator creates a new orchestrator with a primary provider and optional fallbacks
func NewOrchestrator(primary Provider, fallbacks []Provider, tracker *cost.Tracker) *Orchestrator {
	providers := make([]Provider, 0, len(fallbacks)+1)
	for _, p := range append([]Provider{primary}, fallbacks...) {
		if p != nil {
			providers = append(providers, p)
		}
	}

	return &Orchestrator{
		providers: providers,
		tracker:   tracker,
		estimator: NewEstimator(),
	}
}

// Complete executes a completion request with fallback chain and budget enforcement
func (o *Orchestrator) Complete(ctx context.Context, prompt string, opts Options) (Response, error) {
	if len(o.providers) == 0 {
		return Response{}, ErrNoProviders
	}

	tokensOut := opts.MaxTokens
	if tokensOut == 0 {
		tokensOut = defaultMaxTokens
	}

	var lastErr error

	for i, provider := range o.providers {
		tokensIn := o.estimator.Estimate(provider, prompt, opts.ModelKey)

		// The budget is checked and charged against the same key, so a request
		// can never be admitted under one price and billed under another.
		if o.tracker != nil && !o.tracker.Allow(tokensIn, tokensOut, opts.ModelKey) {
			return Response{}, fmt.Errorf("%w: insufficient budget for %s", ErrBudgetExceeded, provider.Name())
		}

		resp, err := o.executeWithTimeout(ctx, provider, prompt, opts)
		if err != nil {
			lastErr = fmt.Errorf("provider %s failed: %w", provider.Name(), err)

			if i < len(o.providers)-1 {
				continue
			}

			return Response{}, fmt.Errorf("%w: %w", ErrAllProvidersFailed, lastErr)
		}

		if o.tracker != nil {
			if err := o.tracker.Spend(resp.TokensIn, resp.TokensOut, opts.ModelKey); err != nil {
				// The request already succeeded and the money is already spent;
				// failing here would discard a paid response. Record instead,
				// so a run with no cost control is observable.
				o.untracked.Add(1)
			}
		}

		return resp, nil
	}

	return Response{}, fmt.Errorf("%w: %w", ErrAllProvidersFailed, lastErr)
}

// executeWithTimeout wraps provider execution with context timeout
func (o *Orchestrator) executeWithTimeout(ctx context.Context, provider Provider, prompt string, opts Options) (Response, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultProviderTimeout)
		defer cancel()
	}

	type result struct {
		resp Response
		err  error
	}

	// Buffered so the provider goroutine can finish and exit after a timeout
	// instead of blocking forever on an abandoned send.
	resultCh := make(chan result, 1)

	go func() {
		resp, err := provider.Complete(prompt, opts)
		resultCh <- result{resp: resp, err: err}
	}()

	select {
	case res := <-resultCh:
		return res.resp, res.err
	case <-ctx.Done():
		return Response{}, fmt.Errorf("request timeout: %w", ctx.Err())
	}
}

// GetProviderChain returns the current provider chain (primary + fallbacks)
func (o *Orchestrator) GetProviderChain() []string {
	names := make([]string, 0, len(o.providers))
	for _, p := range o.providers {
		names = append(names, p.Name())
	}
	return names
}

// RemainingBudget returns the remaining budget (perRun, daily)
func (o *Orchestrator) RemainingBudget() (float64, float64) {
	if o.tracker == nil {
		return 0, 0
	}
	return o.tracker.Remaining()
}

// UntrackedSpends returns how many completions were paid for but charged to no
// budget, because the model key has no price entry.
func (o *Orchestrator) UntrackedSpends() int {
	return int(o.untracked.Load())
}

// ResetPerRunBudget resets the per-run budget counter
func (o *Orchestrator) ResetPerRunBudget() {
	if o.tracker != nil {
		o.tracker.ResetPerRun()
	}
}
