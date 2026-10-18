// Cost control for `aurumcode review`: --limite is a hard USD ceiling on what
// one run may spend calling the model; a call that would cross it is refused
// before it is made, and the estimated and actual spend are reported.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	// envCostInputPer1K / envCostOutputPer1K override the flat per-1000-token
	// price this command converts tokens into dollars with. The names mirror
	// (AURUMCODE_LLM_*_USD_PER_1K), so an operator can price the configured
	// model without coupling this command to a vendor.
	envCostInputPer1K  = "AURUMCODE_LLM_INPUT_USD_PER_1K"
	envCostOutputPer1K = "AURUMCODE_LLM_OUTPUT_USD_PER_1K"
)

// defaultInputPer1KUSD / defaultOutputPer1KUSD price a review when the two
// variables above are not set: a conservative default (docs/specs/AUR-433.md)
// in the range of a mid-tier hosted model's published rate, so --limite is
// enforceable out of the box without first requiring the operator to price
// whatever model they configured.
const (
	defaultInputPer1KUSD  = 0.01
	defaultOutputPer1KUSD = 0.03
)

// defaultCostModelKey is the model key cost tracking uses when --modelo was
// not given, matching the card's own example: `aurumcode review --base
// HEAD~1 --limite 0.50`.
const defaultCostModelKey = "default"

// parseLimiteUSD parses --limite's value. It fails closed: unparsable or
// non-positive input is a usage error, never a silently-disabled limit.
func parseLimiteUSD(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("--limite: must be a number of US dollars (got %q)", raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("--limite: must be greater than zero (got %q)", raw)
	}
	return value, nil
}

// costPrice resolves the flat per-1000-token price this run charges: the
// configured override when both halves are set and valid, the documented
// default when neither is set, and a usage error when only one half is set
// -- half a price is not a price, because the missing side would silently
// bill those tokens at $0.
func costPrice() (cost.PriceMap, error) {
	rawIn := strings.TrimSpace(os.Getenv(envCostInputPer1K))
	rawOut := strings.TrimSpace(os.Getenv(envCostOutputPer1K))

	switch {
	case rawIn == "" && rawOut == "":
		return cost.PriceMap{InputPer1K: defaultInputPer1KUSD, OutputPer1K: defaultOutputPer1KUSD}, nil

	case rawIn != "" && rawOut != "":
		in, errIn := strconv.ParseFloat(rawIn, 64)
		if errIn != nil || in <= 0 {
			return cost.PriceMap{}, fmt.Errorf("%s must be a positive number of US dollars per 1000 tokens (got %q)", envCostInputPer1K, rawIn)
		}
		out, errOut := strconv.ParseFloat(rawOut, 64)
		if errOut != nil || out <= 0 {
			return cost.PriceMap{}, fmt.Errorf("%s must be a positive number of US dollars per 1000 tokens (got %q)", envCostOutputPer1K, rawOut)
		}
		return cost.PriceMap{InputPer1K: in, OutputPer1K: out}, nil

	default:
		missing, given := envCostOutputPer1K, envCostInputPer1K
		if rawIn == "" {
			missing, given = envCostInputPer1K, envCostOutputPer1K
		}
		return cost.PriceMap{}, fmt.Errorf("%s is set but %s is not; set both or neither, "+
			"because a missing side prices those tokens at $0", given, missing)
	}
}

// costModelKey is the single model key --limite's price map, the tracker,
// and fixedModelProvider all key cost tracking against: the chosen
// --modelo name, or defaultCostModelKey without it.
func costModelKey(modelo string) string {
	if modelo != "" {
		return modelo
	}
	return defaultCostModelKey
}

// fixedModelProvider forces the model key the cost tracker enforces
// against, regardless of what the wrapped provider would otherwise
// resolve.
//
// llm.ResolveModelKey prefers whatever the provider itself reports (see
// llm.ModelResolver): the offline fixture provider (review.FakeProvider)
// reports nothing, which resolves to an empty key with no price entry, and
// cost.Tracker.Reserve fails closed on any key its price table does not
// contain -- so without this, every --limite run would refuse regardless
// of the chosen limit, offline or not. Pinning the key here keeps the
// ceiling and the eventual charge on the same, always-priced key.
//
// The embedded field is the llm.Provider INTERFACE, not a concrete
// provider type: ResolveModel is not part of that interface, so it is
// never promoted through the embedding, and this type's own ResolveModel
// below is the only one llm.ResolveModelKey ever sees -- there is no
// shadowing ambiguity with a concrete provider (e.g. litellm.Provider)
// that happens to implement llm.ModelResolver itself.
type fixedModelProvider struct {
	llm.Provider
	model string
}

var _ llm.ModelResolver = (*fixedModelProvider)(nil)

// Unwrap implements llm.Unwrapper: the decorator only names the model the
// cost is priced for and forwards every request unchanged, so a capability
// of the provider behind it (separate system/user messages) stays visible.
func (f *fixedModelProvider) Unwrap() llm.Provider { return f.Provider }

// ResolveModel implements llm.ModelResolver.
func (f *fixedModelProvider) ResolveModel(llm.Options) string {
	return f.model
}

// buildCostTracker is the one place --limite's parsed value reaches
// enforcement: both the per-run and the daily ceiling cost.NewTracker is
// given are this exact value, and the price map has exactly one entry, for
// the one key fixedModelProvider forces every request onto. A fresh
// tracker is built for this process invocation alone, so the daily ceiling
// never independently binds -- nothing was spent against it before this
// run started.
func buildCostTracker(limiteUSD float64, modelKey string, price cost.PriceMap) *cost.Tracker {
	ceilingUSD := limiteUSD
	return cost.NewTracker(ceilingUSD, ceilingUSD, map[string]cost.PriceMap{modelKey: price})
}

// realCostUSD reads the actual dollars committed so far from the tracker's
// own ledger. A fresh tracker starts with nothing spent, so the ceiling
// minus what Remaining() now reports IS the accumulated real cost -- no
// separate bookkeeping is needed beyond the tracker's own exported method.
func realCostUSD(tracker *cost.Tracker, limiteUSD float64) float64 {
	remaining, _ := tracker.Remaining()
	return limiteUSD - remaining
}

// estimateCostUSD prices the AUR-433 pre-flight: tokensIn is the size of
// the prompt the review will send (review.Reviewer.EstimatePromptTokens:
// instructions, policy, codebase context, evidence and diff, assembled by
// the review's own builder, #198), tokensOut the orchestrator's bounded
// planning estimate when the provider owns the actual response limit. It is
// an estimate for the optional financial gate, never a client-side
// truncation, and not the number the budget decision is made against: that
// check happens inside internal/llm.Orchestrator.Complete, request by
// request, and a refusal that makes the review split or retry is not
// foreseen here.
func estimateCostUSD(tokensIn int, price cost.PriceMap) float64 {
	cfg := review.DefaultConfig()
	tokensOut := cfg.MaxTokens - cfg.ReserveReply
	if tokensOut <= 0 {
		tokensOut = llm.DefaultOutputTokenEstimate()
	}
	return priceUSD(tokensIn, tokensOut, price)
}

func priceUSD(tokensIn, tokensOut int, price cost.PriceMap) float64 {
	return (float64(tokensIn)/1000.0)*price.InputPer1K + (float64(tokensOut)/1000.0)*price.OutputPer1K
}

// printCostEstimate reports the AUR-433 pre-flight estimate on stderr,
// before the model is called, so stdout stays byte-identical with and
// without --limite exactly like the --modelo and --fail-on notes. It goes
// through the same redacted stderr writer runReview already holds, not a
// new sink.
//
// The figure prices the prompt the review assembles (estimateCostUSD,
// #198), with the tokens it counted. The enforced check inside
// internal/llm.Orchestrator.Complete prices each request as it is sent, so
// a refusal that makes the review split, or the passes of several analysts,
// can still take it past this figure (see reportBudgetExceeded): the line is
// never a promise of what the enforced check compares against --limite.
func printCostEstimate(stderr io.Writer, estimatedUSD float64, promptTokens int, limiteUSD float64) {
	fmt.Fprintf(stderr, "aurumcode review: estimated cost $%.4f, pre-flight of the assembled prompt (about %d tokens: instructions, policy, context and diff) (--limite $%.4f)\n", estimatedUSD, promptTokens, limiteUSD)
}

// preflight prints the pre-flight estimate of the review reviewer is about
// to run, when --limite set a price: the prompt counted by the review's own
// builder.
func (s *reviewState) preflight(reviewer *review.Reviewer, diff *types.Diff, reviewContext review.ReviewContext) {
	if s.costPrice == nil || reviewer == nil {
		return
	}
	tokens := reviewer.EstimatePromptTokens(diff, reviewContext)
	printCostEstimate(s.stderr, estimateCostUSD(tokens, *s.costPrice), tokens, s.limiteUSD)
}

// printRealCost reports the AUR-433 actual cost after a successful review,
// derived from the tracker's own ledger, so no unexported package
// internals are needed to know it.
func printRealCost(stderr io.Writer, realUSD, limiteUSD float64) {
	fmt.Fprintf(stderr, "aurumcode review: actual cost $%.4f (--limite $%.4f)\n", realUSD, limiteUSD)
}

// reportBudgetExceeded prints AUR-433's refusal message and returns the
// command's exit code, 1 -- "the review itself failed", the same code
// every other review-could-not-run path in this command already uses. The
// run never reached the model: cost.Tracker.Reserve refuses inside
// internal/llm.Orchestrator.Complete before provider.Complete is ever
// called, so nothing was spent and there is no actual cost to report, only
// the pre-flight estimate printCostEstimate already printed.
//
// The message deliberately does NOT attribute the refusal to
// printCostEstimate's printed number: that figure prices the diff alone
// and can be smaller than --limite even on a run this command correctly
// refuses, because the enforced check prices the larger, fully assembled
// prompt (see printCostEstimate and estimateCostUSD). Saying "the request"
// rather than repeating a dollar amount keeps the two lines from ever
// contradicting each other.
func reportBudgetExceeded(stderr io.Writer, limiteUSD float64, cause error) int {
	fmt.Fprintf(stderr, "aurumcode review: refusing to call the model: the request's cost would exceed --limite $%.4f; nothing was spent (%v)\n", limiteUSD, cause)
	return 1
}
