package config

import (
	"strings"
	"testing"
)

// #198: the model's window and the codebase context's ceiling are declared
// in config.yml; negative values are refused.
func TestModelWindowAndContextCeilingParse(t *testing.T) {
	cfg, err := Parse([]byte("review:\n  model_window_tokens: 262144\n  context:\n    max_tokens: 40000\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Review.ModelWindowTokens != 262144 || cfg.Review.Context.MaxTokens != 40000 {
		t.Fatalf("got window=%d context=%d", cfg.Review.ModelWindowTokens, cfg.Review.Context.MaxTokens)
	}
	for _, bad := range []string{"review:\n  model_window_tokens: -1\n", "review:\n  context:\n    max_tokens: -5\n"} {
		if _, err := Parse([]byte(bad), ""); err == nil || !strings.Contains(err.Error(), "must not be negative") {
			t.Fatalf("%q: want a negative refusal, got %v", bad, err)
		}
	}
}
