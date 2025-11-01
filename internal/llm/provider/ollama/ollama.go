package ollama

import (
	"aurumcode/internal/llm"
	"aurumcode/internal/llm/httpbase"
	"context"
	"fmt"
	"net/http"
)

// Provider is the Ollama LLM provider
type Provider struct {
	baseURL string
	client  *httpbase.Client
}

// NewProvider creates a new Ollama provider
func NewProvider(baseURL string) *Provider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Provider{
		baseURL: baseURL,
		client:  httpbase.NewClient(baseURL),
	}
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "ollama"
}

// Complete sends a completion request to Ollama
func (p *Provider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	model := opts.ModelKey
	if model == "" {
		model = "llama3"
	}
	
	req := &httpbase.Request{
		Method: http.MethodPost,
		Path:   "/api/generate",
		Body: map[string]interface{}{
			"model":       model,
			"prompt":      prompt,
			"temperature": opts.Temperature,
			"num_predict": opts.MaxTokens,
		},
	}
	
	ctx := context.Background()
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("ollama request failed: %w", err)
	}
	
	var ollamaResp struct {
		Response           string `json:"response"`
		PromptEvalCount    int    `json:"prompt_eval_count"`
		EvalCount          int    `json:"eval_count"`
	}
	
	err = httpbase.DecodeJSON(resp, &ollamaResp)
	if err != nil {
		return llm.Response{}, fmt.Errorf("failed to decode ollama response: %w", err)
	}
	
	return llm.Response{
		Text:      ollamaResp.Response,
		TokensIn:  ollamaResp.PromptEvalCount,
		TokensOut: ollamaResp.EvalCount,
		Model:     model,
	}, nil
}

// Tokens estimates token count
func (p *Provider) Tokens(input string) (int, error) {
	// Ollama models use roughly 4 chars per token
	return len(input) / 4, nil
}

