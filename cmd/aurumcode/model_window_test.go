package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
)

// #198: the environment overriding a different config window is said once,
// whatever the number of passes that ask for the window.
func TestModelWindowEnvOverrideIsSaidOnce(t *testing.T) {
	cfg := &config.Config{}
	cfg.Review.ModelWindowTokens = 128000
	env := reviewEnv{modelWindowTokens: "262144"}
	var stderr strings.Builder
	s := &reviewState{stderr: &stderr, cfg: cfg, deps: reviewDeps{env: &env}}
	for i := 0; i < 3; i++ {
		if got := s.modelWindowTokens(); got != 262144 {
			t.Fatalf("window = %d, want the environment's", got)
		}
		s.declareModelWindow(review.NewReviewer(nil, review.DefaultConfig()))
	}
	want := "AURUMCODE_MODEL_WINDOW_TOKENS (262144) overrides review.model_window_tokens (128000)"
	if got := strings.Count(stderr.String(), want); got != 1 {
		t.Fatalf("override notice written %d times, want once: %q", got, stderr.String())
	}
}

// The same value in both places, or only one of them set, is no override.
func TestModelWindowWithoutOverrideSaysNothing(t *testing.T) {
	for _, c := range []struct {
		cfg int
		env string
	}{{128000, "128000"}, {0, "262144"}, {128000, ""}, {128000, "x"}} {
		cfg := &config.Config{}
		cfg.Review.ModelWindowTokens = c.cfg
		var stderr strings.Builder
		noteWindowOverride(&stderr, cfg, reviewEnv{modelWindowTokens: c.env})
		if stderr.Len() != 0 {
			t.Fatalf("cfg %d env %q: %q", c.cfg, c.env, stderr.String())
		}
	}
}
