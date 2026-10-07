package litellm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Mpaape/AurumCode/internal/llm"
	"io"
	"net/http"
)

// Provider implements LiteLLM proxy provider (OpenAI-compatible)
type Provider struct {
	apiKey  string
	baseURL string
	model   string
	dialect Dialect
	client  *http.Client
}

// NewProvider creates a new LiteLLM provider. The client timeout comes from
// llm.ProviderTimeout(), the single source of truth this package shares with
// the orchestrator's own context deadline (see its doc comment).
func NewProvider(apiKey, baseURL, model string) *Provider {
	return NewProviderWithDialect(DefaultDialect(), apiKey, baseURL, model)
}

// NewProviderWithDialect creates a provider that speaks dialect: one
// OpenAI-compatible engine, configured per provider by the catalog.
func NewProviderWithDialect(dialect Dialect, apiKey, baseURL, model string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		dialect: dialect,
		client: &http.Client{
			Timeout: llm.ProviderTimeout(),
		},
	}
}

// completionRequest carries no temperature field, deliberately (AUR-460).
// The gateway this provider talks to fans out to whatever models the
// operator has configured behind it, and a growing family of them (the
// measured case: gpt-5.6-luna/sol/terra) rejects ANY explicit
// "temperature" -- including the provider's own advertised default and
// including 0 -- with a 400: "Unsupported value: 'temperature' does not
// support <n> with this model. Only the default (1) value is supported."
// There is no per-model compatibility table here on purpose (the card
// forbids inventing one): the only value guaranteed to work across the
// whole fleet is the key being absent from the JSON entirely, so the
// upstream provider applies whatever default it wants. If a model still
// rejects the call for some other reason, that is surfaced verbatim as the
// provider's own error message, not papered over here.
type completionRequest struct {
	Model     string    `json:"model,omitempty"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	// MaxCompletionTokens replaces MaxTokens for a dialect whose models
	// refuse max_tokens; at most one of the two is ever set.
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	ResponseFormat      *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *jsonSchemaFormat `json:"json_schema,omitempty"`
}

// jsonSchemaFormat is the structured-output form of response_format: the
// answer must follow Schema.
type jsonSchemaFormat struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

// answerFormat is the response_format a tool-conversation request carries:
// the caller's JSON Schema when it gave one and the dialect honours it, the
// bare JSON object mode when it only asked for JSON (or the dialect takes
// no schema), nothing when the dialect takes no response_format.
func (d Dialect) answerFormat(opts llm.Options) *responseFormat {
	if d.StructuredOutput != StructuredJSONSchema {
		return d.jsonModeFormat(opts.JSONMode || len(opts.ResponseSchema) > 0)
	}
	if len(opts.ResponseSchema) > 0 {
		return &responseFormat{Type: "json_schema", JSONSchema: &jsonSchemaFormat{Name: opts.ResponseSchemaNameOrDefault(), Schema: opts.ResponseSchema}}
	}
	if opts.JSONMode {
		return &responseFormat{Type: "json_object"}
	}
	return nil
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

// ResolveModel reports the model this provider will actually send, before the
// call is made, so the cost tracker can cap and charge on the same key.
// Implements llm.ModelResolver.
//
// This provider deliberately ignores opts.ModelKey: the model is fixed at
// construction time and that is what goes upstream, so that is what gets billed.
func (p *Provider) ResolveModel(opts llm.Options) string {
	return p.model
}

// Complete sends a completion request to LiteLLM
func (p *Provider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	// Build request
	// opts.Temperature is deliberately not read here: see completionRequest.
	reqBody := completionRequest{
		Model: p.ResolveModel(opts),
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		ResponseFormat: p.dialect.jsonModeFormat(opts.JSONMode),
	}
	reqBody.MaxTokens, reqBody.MaxCompletionTokens = p.dialect.replyCap(opts.MaxTokens)

	// Add system message if provided
	if opts.System != "" {
		reqBody.Messages = []message{
			{Role: "system", Content: opts.System},
			{Role: "user", Content: prompt},
		}
	}

	body, err := p.post(reqBody)
	if err != nil {
		return llm.Response{}, err
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
		Text:         completion.Choices[0].Message.Content,
		TokensIn:     completion.Usage.PromptTokens,
		TokensOut:    completion.Usage.CompletionTokens,
		Model:        completion.Model,
		FinishReason: completion.Choices[0].FinishReason,
	}, nil
}

// post sends one chat-completions request and returns the raw body of a 200
// response. Every request this provider makes goes through it.
func (p *Provider) post(reqBody any) ([]byte, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := p.dialect.requestURL(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid base URL: %w", p.dialect.Name, err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	p.dialect.authorize(req, p.apiKey)

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, p.apiError(resp.StatusCode, body)
	}
	return body, nil
}

// Tokens estimates token count (approximate)
func (p *Provider) Tokens(input string) (int, error) {
	// Rough approximation: 1 token â‰ˆ 4 characters
	return len(input) / 4, nil
}

// BaseURL is the endpoint this provider calls (never the key): two
// endpoints serving the same model name are distinct answering entities.
func (p *Provider) BaseURL() string {
	return p.baseURL
}

// Name returns the provider name
func (p *Provider) Name() string {
	return p.dialect.Name
}
