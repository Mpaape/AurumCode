package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

// TestCompleteReleasesFailedReservationBeforeFallback: a provider attempt that
// errors never bills anything, so its reservation must be handed back before
// the next provider in the chain is tried. If the hold from the failed primary
// is left booked, the fallback's own reservation can be pushed over a ceiling
// the run actually had room under - the fallback is refused with
// ErrBudgetExceeded even though the failed attempt never charged a cent.
//
// The budget here is sized to fit exactly one reservation ($1.00, one call
// worth). The primary always fails. The fallback only fits if the primary's
// reservation was released first: with the hold leaked, the fallback's Reserve
// sees the ledger still carrying the primary's booking and refuses it.
//
// This guards internal/llm/orchestrator.go's call to reservation.Release() on
// the provider-error path: commenting that call out leaves every other test in
// this package green (reproduced separately), because none of them puts a
// second provider behind a budget too tight to survive a leaked hold.
func TestCompleteReleasesFailedReservationBeforeFallback(t *testing.T) {
	const modelKey = "test-model"

	primary := &mockProvider{
		name:       "primary",
		err:        errors.New("primary unavailable"),
		tokenCount: 1000,
	}

	fallback := &mockProvider{
		name: "fallback",
		response: Response{
			Text:      "fallback success",
			TokensIn:  1000,
			TokensOut: 1000,
			Model:     modelKey,
		},
		tokenCount: 1000,
	}

	// 1000 in * 0.5/1k + 1000 out * 0.5/1k = $1.00 per reservation. The per-run
	// ceiling fits exactly one of those - a second concurrent booking on top of
	// an unreleased first one must not fit.
	tracker := cost.NewTracker(1.0, 100.0, map[string]cost.PriceMap{
		modelKey: {InputPer1K: 0.5, OutputPer1K: 0.5},
	})

	orch := NewOrchestrator(primary, []Provider{fallback}, tracker)

	resp, err := orch.Complete(context.Background(), "prompt", Options{
		ModelKey:  modelKey,
		MaxTokens: 1000,
	})

	if err != nil {
		if errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("fallback was refused on budget it should have had: %v "+
				"(the primary's failed reservation was not released before the fallback tried to reserve)", err)
		}
		t.Fatalf("Complete: unexpected error: %v", err)
	}

	if resp.Text != "fallback success" {
		t.Fatalf("expected fallback response, got: %q", resp.Text)
	}

	if primary.callCount != 1 {
		t.Errorf("expected 1 call to primary, got: %d", primary.callCount)
	}

	if fallback.callCount != 1 {
		t.Fatalf("expected 1 call to fallback, got: %d (a leaked hold would have refused it before the call)", fallback.callCount)
	}
}
