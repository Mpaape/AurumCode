package litellm

import (
	"aurumcode/internal/llm"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Provider implements LiteLLM proxy provider (OpenAI-compatible)
type Provider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewProvider creates a new LiteLLM provider
func NewProvider(apiKey, baseURL, model string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type completionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Complete sends a completion request to LiteLLM
func (p *Provider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	// Build request
	reqBody := completionRequest{
		Model: p.model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	}

	// Add system message if provided
	if opts.System != "" {
		reqBody.Messages = []message{
			{Role: "system", Content: opts.System},
			{Role: "user", Content: prompt},
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return llm.Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := p.baseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return llm.Response{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return llm.Response{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf("LiteLLM API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var completion completionResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return llm.Response{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(completion.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("no choices in response")
	}

	return llm.Response{
		Text:      completion.Choices[0].Message.Content,
		TokensIn:  completion.Usage.PromptTokens,
		TokensOut: completion.Usage.CompletionTokens,
		Model:     completion.Model,
	}, nil
}

// Tokens estimates token count (approximate)
func (p *Provider) Tokens(input string) (int, error) {
	// Rough approximation: 1 token ≈ 4 characters
	return len(input) / 4, nil
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "litellm"
}
