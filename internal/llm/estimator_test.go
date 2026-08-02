package llm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type tokenlessProvider struct{}

func (tokenlessProvider) Complete(string, Options) (Response, error) {
	return Response{}, errors.New("not used")
}
func (tokenlessProvider) Tokens(string) (int, error) { return 0, errors.New("no tokenizer") }
func (tokenlessProvider) Name() string               { return "tokenless" }

// TestEstimateDoesNotPoisonSharedCache: Estimate and EstimateTokens share one
// cache but disagree on the floor, so an Estimate miss can pin a later budget
// check to zero tokens.
func TestEstimateDoesNotPoisonSharedCache(t *testing.T) {
	e := NewEstimator()

	const short = "abc"
	if got := e.Estimate(tokenlessProvider{}, short, "any-model"); got < 1 {
		t.Errorf("Estimate(%q) = %d, want at least 1 token", short, got)
	}

	if got := e.EstimateTokens(short); got < 1 {
		t.Errorf("EstimateTokens(%q) = %d after Estimate, want at least 1", short, got)
	}
}

// TestEstimatorCacheIsBounded: the cache is keyed by caller-supplied input and
// is consulted for the lifetime of the process, so it must have a ceiling.
func TestEstimatorCacheIsBounded(t *testing.T) {
	e := NewEstimator()

	for i := 0; i < 20000; i++ {
		e.EstimateTokens(fmt.Sprintf("prompt-%d", i))
	}

	e.mu.RLock()
	size := len(e.cache)
	e.mu.RUnlock()

	// Independent ceiling: the test must fail if the implementation raises its
	// own limit, so it does not read the production constant.
	const ceiling = 4096
	if size > ceiling {
		t.Errorf("cache grew to %d entries, want at most %d", size, ceiling)
	}
}

// TestEstimatorDoesNotRetainPromptText: prompts carry user content and
// credentials pasted by users; the cache must not keep them alive.
func TestEstimatorDoesNotRetainPromptText(t *testing.T) {
	e := NewEstimator()

	const secret = "ghp-do-not-retain-this-literal"
	e.EstimateTokens("please review " + secret)
	e.Estimate(tokenlessProvider{}, "please review "+secret, "some-model")

	e.mu.RLock()
	defer e.mu.RUnlock()
	for key := range e.cache {
		if strings.Contains(key, secret) {
			t.Fatalf("cache key retains prompt text: %q", key)
		}
	}
}
