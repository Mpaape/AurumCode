package main

// AUR-543 behavior proof. AC-001 and AC-002 (FixedContentDigest itself) are
// proven at the internal/prompt package boundary (internal/prompt/aur543_test.go).
// This file covers AC-003 end-to-end: the same model name, served by TWO
// real httptest.Server endpoints (different LLM_BASE_URL), with a persisted
// cache across both rounds, must call the SECOND server for real -- never
// serve round 2 from round 1's cache entry -- proven the only trustworthy
// way: an independent request counter per server. AUR-513 already folds
// LLM_BASE_URL into modelCacheKey, but until this card no test proved it
// end-to-end with two actual servers (the AUR-513 review's own documented
// gap); this test also fails if that LLM_BASE_URL fold-in regresses.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const aur543CleanResponse = `{"summary":"ok","verdict":"approve","issues":[]}`

func aur543CountingServer() (*httptest.Server, *int32) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + jsonEscapeAUR543(aur543CleanResponse) + `"}}],"usage":{}}`))
	}))
	return server, &requests
}

// jsonEscapeAUR543 escapes aur543CleanResponse's own quotes so it can be
// embedded as a JSON string literal inside the fixed LiteLLM-shaped
// response body above.
func jsonEscapeAUR543(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// TestAUR543AC003DifferentBaseURLForcesFreshReview covers AC-003: the exact
// same model name (LLM_MODEL unchanged) answered by a second, independent
// endpoint (a different LLM_BASE_URL) must never be served from the cache
// entry the first endpoint's round wrote. A persisted AURUMCODE_CACHE_DIR
// (set once by cleanFixture, kept across both rounds here) is what makes a
// wrongly-shared key observable at all -- an unmetered cache would hide this
// exact defect, since every run would reach the model anyway.
func TestAUR543AC003DifferentBaseURLForcesFreshReview(t *testing.T) {
	cleanFixture(t, "")

	server1, requests1 := aur543CountingServer()
	defer server1.Close()
	server2, requests2 := aur543CountingServer()
	defer server2.Close()

	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "model-shared-name")
	t.Setenv("LLM_BASE_URL", server1.URL)

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}
	if got := atomic.LoadInt32(requests1); got != 1 {
		t.Fatalf("expected exactly 1 request to server1 after round1, got %d", got)
	}
	if got := atomic.LoadInt32(requests2); got != 0 {
		t.Fatalf("expected 0 requests to server2 before it is ever selected, got %d", got)
	}

	// Round 2: the SAME model name, but a different endpoint. The diff is
	// byte-identical to round 1's (same HEAD~1 fixture, untouched).
	t.Setenv("LLM_BASE_URL", server2.URL)
	var out2, err2 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter()); code != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reused") {
		t.Fatalf("AC-003: a different LLM_BASE_URL under the same model name must never reuse the first endpoint's cache entry:\n%s", err2.String())
	}
	if got := atomic.LoadInt32(requests2); got != 1 {
		t.Fatalf("AC-003: expected exactly 1 real request to server2 after switching LLM_BASE_URL, got %d (a cache hit would leave this at 0)", got)
	}
	if got := atomic.LoadInt32(requests1); got != 1 {
		t.Fatalf("expected server1's request count to stay at 1 (round2 must not re-call server1), got %d", got)
	}

	// Round 3: back to server1's URL with the same model name and the same
	// diff -- round1's own entry must still be a valid hit (this is not
	// simply "every round is always a miss").
	t.Setenv("LLM_BASE_URL", server1.URL)
	var out3, err3 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out3, &err3, redaction.NewFilter()); code != 0 {
		t.Fatalf("round3 exit=%d, want 0; stdout=%s stderr=%s", code, out3.String(), err3.String())
	}
	if !strings.Contains(err3.String(), "reused") {
		t.Fatalf("expected round3 (server1's URL again, same diff) to reuse round1's cache entry:\n%s", err3.String())
	}
	if got := atomic.LoadInt32(requests1); got != 1 {
		t.Fatalf("expected server1's request count to stay at 1 (round3 must be served from cache, not a new request), got %d", got)
	}
}
