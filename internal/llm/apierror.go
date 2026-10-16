package llm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// APIError is an HTTP error a provider's API answered: the provider's name,
// the status, and the bounded, redacted summary of its message. Error() is
// the text every log and review already carried. TooLong is set when the
// provider refused the request because the prompt is longer than the model's
// context window, which is the one failure the reviewer answers by splitting
// the prompt instead of giving up (internal/review/split.go).
type APIError struct {
	Provider string
	Status   int
	Message  string
	TooLong  *PromptTooLong
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s API error (status %d): %s", e.Provider, e.Status, e.Message)
}

// PromptTooLong is a provider's refusal of a prompt for its length. Limit
// and Requested are the model's context window and the size of the refused
// request, in the provider's own tokens; zero when the message does not say.
type PromptTooLong struct {
	Limit     int
	Requested int
}

// promptTooLongMarkers are the phrases providers use to say the prompt is
// longer than the context window, lower case. The ones seen on the wire:
//
//	OpenAI, Azure, LiteLLM: "ContextWindowExceededError ... This model's maximum
//	                         context length is N tokens. However, your messages
//	                         resulted in M tokens." (code context_length_exceeded)
//	Anthropic:              "prompt is too long: M tokens > N maximum"
//	Google:                 "The input token count (M) exceeds the maximum number
//	                         of tokens allowed (N)."
//	Bedrock:                "Input is too long for requested model."
var promptTooLongMarkers = []string{
	"context_length_exceeded",
	"contextwindowexceeded",
	"maximum context length",
	"prompt is too long",
	"exceeds the maximum number of tokens",
	"input is too long",
	"reduce the length of the messages",
	"exceed context limit",
}

var (
	openAIOverflow    = regexp.MustCompile(`maximum context length is (\d+) tokens.{0,40}?resulted in (\d+) tokens`)
	anthropicOverflow = regexp.MustCompile(`prompt is too long: (\d+) tokens > (\d+) maximum`)
	googleOverflow    = regexp.MustCompile(`input token count \((\d+)\) exceeds the maximum number of tokens allowed \((\d+)\)`)
	// Anthropic, when the prompt plus the reply cap crosses the window:
	// "input length and `max_tokens` exceed context limit: A + B > C".
	anthropicCap = regexp.MustCompile("exceed context limit: (\\d+) \\+ (\\d+) > (\\d+)")
	// vLLM, directly or through LiteLLM (#198): "This model's maximum
	// context length is N tokens. However, you requested O output tokens and
	// your prompt contains at least M input tokens", and the older "However,
	// you requested M tokens (X in the messages, Y in the completion)".
	vllmOverflow      = regexp.MustCompile(`maximum context length is (\d+) tokens.{0,80}?you requested (\d+) output tokens.{0,60}?prompt contains (?:at least )?(\d+) input tokens`)
	vllmTotalOverflow = regexp.MustCompile(`maximum context length is (\d+) tokens.{0,40}?you requested (\d+) tokens`)
	// OpenAI Responses, through a LiteLLM proxy, when one input text is
	// longer than the API takes, in characters: "Invalid
	// 'input[0].content[0].text': string too long. Expected a string with
	// maximum length N, but got a string with length M instead."
	stringTooLong = regexp.MustCompile(`string too long.{0,80}?maximum length (\d+)(?:.{0,40}?with length (\d+))?`)
	// Anthropic, when this request alone is larger than the organization's
	// input tokens per minute: "This request would exceed your rate limit of
	// N input tokens per minute". A request larger than N is never accepted,
	// however long it waits, so it is a refusal for length with window N.
	perMinuteWindow = regexp.MustCompile(`request would exceed (?:your|the) rate limit of ([\d,]+) input tokens per minute`)
)

// charsPerToken is the product's own estimate of characters per token
// (internal/prompt/estimator.go), used to read a limit given in characters.
const charsPerToken = 4

// DetectPromptTooLong classifies a non-200 answer: it is a refusal for length
// when the status is 413 (the request is too large; a gateway says no more
// than that) or 400 with a message that says the prompt is longer than the
// context window. text is the redacted summary of the provider's message,
// before it is bounded for a log line, so the sizes are still in it.
//
// Two more answers are refusals for length with a window the message states:
// a 400 saying one input string is longer than the API takes (the limit is
// in characters and is converted to tokens with the product's estimate), and
// a 429 saying this request would exceed the input tokens per minute allowed
// (a plain 429, without that text, stays a transient error). The same 429
// text also comes when the request only exceeds what is left of the minute,
// so the provider that holds the request confirms it against the request's
// own size before treating it as a refusal for length
// (internal/llm/provider/litellm/retry.go, confirmSize).
func DetectPromptTooLong(status int, text string) *PromptTooLong {
	lower := strings.ToLower(text)
	if status == 429 {
		if m := perMinuteWindow.FindStringSubmatch(lower); m != nil {
			return &PromptTooLong{Limit: atoi(strings.ReplaceAll(m[1], ",", ""))}
		}
		return nil
	}
	if status != 400 && status != 413 {
		return nil
	}
	if m := stringTooLong.FindStringSubmatch(lower); m != nil {
		return &PromptTooLong{Limit: atoi(m[1]) / charsPerToken, Requested: atoi(m[2]) / charsPerToken}
	}
	marked := status == 413
	for _, marker := range promptTooLongMarkers {
		if strings.Contains(lower, marker) {
			marked = true
			break
		}
	}
	if !marked {
		return nil
	}
	refusal := &PromptTooLong{}
	if m := openAIOverflow.FindStringSubmatch(lower); m != nil {
		refusal.Limit, refusal.Requested = atoi(m[1]), atoi(m[2])
	} else if m := anthropicOverflow.FindStringSubmatch(lower); m != nil {
		refusal.Requested, refusal.Limit = atoi(m[1]), atoi(m[2])
	} else if m := googleOverflow.FindStringSubmatch(lower); m != nil {
		refusal.Requested, refusal.Limit = atoi(m[1]), atoi(m[2])
	} else if m := anthropicCap.FindStringSubmatch(lower); m != nil {
		refusal.Requested, refusal.Limit = atoi(m[1])+atoi(m[2]), atoi(m[3])
	} else if m := vllmOverflow.FindStringSubmatch(lower); m != nil {
		refusal.Limit, refusal.Requested = atoi(m[1]), atoi(m[3])+atoi(m[2])
	} else if m := vllmTotalOverflow.FindStringSubmatch(lower); m != nil {
		refusal.Limit, refusal.Requested = atoi(m[1]), atoi(m[2])
	}
	return refusal
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// AsPromptTooLong reports whether err, or any error it wraps (a fallback
// chain wraps one error per provider), is a provider refusing the prompt for
// its length. With several refusals it returns the one with the smallest known
// window.
func AsPromptTooLong(err error) (PromptTooLong, bool) {
	var found PromptTooLong
	ok := false
	walkErrors(err, func(e error) {
		api, is := e.(*APIError)
		if !is || api.TooLong == nil {
			return
		}
		if !ok || (api.TooLong.Limit > 0 && (found.Limit == 0 || api.TooLong.Limit < found.Limit)) {
			found, ok = *api.TooLong, true
		}
	})
	return found, ok
}

// walkErrors visits err and every error in its wrap tree, depth first.
func walkErrors(err error, visit func(error)) {
	if err == nil {
		return
	}
	visit(err)
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		walkErrors(u.Unwrap(), visit)
	case interface{ Unwrap() []error }:
		for _, wrapped := range u.Unwrap() {
			walkErrors(wrapped, visit)
		}
	}
}
