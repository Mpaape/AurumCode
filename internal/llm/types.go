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

// DefaultOptions returns sensible defaults for LLM options
func DefaultOptions() Options {
	return Options{
		Temperature: 0.3,
		MaxTokens:   4000,
	}
}
