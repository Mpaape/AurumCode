package llm

// Options represents LLM request options
type Options struct {
	System      string            `json:"system,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ModelKey    string            `json:"model_key,omitempty"`
}

// Response represents an LLM response
type Response struct {
	Text       string                 `json:"text"`
	TokensIn   int                    `json:"tokens_in"`
	TokensOut  int                    `json:"tokens_out"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
	Model      string                 `json:"model,omitempty"`
	FinishReason string               `json:"finish_reason,omitempty"`
}

// Provider defines the interface for LLM providers
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

