package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

// TestCompleteChargesTheBudgetItChecked: the budget is checked against the
// requested model key but charged against whatever the response echoes back.
// A provider that omits the model field therefore never consumes budget, and
// the caller can spend without limit.
func TestCompleteChargesTheBudgetItChecked(t *testing.T) {
	provider := &mockProvider{
		name: "primary",
		response: Response{
			Text:      "ok",
			TokensIn:  100_000,
			TokensOut: 100_000,
			Model:     "", // provider did not echo a model
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})
	orch := NewOrchestrator(provider, nil, tracker)

	if _, err := orch.Complete(context.Background(), "prompt", Options{ModelKey: "test-model"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	perRun, daily := orch.RemainingBudget()
	if perRun == 10.0 || daily == 100.0 {
		t.Fatalf("a completed request left the budget untouched: perRun=%f daily=%f", perRun, daily)
	}
}

// TestCompleteReportsSpendItCouldNotCharge: with no price entry the run has no
// cost control at all, so that must be visible to the caller.
func TestCompleteReportsSpendItCouldNotCharge(t *testing.T) {
	provider := &mockProvider{
		name:     "primary",
		response: Response{Text: "ok", TokensIn: 10, TokensOut: 20, Model: "test-model"},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{})
	orch := NewOrchestrator(provider, nil, tracker)

	for i := 0; i < 3; i++ {
		if _, err := orch.Complete(context.Background(), "prompt", Options{ModelKey: "test-model"}); err != nil {
			t.Fatalf("Complete: %v", err)
		}
	}

	if got := orch.UntrackedSpends(); got != 3 {
		t.Fatalf("UntrackedSpends() = %d, want 3", got)
	}
}

// TestCompleteWithOnlyNilProvidersReportsNoProviders: a chain that cannot call
// anything has no providers; reporting "all providers failed" with a nil cause
// hides a configuration error behind a runtime one.
func TestCompleteWithOnlyNilProvidersReportsNoProviders(t *testing.T) {
	orch := NewOrchestrator(nil, []Provider{nil, nil}, nil)

	_, err := orch.Complete(context.Background(), "prompt", Options{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrNoProviders) {
		t.Fatalf("expected ErrNoProviders, got: %v", err)
	}
}
