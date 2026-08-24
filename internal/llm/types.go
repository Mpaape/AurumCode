package llm

// Options represents LLM request options. Every field here must be carried by
// a provider; a field no provider sends is a silent no-op for the caller.
type Options struct {
	System      string  `json:"system,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	ModelKey    string  `json:"model_key,omitempty"`
}

// Response represents an LLM response
type Response struct {
	Text         string `json:"text"`
	TokensIn     int    `json:"tokens_in"`
	TokensOut    int    `json:"tokens_out"`
	Model        string `json:"model,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// Provider defines the interface for LLM providers.
//
// Complete takes no context: cancellation cannot reach the provider's
// transport, so the orchestrator can only abandon a slow call, not stop it.
type Provider interface {
	Complete(prompt string, opts Options) (Response, error)
	Tokens(input string) (int, error)
	Name() string
}

// ModelResolver is implemented by providers that can name, before the call is
// made, the concrete model they will bill against for a given set of options.
//
// This exists because Options.ModelKey is routinely empty - DefaultOptions
// never sets it - while every provider here substitutes a model of its own
// ("gpt-4", "llama3", a configured LiteLLM model). Without a pre-call answer
// the ceiling is checked against a key no price table contains while the charge
// lands on a different one, so the ceiling reads a ledger that never moves.
type ModelResolver interface {
	ResolveModel(opts Options) string
}

// ResolveModelKey returns the single model key that both the budget ceiling and
// the charge must be accounted against for this request.
//
// Preference order: what the provider says it will actually send, then the
// caller's explicit ModelKey. The result is empty only when neither is known,
// and a configured tracker refuses an empty key rather than admitting spend it
// cannot price.
//
// It deliberately never substitutes a placeholder such as "default": a
// configured price table has no entry for that, so the request would be refused
// while the provider bills a real model.
func ResolveModelKey(provider Provider, opts Options) string {
	if r, ok := provider.(ModelResolver); ok {
		if key := r.ResolveModel(opts); key != "" {
			return key
		}
	}
	return opts.ModelKey
}

// DefaultOptions returns sensible defaults for LLM options.
//
// Temperature is left at its zero value, deliberately (AUR-460): a fixed
// 0.3 default meant every request carried an explicit sampling parameter,
// and a growing family of gateway-served models (measured: gpt-5.6-luna,
// -sol, -terra) 400s on ANY explicit "temperature" value, including 0 and
// including that model's own advertised default. A caller that wants a
// specific temperature still sets Options.Temperature itself; the field
// stays on the struct for that. What no longer happens is this package
// picking a non-zero value nobody asked for and every provider dutifully
// forwarding it upstream.
func DefaultOptions() Options {
	return Options{
		MaxTokens: 4000,
	}
}
