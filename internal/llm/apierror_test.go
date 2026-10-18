package llm

import (
	"errors"
	"fmt"
	"testing"
)

// The refusals below are the texts the providers answered on 2026-10-14 to a
// request far past the model's window, through the chain the CI uses.
func TestDetectPromptTooLongOnWhatProvidersAnswer(t *testing.T) {
	cases := []struct {
		name   string
		status int
		text   string
		want   *PromptTooLong
	}{
		{"litellm over openai", 400, "invalid_request_error: litellm.ContextWindowExceededError: litellm.BadRequestError: ContextWindowExceededError: OpenAIException - This model's maximum context length is 128000 tokens. However, your messages resulted in 1258937 tokens. Please reduce the length of the messages. model=gpt-4o-mini.", &PromptTooLong{Limit: 128000, Requested: 1258937}},
		{"anthropic", 400, "invalid_request_error: prompt is too long: 1946751 tokens > 1000000 maximum", &PromptTooLong{Limit: 1000000, Requested: 1946751}},
		{"google", 400, "INVALID_ARGUMENT: The input token count (1200000) exceeds the maximum number of tokens allowed (1048576).", &PromptTooLong{Limit: 1048576, Requested: 1200000}},
		{"anthropic, reply cap crosses the window", 400, "invalid_request_error: input length and `max_tokens` exceed context limit: 998000 + 8192 > 1000000, decrease input length or `max_tokens` and try again", &PromptTooLong{Limit: 1000000, Requested: 1006192}},
		{"a gateway that only says too large", 413, "Request Entity Too Large", &PromptTooLong{}},
		{"bedrock, sizes unsaid", 400, "ValidationException: Input is too long for requested model.", &PromptTooLong{}},
		{"too large, sizes unsaid", 413, "context_length_exceeded", &PromptTooLong{}},
		{"another bad request", 400, "invalid_request_error: max_tokens is too large: 9000000", nil},
		{"bad key", 401, "invalid_api_key: prompt is too long", nil},
		{"rate limit", 429, "rate_limit_exceeded: too many requests", nil},
		{"openai responses through litellm, limit in characters", 400, "invalid_request_error: litellm.BadRequestError: OpenAIException - Invalid 'input[0].content[0].text': string too long. Expected a string with maximum length 10485760, but got a string with length 15213664 instead.", &PromptTooLong{Limit: 2621440, Requested: 3803416}},
		{"anthropic, this request alone exceeds the tokens per minute", 429, "rate_limit_error: This request would exceed your rate limit of 2,000,000 input tokens per minute (org: 00000000-0000-0000-0000-000000000000, model: claude-haiku-5-5). For details, refer to: https://docs.claude.com/en/api/rate-limits.", &PromptTooLong{Limit: 2000000}},
		{"vllm through litellm, output and input counted apart (#198)", 400, "BadRequestError: litellm.BadRequestError: OpenAIException - This model's maximum context length is 262144 tokens. However, you requested 5 output tokens and your prompt contains at least 554103 input tokens, for a total of at least 554108 tokens. Please reduce the length of the input prompt or the number of requested output tokens.", &PromptTooLong{Limit: 262144, Requested: 554108}},
		{"vllm, total requested", 400, "This model's maximum context length is 32768 tokens. However, you requested 40000 tokens (39000 in the messages, 1000 in the completion). Please reduce the length of the messages or completion.", &PromptTooLong{Limit: 32768, Requested: 40000}},
		{"anthropic, an ordinary rate limit", 429, "rate_limit_error: Number of request tokens has exceeded your per-minute rate limit (org: 00000000-0000-0000-0000-000000000000, model: claude-haiku-5-5).", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetectPromptTooLong(c.status, c.text)
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("DetectPromptTooLong(%d, %q) = %+v, want %+v", c.status, c.text, got, c.want)
			}
		})
	}
}

func TestAPIErrorKeepsTheTextEveryLogAlreadyCarried(t *testing.T) {
	err := &APIError{Provider: "litellm", Status: 400, Message: "invalid_request_error: boom"}
	if got, want := err.Error(), "litellm API error (status 400): invalid_request_error: boom"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

// A fallback chain wraps one error per provider; the refusal is found
// whichever position it has, and with several the smallest window wins.
func TestAsPromptTooLongFindsTheRefusalInAFallbackChain(t *testing.T) {
	timeout := &APIError{Provider: "a", Status: 504, Message: "gateway timeout"}
	small := &APIError{Provider: "b", Status: 400, Message: "x", TooLong: &PromptTooLong{Limit: 128000, Requested: 400000}}
	large := &APIError{Provider: "c", Status: 400, Message: "y", TooLong: &PromptTooLong{Limit: 1000000, Requested: 1200000}}
	chain := fmt.Errorf("LLM request failed: %w", errors.Join(errors.New("all providers failed"), timeout, large, small))

	got, ok := AsPromptTooLong(chain)
	if !ok || got.Limit != 128000 || got.Requested != 400000 {
		t.Fatalf("AsPromptTooLong = %+v, %v; want the 128000-token refusal", got, ok)
	}
	if _, ok := AsPromptTooLong(fmt.Errorf("failed: %w", timeout)); ok {
		t.Fatal("a timeout was taken for a refusal for length")
	}
	if _, ok := AsPromptTooLong(nil); ok {
		t.Fatal("nil was taken for a refusal for length")
	}
}
