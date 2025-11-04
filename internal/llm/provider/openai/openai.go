package openai

import (
	"context"
	"fmt"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/httpbase"
	"net/http"
)

// Provider is the OpenAI LLM provider
type Provider struct {
	baseURL string
	apiKey  string
	client  *httpbase.Client
}

// NewProvider creates a new OpenAI provider
func NewProvider(apiKey string) *Provider {
	return &Provider{
		baseURL: "https://api.openai.com/v1",
		apiKey:  apiKey,
		client:  httpbase.NewClient("https://api.openai.com/v1"),
	}
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "openai"
}

// Complete sends a completion request to OpenAI
func (p *Provider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	model := opts.ModelKey
	if model == "" {
		model = "gpt-4"
	}

	req := &httpbase.Request{
		Method: http.MethodPost,
		Path:   "/chat/completions",
		Headers: map[string]string{
			"Authorization": "Bearer " + p.apiKey,
		},
		Body: map[string]interface{}{
			"model":       model,
			"messages":    []map[string]string{{"role": "user", "content": prompt}},
			"temperature": opts.Temperature,
			"max_tokens":  opts.MaxTokens,
		},
	}

	ctx := context.Background()
	resp, err := p.client.Do(ctx, req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai request failed: %w", err)
	}

	var openaiResp struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	err = httpbase.DecodeJSON(resp, &openaiResp)
	if err != nil {
		return llm.Response{}, fmt.Errorf("failed to decode openai response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("no choices in openai response")
	}

	return llm.Response{
		Text:         openaiResp.Choices[0].Message.Content,
		TokensIn:     openaiResp.Usage.PromptTokens,
		TokensOut:    openaiResp.Usage.CompletionTokens,
		Model:        model,
		FinishReason: openaiResp.Choices[0].FinishReason,
	}, nil
}

// Tokens estimates token count using OpenAI's API
func (p *Provider) Tokens(input string) (int, error) {
	// Simple heuristic approximation
	// OpenAI's tiktoken library uses roughly 4 chars per token for English text
	return len(input) / 4, nil
}
