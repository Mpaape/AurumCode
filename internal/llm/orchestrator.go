package llm

import (
	"context"
	"errors"
	"fmt"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"time"
)

var (
	// ErrBudgetExceeded indicates the request exceeds available budget
	ErrBudgetExceeded = errors.New("budget exceeded")

	// ErrNoProviders indicates no providers are available
	ErrNoProviders = errors.New("no providers available")

	// ErrAllProvidersFailed indicates all providers in the chain failed
	ErrAllProvidersFailed = errors.New("all providers failed")
)

// Orchestrator manages LLM provider chains with fallback and budget enforcement
type Orchestrator struct {
	primary   Provider
	fallbacks []Provider
	tracker   *cost.Tracker
	estimator *Estimator
}

// NewOrchestrator creates a new orchestrator with a primary provider and optional fallbacks
func NewOrchestrator(primary Provider, fallbacks []Provider, tracker *cost.Tracker) *Orchestrator {
	return &Orchestrator{
		primary:   primary,
		fallbacks: fallbacks,
		tracker:   tracker,
		estimator: NewEstimator(),
	}
}

// Complete executes a completion request with fallback chain and budget enforcement
func (o *Orchestrator) Complete(ctx context.Context, prompt string, opts Options) (Response, error) {
	if o.primary == nil && len(o.fallbacks) == 0 {
		return Response{}, ErrNoProviders
	}

	// Build provider chain: primary + fallbacks
	providers := []Provider{o.primary}
	providers = append(providers, o.fallbacks...)

	// Estimate tokens
	tokensIn, err := o.estimator.EstimateTokens(prompt)
	if err != nil {
		// Use heuristic if estimation fails
		tokensIn = len(prompt) / 4
	}

	tokensOut := opts.MaxTokens
	if tokensOut == 0 {
		tokensOut = 1000 // reasonable default estimate
	}

	var lastErr error

	// Try each provider in order
	for i, provider := range providers {
		if provider == nil {
			continue
		}

		// Check budget before attempting
		model := opts.ModelKey
		if model == "" {
			model = "default"
		}

		if o.tracker != nil && !o.tracker.Allow(tokensIn, tokensOut, model) {
			return Response{}, fmt.Errorf("%w: insufficient budget for %s", ErrBudgetExceeded, provider.Name())
		}

		// Execute with timeout
		resp, err := o.executeWithTimeout(ctx, provider, prompt, opts)
		if err != nil {
			lastErr = fmt.Errorf("provider %s failed: %w", provider.Name(), err)

			// If this is not the last provider, continue to next
			if i < len(providers)-1 {
				continue
			}

			// All providers failed
			return Response{}, fmt.Errorf("%w: %v", ErrAllProvidersFailed, lastErr)
		}

		// Success - record spending
		if o.tracker != nil {
			if err := o.tracker.Spend(resp.TokensIn, resp.TokensOut, resp.Model); err != nil {
				// Log but don't fail on tracking error
				fmt.Printf("warning: failed to record spending: %v\n", err)
			}
		}

		return resp, nil
	}

	return Response{}, fmt.Errorf("%w: %v", ErrAllProvidersFailed, lastErr)
}

// executeWithTimeout wraps provider execution with context timeout
func (o *Orchestrator) executeWithTimeout(ctx context.Context, provider Provider, prompt string, opts Options) (Response, error) {
	// Create timeout context if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}

	// Execute in goroutine to respect context
	type result struct {
		resp Response
		err  error
	}

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
	names := []string{}

	if o.primary != nil {
		names = append(names, o.primary.Name())
	}

	for _, p := range o.fallbacks {
		if p != nil {
			names = append(names, p.Name())
		}
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

// ResetPerRunBudget resets the per-run budget counter
func (o *Orchestrator) ResetPerRunBudget() {
	if o.tracker != nil {
		o.tracker.ResetPerRun()
	}
}
