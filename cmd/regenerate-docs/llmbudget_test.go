package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	litellmProvider "github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	openaiProvider "github.com/Mpaape/AurumCode/internal/llm/provider/openai"
)

// clearLLMEnv removes every cost-control variable so a case starts from a known
// state regardless of what the surrounding environment carries.
func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		envLLMModel, envLLMInputPer1K, envLLMOutputPer1K,
		envLLMPerRunUSD, envLLMDailyUSD, envLLMAllowUnpriced,
	} {
		t.Setenv(name, "")
	}
}

// TestResolveLLMBudgetPricesTheDefaultLiteLLMModel: the LiteLLM branch is the
// primary production path, and it used to hand the tracker an empty price
// table - a tracker that cannot price anything cannot cap anything.
func TestResolveLLMBudgetPricesTheDefaultLiteLLMModel(t *testing.T) {
	clearLLMEnv(t)

	budget, err := resolveLLMBudget(defaultLLMModel)
	if err != nil {
		t.Fatalf("resolveLLMBudget(%q): %v", defaultLLMModel, err)
	}

	if !budget.Enforced {
		t.Fatal("the default LiteLLM model resolved to an unenforceable budget")
	}
	if len(budget.Prices) == 0 {
		t.Fatal("price table is empty, so no ceiling can bind")
	}

	price, ok := budget.Prices[defaultLLMModel]
	if !ok {
		t.Fatalf("price table has no entry for %q, keys=%v", defaultLLMModel, keysOf(budget.Prices))
	}
	if price.InputPer1K <= 0 || price.OutputPer1K <= 0 {
		t.Fatalf("a zero price books nothing: %+v", price)
	}
}

// TestResolveLLMBudgetKeyIsWhatTheOrchestratorWillCharge ties the price-table
// key to the key the orchestrator actually resolves at call time. If these two
// drift, the table is populated but never consulted.
func TestResolveLLMBudgetKeyIsWhatTheOrchestratorWillCharge(t *testing.T) {
	clearLLMEnv(t)

	cases := []struct {
		name     string
		provider llm.Provider
		model    string
	}{
		{
			name:     "litellm",
			provider: litellmProvider.NewProvider("key", "https://example.invalid", defaultLLMModel),
			model:    defaultLLMModel,
		},
		{
			name:     "openai",
			provider: openaiProvider.NewProvider("key"),
			model:    openaiDefaultModel,
		},
	}

	if len(cases) == 0 {
		t.Fatal("no cases: a harness that verifies nothing must not report success")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			budget, err := resolveLLMBudget(tc.model)
			if err != nil {
				t.Fatalf("resolveLLMBudget(%q): %v", tc.model, err)
			}

			// DefaultOptions leaves ModelKey empty, which is the production case.
			resolved := llm.ResolveModelKey(tc.provider, llm.DefaultOptions())
			if resolved == "" {
				t.Fatal("provider resolved an empty model key, which no price table can contain")
			}

			if _, ok := budget.Prices[resolved]; !ok {
				t.Fatalf("orchestrator will charge %q but the price table only has %v",
					resolved, keysOf(budget.Prices))
			}
		})
	}
}

// TestResolveLLMBudgetFailsClosedOnUnpricedModel: a LiteLLM deployment alias is
// not in any catalog. Starting that run uncapped is the defect; refusing it,
// with the fix named, is the fix.
func TestResolveLLMBudgetFailsClosedOnUnpricedModel(t *testing.T) {
	clearLLMEnv(t)

	_, err := resolveLLMBudget("my-internal-deployment-alias")
	if err == nil {
		t.Fatal("an unpriceable model produced a budget instead of an error")
	}
	for _, want := range []string{envLLMInputPer1K, envLLMOutputPer1K, envLLMAllowUnpriced} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not tell the operator about %s: %v", want, err)
		}
	}
}

func TestResolveLLMBudgetExplicitPriceOverridesCatalog(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv(envLLMInputPer1K, "0.25")
	t.Setenv(envLLMOutputPer1K, "0.75")

	budget, err := resolveLLMBudget("my-internal-deployment-alias")
	if err != nil {
		t.Fatalf("resolveLLMBudget: %v", err)
	}
	if !budget.Enforced {
		t.Fatal("an explicitly priced model produced an unenforceable budget")
	}

	got := budget.Prices["my-internal-deployment-alias"]
	if got.InputPer1K != 0.25 || got.OutputPer1K != 0.75 {
		t.Fatalf("explicit price not applied: %+v", got)
	}
}

// A half-configured price silently values one side of the request at $0.
func TestResolveLLMBudgetRejectsHalfAPrice(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{name: "input only", in: "0.25", out: ""},
		{name: "output only", in: "", out: "0.75"},
	}

	if len(cases) == 0 {
		t.Fatal("no cases: a harness that verifies nothing must not report success")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearLLMEnv(t)
			t.Setenv(envLLMInputPer1K, tc.in)
			t.Setenv(envLLMOutputPer1K, tc.out)

			if _, err := resolveLLMBudget(defaultLLMModel); err == nil {
				t.Fatal("half a price was accepted, so one side of every request bills at $0")
			}
		})
	}
}

func TestResolveLLMBudgetExplicitUnpricedOptOut(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv(envLLMAllowUnpriced, "true")

	budget, err := resolveLLMBudget("my-internal-deployment-alias")
	if err != nil {
		t.Fatalf("resolveLLMBudget: %v", err)
	}
	if budget.Enforced {
		t.Fatal("an opted-out budget claims to be enforced")
	}
	if len(budget.Prices) != 0 {
		t.Fatalf("opted-out budget carries prices: %v", keysOf(budget.Prices))
	}

	// The tracker built from it must not block the run, and must report every
	// spend as unbooked rather than pretending it was charged.
	tracker := cost.NewTracker(budget.PerRunUSD, budget.DailyUSD, budget.Prices)
	if !tracker.Allow(1_000_000, 1_000_000, "my-internal-deployment-alias") {
		t.Fatal("an explicitly unpriced run was blocked")
	}
	if err := tracker.Spend(10, 10, "my-internal-deployment-alias"); err == nil {
		t.Fatal("Spend claimed success for a charge it never booked")
	}
	if got := tracker.Untracked(); got != 1 {
		t.Fatalf("Untracked() = %d, want 1", got)
	}
}

func TestResolveLLMBudgetRejectsBadNumbers(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{name: "per-run not a number", env: map[string]string{envLLMPerRunUSD: "lots"}},
		{name: "per-run zero", env: map[string]string{envLLMPerRunUSD: "0"}},
		{name: "per-run negative", env: map[string]string{envLLMPerRunUSD: "-5"}},
		{name: "daily not a number", env: map[string]string{envLLMDailyUSD: "plenty"}},
		{name: "daily below per-run", env: map[string]string{envLLMPerRunUSD: "100", envLLMDailyUSD: "10"}},
		{name: "price not a number", env: map[string]string{envLLMInputPer1K: "cheap", envLLMOutputPer1K: "0.1"}},
		{name: "price negative", env: map[string]string{envLLMInputPer1K: "-0.1", envLLMOutputPer1K: "0.1"}},
		{name: "opt-out garbage", env: map[string]string{envLLMAllowUnpriced: "maybe"}},
	}

	if len(cases) == 0 {
		t.Fatal("no cases: a harness that verifies nothing must not report success")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearLLMEnv(t)
			for name, value := range tc.env {
				t.Setenv(name, value)
			}

			// "opt-out garbage" only reaches the parser for an unpriced model.
			model := defaultLLMModel
			if _, ok := tc.env[envLLMAllowUnpriced]; ok {
				model = "my-internal-deployment-alias"
			}

			if budget, err := resolveLLMBudget(model); err == nil {
				t.Fatalf("bad configuration accepted: %+v", budget)
			}
		})
	}
}

func TestResolveLLMBudgetHonoursBudgetOverrides(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv(envLLMPerRunUSD, "1.5")
	t.Setenv(envLLMDailyUSD, "12.5")

	budget, err := resolveLLMBudget(defaultLLMModel)
	if err != nil {
		t.Fatalf("resolveLLMBudget: %v", err)
	}
	if budget.PerRunUSD != 1.5 {
		t.Errorf("PerRunUSD = %f, want 1.5", budget.PerRunUSD)
	}
	if budget.DailyUSD != 12.5 {
		t.Errorf("DailyUSD = %f, want 12.5", budget.DailyUSD)
	}
}

func TestResolveLLMBudgetRejectsEmptyModel(t *testing.T) {
	clearLLMEnv(t)

	if _, err := resolveLLMBudget("   "); err == nil {
		t.Fatal("an empty model name produced a budget")
	}
}

// TestResolvedBudgetActuallyCapsARun closes the loop: the configuration this
// package produces, wired into the real orchestrator, refuses spend past the
// ceiling. This is the property the empty price table silently removed.
func TestResolvedBudgetActuallyCapsARun(t *testing.T) {
	clearLLMEnv(t)
	t.Setenv(envLLMInputPer1K, "1.0")
	t.Setenv(envLLMOutputPer1K, "1.0")
	t.Setenv(envLLMPerRunUSD, "2.0")
	t.Setenv(envLLMDailyUSD, "2.0")

	const model = "my-internal-deployment-alias"

	budget, err := resolveLLMBudget(model)
	if err != nil {
		t.Fatalf("resolveLLMBudget: %v", err)
	}

	tracker := cost.NewTracker(budget.PerRunUSD, budget.DailyUSD, budget.Prices)

	// $1.00 of input plus $1.00 of output is exactly the ceiling.
	res, ok := tracker.Reserve(1000, 1000, model)
	if !ok {
		t.Fatal("the first request, which fits exactly, was refused")
	}
	if err := res.Commit(1000, 1000, model); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, ok := tracker.Reserve(1000, 1000, model); ok {
		perRun, daily := tracker.Remaining()
		t.Fatalf("a second request was admitted past the ceiling: perRun=%f daily=%f", perRun, daily)
	}
}

func keysOf(m map[string]cost.PriceMap) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
