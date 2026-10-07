package litellm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// errorSummaryLimit bounds the provider text an error carries, so a large
// or adversarial error body cannot balloon a log line.
const errorSummaryLimit = 300

// errorEnvelope covers the error bodies the known OpenAI-compatible
// endpoints return:
//
//	OpenAI, Azure, OpenRouter, LiteLLM: {"error":{"message":...}}
//	Anthropic:                          {"type":"error","error":{"type":...,"message":...}}
//	Google:                             {"error":{"status":...,"message":...}} or a one-element array of it
//	Bedrock:                            {"message":...}
type errorEnvelope struct {
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Status  string `json:"status"`
	} `json:"error"`
	Message string `json:"message"`
}

// apiError is the error a non-200 answer becomes: it names the provider,
// keeps the status, summarises the body to the provider's own message
// when the envelope is known, and redacts the key and every known secret
// shape before the text can reach a log, a terminal or a review.
func (p *Provider) apiError(status int, body []byte) error {
	// Redact before bounding: a cut could split a secret and hide it from
	// the filter.
	summary := redaction.NewFilter(p.apiKey).Redact(summarizeErrorBody(body))
	if len(summary) > errorSummaryLimit {
		summary = summary[:errorSummaryLimit] + "... (truncated)"
	}
	return fmt.Errorf("%s API error (status %d): %s", p.dialect.Name, status, summary)
}

// summarizeErrorBody extracts the provider's message from a known envelope,
// or falls back to the raw body, on one line.
func summarizeErrorBody(body []byte) string {
	text := string(body)
	if msg := envelopeMessage(body); msg != "" {
		text = msg
	}
	return strings.Join(strings.Fields(text), " ")
}

// envelopeMessage returns "<kind>: <message>" from a known error envelope,
// "" when the body is not one.
func envelopeMessage(body []byte) string {
	var env errorEnvelope
	if json.Unmarshal(body, &env) != nil {
		var list []errorEnvelope
		if json.Unmarshal(body, &list) != nil || len(list) == 0 {
			return ""
		}
		env = list[0]
	}
	if env.Error != nil && env.Error.Message != "" {
		kind := env.Error.Type
		if kind == "" {
			kind = env.Error.Status
		}
		if kind == "" {
			return env.Error.Message
		}
		return kind + ": " + env.Error.Message
	}
	return env.Message
}
