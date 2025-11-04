package anthropic

import (
	"context"
	"fmt"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/httpbase"
	"net/http"
)

// Provider is the Anthropic Claude LLM provider
type Provider struct {
	baseURL string
	apiKey  string
	client  *httpbase.Client
}

// NewProvider creates a new Anthropic provider
func NewProvider(apiKey string) *Provider {
	return &Provider{
		baseURL: "https://api.anthropic.com/v1",
		apiKey:  apiKey,
		client:  httpbase.NewClient("https://api.anthropic.com/v1"),
	}
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "anthropic"
}

// Complete sends a completion request to Anthropic
func (p *Provider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	model := opts.ModelKey
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	req := &httpbase.Request{
		Method: http.MethodPost,
		Path:   "/messages",
		Headers: map[string]string{
			"x-api-key":         p.apiKey,
			"anthropic-version": "2023-06-01",
		},
		Body: map[string]interface{}{
			"model":       model,
			"max_tokens":  opts.MaxTokens,
			"messages":    []map[string]string{{"role": "user", "content": prompt}},
			"temperature": opts.Temperature,
		},
	}

	ctx := context.Background()
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic request failed: %w", err)
	}

	var anthropicResp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	err = httpbase.DecodeJSON(resp, &anthropicResp)
	if err != nil {
		return llm.Response{}, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return llm.Response{}, fmt.Errorf("no content in anthropic response")
	}

	return llm.Response{
		Text:      anthropicResp.Content[0].Text,
		TokensIn:  anthropicResp.Usage.InputTokens,
		TokensOut: anthropicResp.Usage.OutputTokens,
		Model:     model,
	}, nil
}

// Tokens estimates token count
func (p *Provider) Tokens(input string) (int, error) {
	// Claude uses roughly 4 chars per token for English
	return len(input) / 4, nil
}
