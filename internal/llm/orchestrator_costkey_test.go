package llm

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

// resolvingProvider is a provider that, like every real provider in this
// repository, substitutes a model of its own when the caller names none. It
// reports that substitution up front so the ceiling and the charge can agree.
type resolvingProvider struct {
	name     string
	model    string
	response Response

	// gate, when non-nil, blocks inside Complete until it is closed. It widens
	// the window between the budget decision and the charge, which is where a
	// check-then-act ledger loses requests.
	gate    chan struct{}
	arrived atomic.Int64
}

func (p *resolvingProvider) ResolveModel(opts Options) string {
	if opts.ModelKey != "" {
		return opts.ModelKey
	}
	return p.model
}

func (p *resolvingProvider) Complete(prompt string, opts Options) (Response, error) {
	if p.gate != nil {
		p.arrived.Add(1)
		<-p.gate
	}
	resp := p.response
	return resp, nil
}

func (p *resolvingProvider) Tokens(input string) (int, error) { return len(input) / 4, nil }
func (p *resolvingProvider) Name() string                     { return p.name }

// TestOrchestrator_ProductionWiring_CapAndChargeShareResolvedModelKey: the
// ceiling is checked against one key and the charge is booked against another,
// so the ledger the ceiling reads never moves and the cap is silently inert.
//
// The provider here bills "vendor-model". A run that checks the ceiling against
// something else is spending real money against a limit it can never reach.
func TestOrchestrator_ProductionWiring_CapAndChargeShareResolvedModelKey(t *testing.T) {
	provider := &resolvingProvider{
		name:  "primary",
		model: "vendor-model",
		response: Response{
			Text:      "ok",
			TokensIn:  3000,
			TokensOut: 3000,
			// The provider does not echo a model back, which is exactly what a
			// charge keyed on the response cannot survive.
			Model: "",
		},
	}

	tracker := cost.NewTracker(1.0, 10.0, map[string]cost.PriceMap{
		"vendor-model": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})
	orch := NewOrchestrator(provider, nil, tracker)

	// No ModelKey: DefaultOptions never sets one, so this is the production path.
	if _, err := orch.Complete(context.Background(), "prompt", Options{MaxTokens: 3000}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// 3000 in * 0.01/1k + 3000 out * 0.02/1k = 0.03 + 0.06 = 0.09
	const wantPerRun = 1.0 - 0.09
	perRun, _ := orch.RemainingBudget()
	if !nearly(perRun, wantPerRun) {
		t.Fatalf("charge did not land on the resolved model key: want per-run remaining %f, got %f",
			wantPerRun, perRun)
	}

	if got := orch.UntrackedSpends(); got != 0 {
		t.Fatalf("a charge that should have been booked was recorded as untracked: %d", got)
	}
}

// TestOrchestrator_ProductionWiring_EmptyModelKeyStillCharges: DefaultOptions()
// leaves ModelKey empty, so an orchestrator that prices on opts.ModelKey prices
// on "" - a key no price table contains - for every default request.
func TestOrchestrator_ProductionWiring_EmptyModelKeyStillCharges(t *testing.T) {
	provider := &resolvingProvider{
		name:     "primary",
		model:    "vendor-model",
		response: Response{Text: "ok", TokensIn: 1000, TokensOut: 1000, Model: ""},
	}

	tracker := cost.NewTracker(100.0, 100.0, map[string]cost.PriceMap{
		"vendor-model": {InputPer1K: 0.05, OutputPer1K: 0.04},
	})
	orch := NewOrchestrator(provider, nil, tracker)

	opts := DefaultOptions()
	if opts.ModelKey != "" {
		t.Fatalf("precondition broken: DefaultOptions now sets ModelKey=%q", opts.ModelKey)
	}

	if _, err := orch.Complete(context.Background(), "prompt", opts); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// 1000 in * 0.05/1k + 1000 out * 0.04/1k = 0.05 + 0.04 = 0.09
	const wantPerRun = 100.0 - 0.09
	perRun, _ := orch.RemainingBudget()
	if !nearly(perRun, wantPerRun) {
		t.Fatalf("per-run counter did not move: want %f, got %f", wantPerRun, perRun)
	}
}

// TestOrchestrator_ConcurrentCompletesNeverExceedPerRunCap: a ceiling that is
// read and then written in two steps admits every request that arrives inside
// the window, because none of them has written to the ledger the others read.
//
// The budget below fits exactly one request. Anything above one admission is
// money spent past a cap the operator set.
func TestOrchestrator_ConcurrentCompletesNeverExceedPerRunCap(t *testing.T) {
	const (
		callers  = 8
		tokensIn = 1000
		tokenOut = 1000
	)

	gate := make(chan struct{})
	provider := &resolvingProvider{
		name:     "primary",
		model:    "vendor-model",
		gate:     gate,
		response: Response{Text: "ok", TokensIn: tokensIn, TokensOut: tokenOut},
	}

	// 1000 in * 1.0/1k + 1000 out * 1.0/1k = $2.00 per call; the cap fits one.
	tracker := cost.NewTracker(2.0, 1000.0, map[string]cost.PriceMap{
		"vendor-model": {InputPer1K: 1.0, OutputPer1K: 1.0},
	})
	orch := NewOrchestrator(provider, nil, tracker)

	var admitted, refused atomic.Int64
	var wg sync.WaitGroup

	// The prompt is sized so the estimate matches what the provider reports, so
	// the reservation and the final charge are the same number and the test
	// measures admission control, not estimation drift.
	prompt := makePrompt(tokensIn * 4)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := orch.Complete(context.Background(), prompt, Options{MaxTokens: tokenOut})
			if err != nil {
				refused.Add(1)
				return
			}
			admitted.Add(1)
		}()
	}

	// Release only once every caller has either been refused at the gate or is
	// parked inside the provider. No sleep: the condition is observable.
	deadline := time.Now().Add(10 * time.Second)
	for provider.arrived.Load()+refused.Load() < callers {
		if time.Now().After(deadline) {
			t.Fatalf("callers never settled: arrived=%d refused=%d of %d",
				provider.arrived.Load(), refused.Load(), callers)
		}
		runtime.Gosched()
	}
	close(gate)
	wg.Wait()

	// A harness that observed nothing must never report success.
	if got := admitted.Load() + refused.Load(); got != callers {
		t.Fatalf("harness lost callers: accounted %d of %d", got, callers)
	}

	if got := admitted.Load(); got != 1 {
		t.Errorf("admitted %d concurrent calls against a cap that fits exactly 1", got)
	}

	perRun, _ := orch.RemainingBudget()
	if perRun < 0 {
		t.Errorf("per-run budget went negative: %f", perRun)
	}
}

func makePrompt(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// nearly compares dollar amounts without asserting exact float equality.
func nearly(got, want float64) bool {
	const epsilon = 1e-9
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < epsilon
}
