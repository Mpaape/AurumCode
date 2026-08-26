package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

// Cost control is configured through these variables. The price pair is what
// turns the tracker from a counter that books nothing into an enforceable
// ceiling: without a price for the model actually being called, the tracker has
// no way to convert tokens into dollars, so no limit it is given can bind.
const (
	envLLMModel         = "LLM_MODEL"
	envLLMInputPer1K    = "AURUMCODE_LLM_INPUT_USD_PER_1K"
	envLLMOutputPer1K   = "AURUMCODE_LLM_OUTPUT_USD_PER_1K"
	envLLMPerRunUSD     = "AURUMCODE_LLM_PER_RUN_USD"
	envLLMDailyUSD      = "AURUMCODE_LLM_DAILY_USD"
	envLLMAllowUnpriced = "AURUMCODE_LLM_ALLOW_UNPRICED"
)

// defaultLLMModel is the model the LiteLLM branch calls when none is named.
// Haiku keeps the editorial pass responsive and inexpensive; callers can
// select a stronger model explicitly through LLM_MODEL.
const defaultLLMModel = "claude-3-5-haiku-20241022"

const (
	defaultPerRunUSD = 1000.0
	defaultDailyUSD  = 10000.0
)

// catalogUSDPer1K holds published list prices in US dollars per 1000 tokens,
// for the models this binary is most often pointed at.
//
// These are defaults, not a source of truth: vendor prices change, and a
// LiteLLM deployment name is usually an alias that appears nowhere in this
// table. Set the AURUMCODE_LLM_*_USD_PER_1K pair to state the real price for
// the model in use. Where a rate was ambiguous the higher figure was taken, so
// the ceiling binds early rather than leaking.
var catalogUSDPer1K = map[string]cost.PriceMap{
	"gpt-4o-mini":                {InputPer1K: 0.00015, OutputPer1K: 0.0006},
	"gpt-4o":                     {InputPer1K: 0.0025, OutputPer1K: 0.01},
	"gpt-4-turbo":                {InputPer1K: 0.01, OutputPer1K: 0.03},
	"gpt-4":                      {InputPer1K: 0.03, OutputPer1K: 0.06},
	"gpt-3.5-turbo":              {InputPer1K: 0.0005, OutputPer1K: 0.0015},
	"claude-3-5-sonnet-20241022": {InputPer1K: 0.003, OutputPer1K: 0.015},
	"claude-3-5-haiku-20241022":  {InputPer1K: 0.0008, OutputPer1K: 0.004},
	"claude-3-opus-20240229":     {InputPer1K: 0.015, OutputPer1K: 0.075},
}

// llmBudget is a fully resolved cost-control configuration for one provider.
type llmBudget struct {
	PerRunUSD float64
	DailyUSD  float64

	// Prices is keyed by the model key the orchestrator will resolve for this
	// provider. It is empty only when the operator explicitly opted out of cost
	// control, in which case Enforced is false.
	Prices map[string]cost.PriceMap

	// Enforced reports whether the resulting tracker can actually cap spend.
	Enforced bool

	// Source names where the price came from, for the run log.
	Source string
}

// resolveLLMBudget produces the cost-control configuration for a provider that
// will bill against model.
//
// It FAILS CLOSED. A model this binary cannot price gets no tracker it can
// enforce, so rather than starting a run that spends real money against a limit
// that can never bind, it returns an error naming the two variables that fix
// it. An operator who genuinely wants an uncapped run says so explicitly with
// AURUMCODE_LLM_ALLOW_UNPRICED.
func resolveLLMBudget(model string) (llmBudget, error) {
	if strings.TrimSpace(model) == "" {
		return llmBudget{}, fmt.Errorf("no model name to price: %s resolved to an empty value", envLLMModel)
	}

	perRun, err := envPositiveFloat(envLLMPerRunUSD, defaultPerRunUSD)
	if err != nil {
		return llmBudget{}, err
	}

	daily, err := envPositiveFloat(envLLMDailyUSD, defaultDailyUSD)
	if err != nil {
		return llmBudget{}, err
	}

	if daily < perRun {
		return llmBudget{}, fmt.Errorf("%s ($%.4f) is below %s ($%.4f), so the per-run ceiling can never be reached",
			envLLMDailyUSD, daily, envLLMPerRunUSD, perRun)
	}

	budget := llmBudget{PerRunUSD: perRun, DailyUSD: daily}

	price, source, err := resolveModelPrice(model)
	if err != nil {
		return llmBudget{}, err
	}

	if source == "" {
		// Explicit opt-out: no price table, so the tracker enforces nothing.
		// Every completion still gets reported through UntrackedSpends.
		budget.Prices = map[string]cost.PriceMap{}
		budget.Enforced = false
		budget.Source = "unpriced (opted in via " + envLLMAllowUnpriced + ")"
		return budget, nil
	}

	budget.Prices = map[string]cost.PriceMap{model: price}
	budget.Enforced = true
	budget.Source = source

	return budget, nil
}

// resolveModelPrice returns the price for model and where it came from. A nil
// error with an empty source means the operator explicitly accepted an
// unpriced, uncapped run.
func resolveModelPrice(model string) (cost.PriceMap, string, error) {
	rawIn := strings.TrimSpace(os.Getenv(envLLMInputPer1K))
	rawOut := strings.TrimSpace(os.Getenv(envLLMOutputPer1K))

	switch {
	case rawIn != "" && rawOut != "":
		in, err := parsePrice(envLLMInputPer1K, rawIn)
		if err != nil {
			return cost.PriceMap{}, "", err
		}
		out, err := parsePrice(envLLMOutputPer1K, rawOut)
		if err != nil {
			return cost.PriceMap{}, "", err
		}
		return cost.PriceMap{InputPer1K: in, OutputPer1K: out}, "explicit " + envLLMInputPer1K + "/" + envLLMOutputPer1K, nil

	case rawIn != "" || rawOut != "":
		// Half a price is not a price: the missing side would silently be $0,
		// so half the spend would never reach the ledger.
		missing, given := envLLMOutputPer1K, envLLMInputPer1K
		if rawIn == "" {
			missing, given = envLLMInputPer1K, envLLMOutputPer1K
		}
		return cost.PriceMap{}, "", fmt.Errorf("%s is set but %s is not; set both or neither, "+
			"because a missing side prices those tokens at $0 and they never reach the ledger", given, missing)
	}

	if price, ok := catalogUSDPer1K[model]; ok {
		return price, "built-in catalog", nil
	}

	allow, err := envBool(envLLMAllowUnpriced)
	if err != nil {
		return cost.PriceMap{}, "", err
	}
	if allow {
		return cost.PriceMap{}, "", nil
	}

	return cost.PriceMap{}, "", fmt.Errorf(
		"no price for model %q, so no budget can be enforced against it: set %s and %s "+
			"(US dollars per 1000 tokens), or set %s=true to run without cost control on purpose",
		model, envLLMInputPer1K, envLLMOutputPer1K, envLLMAllowUnpriced)
}

// parsePrice fails closed on a non-positive price. $0 is not a legitimate way
// to state "this model is free": a $0 entry still marks the budget Enforced
// and still populates the price table, so the tracker computes every spend as
// exactly $0 and no ceiling it is given can ever bind - the run looks metered
// in the log while it is actually uncapped. An operator who genuinely runs a
// free or local model has an honest opt-out that says so instead of
// pretending to price it: leave both AURUMCODE_LLM_*_USD_PER_1K variables
// unset and set AURUMCODE_LLM_ALLOW_UNPRICED=true, which correctly reports
// Enforced=false rather than a live cap that can never fire.
func parsePrice(name, raw string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number in US dollars per 1000 tokens (got %q)", name, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero (got %q); a $0 price would silently disable "+
			"the budget it claims to enforce - for a genuinely free or local model, leave %s and %s unset "+
			"and set %s=true instead", name, raw, envLLMInputPer1K, envLLMOutputPer1K, envLLMAllowUnpriced)
	}
	return value, nil
}

func envPositiveFloat(name string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number in US dollars (got %q)", name, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero (got %q)", name, raw)
	}

	return value, nil
}
