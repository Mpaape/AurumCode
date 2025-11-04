package llm

import (
	"context"
	"errors"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"strings"
	"testing"
	"time"
)

// mockProvider is a configurable mock provider for testing
type mockProvider struct {
	name       string
	response   Response
	err        error
	callCount  int
	tokenCount int
}

func (m *mockProvider) Complete(prompt string, opts Options) (Response, error) {
	m.callCount++
	if m.err != nil {
		return Response{}, m.err
	}
	return m.response, nil
}

func (m *mockProvider) Tokens(input string) (int, error) {
	if m.tokenCount > 0 {
		return m.tokenCount, nil
	}
	return len(input) / 4, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func TestOrchestratorComplete_Success(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		response: Response{
			Text:      "Success response",
			TokensIn:  100,
			TokensOut: 200,
			Model:     "test-model",
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	orch := NewOrchestrator(primary, nil, tracker)

	resp, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.Text != "Success response" {
		t.Errorf("expected 'Success response', got: %s", resp.Text)
	}

	if primary.callCount != 1 {
		t.Errorf("expected 1 call to primary, got: %d", primary.callCount)
	}

	// Verify budget was spent
	perRun, _ := orch.RemainingBudget()
	if perRun >= 10.0 {
		t.Errorf("expected budget to be spent, remaining: %f", perRun)
	}
}

func TestOrchestratorComplete_FallbackOnError(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		err:  errors.New("primary failed"),
	}

	fallback := &mockProvider{
		name: "fallback",
		response: Response{
			Text:      "Fallback response",
			TokensIn:  100,
			TokensOut: 150,
			Model:     "fallback-model",
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model":     {InputPer1K: 0.01, OutputPer1K: 0.02},
		"fallback-model": {InputPer1K: 0.005, OutputPer1K: 0.01},
	})

	orch := NewOrchestrator(primary, []Provider{fallback}, tracker)

	resp, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.Text != "Fallback response" {
		t.Errorf("expected 'Fallback response', got: %s", resp.Text)
	}

	if primary.callCount != 1 {
		t.Errorf("expected 1 call to primary, got: %d", primary.callCount)
	}

	if fallback.callCount != 1 {
		t.Errorf("expected 1 call to fallback, got: %d", fallback.callCount)
	}
}

func TestOrchestratorComplete_AllProvidersFail(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		err:  errors.New("primary failed"),
	}

	fallback := &mockProvider{
		name: "fallback",
		err:  errors.New("fallback failed"),
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	orch := NewOrchestrator(primary, []Provider{fallback}, tracker)

	_, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err == nil {
		t.Fatal("expected error when all providers fail")
	}

	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Errorf("expected ErrAllProvidersFailed, got: %v", err)
	}

	if primary.callCount != 1 {
		t.Errorf("expected 1 call to primary, got: %d", primary.callCount)
	}

	if fallback.callCount != 1 {
		t.Errorf("expected 1 call to fallback, got: %d", fallback.callCount)
	}
}

func TestOrchestratorComplete_BudgetExceeded(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		response: Response{
			Text:      "Success",
			TokensIn:  100,
			TokensOut: 200,
			Model:     "test-model",
		},
	}

	// Very low budget to trigger exceed
	tracker := cost.NewTracker(0.001, 0.001, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 1.0, OutputPer1K: 2.0},
	})

	orch := NewOrchestrator(primary, nil, tracker)

	_, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err == nil {
		t.Fatal("expected budget exceeded error")
	}

	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("expected ErrBudgetExceeded, got: %v", err)
	}

	if primary.callCount != 0 {
		t.Errorf("expected 0 calls when budget exceeded, got: %d", primary.callCount)
	}
}

func TestOrchestratorComplete_NoProviders(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil)

	_, err := orch.Complete(context.Background(), "test prompt", Options{})

	if err == nil {
		t.Fatal("expected error with no providers")
	}

	if !errors.Is(err, ErrNoProviders) {
		t.Errorf("expected ErrNoProviders, got: %v", err)
	}
}

// slowProvider is a mock provider that simulates slow response
type slowProvider struct {
	name     string
	delay    time.Duration
	response Response
}

func (s *slowProvider) Complete(prompt string, opts Options) (Response, error) {
	time.Sleep(s.delay)
	return s.response, nil
}

func (s *slowProvider) Tokens(input string) (int, error) {
	return len(input) / 4, nil
}

func (s *slowProvider) Name() string {
	return s.name
}

func TestOrchestratorComplete_ContextTimeout(t *testing.T) {
	slow := &slowProvider{
		name:  "slow",
		delay: 200 * time.Millisecond,
		response: Response{
			Text:      "Success",
			TokensIn:  100,
			TokensOut: 200,
			Model:     "test-model",
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{})
	orch := NewOrchestrator(slow, nil, tracker)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := orch.Complete(ctx, "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err == nil {
		t.Fatal("expected timeout error")
	}

	// Check if error is wrapped with context.DeadlineExceeded
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrAllProvidersFailed) {
		t.Errorf("expected timeout-related error, got: %v", err)
	}

	// Verify that the error contains deadline exceeded somewhere in the chain
	errStr := err.Error()
	if !strings.Contains(errStr, "deadline exceeded") && !strings.Contains(errStr, "timeout") {
		t.Errorf("expected error to mention timeout or deadline, got: %v", err)
	}
}

func TestOrchestratorComplete_MultipleFallbacks(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		err:  errors.New("primary failed"),
	}

	fallback1 := &mockProvider{
		name: "fallback1",
		err:  errors.New("fallback1 failed"),
	}

	fallback2 := &mockProvider{
		name: "fallback2",
		response: Response{
			Text:      "Success from fallback2",
			TokensIn:  100,
			TokensOut: 150,
			Model:     "fallback2-model",
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model":      {InputPer1K: 0.01, OutputPer1K: 0.02},
		"fallback2-model": {InputPer1K: 0.001, OutputPer1K: 0.002},
	})

	orch := NewOrchestrator(primary, []Provider{fallback1, fallback2}, tracker)

	resp, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.Text != "Success from fallback2" {
		t.Errorf("expected 'Success from fallback2', got: %s", resp.Text)
	}

	if primary.callCount != 1 {
		t.Errorf("expected 1 call to primary, got: %d", primary.callCount)
	}

	if fallback1.callCount != 1 {
		t.Errorf("expected 1 call to fallback1, got: %d", fallback1.callCount)
	}

	if fallback2.callCount != 1 {
		t.Errorf("expected 1 call to fallback2, got: %d", fallback2.callCount)
	}
}

func TestOrchestratorGetProviderChain(t *testing.T) {
	primary := &mockProvider{name: "primary"}
	fallback1 := &mockProvider{name: "fallback1"}
	fallback2 := &mockProvider{name: "fallback2"}

	orch := NewOrchestrator(primary, []Provider{fallback1, fallback2}, nil)

	chain := orch.GetProviderChain()

	expected := []string{"primary", "fallback1", "fallback2"}
	if len(chain) != len(expected) {
		t.Fatalf("expected chain length %d, got %d", len(expected), len(chain))
	}

	for i, name := range expected {
		if chain[i] != name {
			t.Errorf("expected chain[%d] = %s, got %s", i, name, chain[i])
		}
	}
}

func TestOrchestratorRemainingBudget(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		response: Response{
			Text:      "Success",
			TokensIn:  100,
			TokensOut: 200,
			Model:     "test-model",
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	orch := NewOrchestrator(primary, nil, tracker)

	// Initial budget
	perRun, daily := orch.RemainingBudget()
	if perRun != 10.0 || daily != 100.0 {
		t.Errorf("expected initial budget (10.0, 100.0), got (%f, %f)", perRun, daily)
	}

	// Execute request
	_, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Budget should be reduced
	perRun, daily = orch.RemainingBudget()
	if perRun >= 10.0 {
		t.Errorf("expected reduced per-run budget, got %f", perRun)
	}
	if daily >= 100.0 {
		t.Errorf("expected reduced daily budget, got %f", daily)
	}
}

func TestOrchestratorResetPerRunBudget(t *testing.T) {
	primary := &mockProvider{
		name: "primary",
		response: Response{
			Text:      "Success",
			TokensIn:  100,
			TokensOut: 200,
			Model:     "test-model",
		},
	}

	tracker := cost.NewTracker(10.0, 100.0, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	orch := NewOrchestrator(primary, nil, tracker)

	// Execute request
	_, err := orch.Complete(context.Background(), "test prompt", Options{
		ModelKey:  "test-model",
		MaxTokens: 500,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Budget should be reduced
	perRun, _ := orch.RemainingBudget()
	if perRun >= 10.0 {
		t.Errorf("expected reduced per-run budget, got %f", perRun)
	}

	// Reset per-run budget
	orch.ResetPerRunBudget()

	// Per-run should be restored, daily should remain reduced
	perRun, daily := orch.RemainingBudget()
	if perRun != 10.0 {
		t.Errorf("expected restored per-run budget 10.0, got %f", perRun)
	}
	if daily >= 100.0 {
		t.Errorf("expected reduced daily budget, got %f", daily)
	}
}

// BenchmarkOrchestratorComplete measures orchestration overhead
func BenchmarkOrchestratorComplete(b *testing.B) {
	primary := &mockProvider{
		name: "primary",
		response: Response{
			Text:      "Success",
			TokensIn:  100,
			TokensOut: 200,
			Model:     "test-model",
		},
	}

	tracker := cost.NewTracker(1000.0, 10000.0, map[string]cost.PriceMap{
		"test-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	orch := NewOrchestrator(primary, nil, tracker)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := orch.Complete(context.Background(), "benchmark prompt", Options{
			ModelKey:  "test-model",
			MaxTokens: 500,
		})
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		orch.ResetPerRunBudget()
	}
}
