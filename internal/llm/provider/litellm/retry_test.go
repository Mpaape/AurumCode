package litellm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// rateLimitedServer answers the first limited requests with a plain 429
// carrying header, then with a completion.
func rateLimitedServer(t *testing.T, limited int32, header, value, text string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) <= limited {
			if header != "" {
				w.Header().Set(header, value)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"` + text + `"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(completionResponse{Choices: []choice{{Message: message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}}})
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

// A plain rate limit is waited out for as long as the provider asks, then
// the request goes again and is answered.
func TestPlainRateLimitWaitsWhatTheProviderAsksAndRetries(t *testing.T) {
	cases := []struct {
		header, value string
		want          time.Duration
	}{
		{"Retry-After", "2", 2 * time.Second},
		{"Retry-After-Ms", "1500", 1500 * time.Millisecond},
	}
	for _, c := range cases {
		t.Run(c.header, func(t *testing.T) {
			server, calls := rateLimitedServer(t, 1, c.header, c.value, "Number of request tokens has exceeded your per-minute rate limit")
			p := NewProvider("test-key", server.URL, "test-model")
			var waits []time.Duration
			p.sleep = func(d time.Duration) { waits = append(waits, d) }

			resp, err := p.Complete("hello", llm.Options{})
			if err != nil || resp.Text != "ok" {
				t.Fatalf("Complete = %+v, %v; want the answer after the wait", resp, err)
			}
			if atomic.LoadInt32(calls) != 2 || len(waits) != 1 || waits[0] != c.want {
				t.Fatalf("calls %d, waits %v; want 2 calls and one wait of %v", atomic.LoadInt32(calls), waits, c.want)
			}
		})
	}
}

// Without a wait named, the retries back off exponentially, with jitter.
func TestPlainRateLimitBacksOffWithoutRetryAfter(t *testing.T) {
	server, calls := rateLimitedServer(t, 3, "", "", "too many requests")
	p := NewProvider("test-key", server.URL, "test-model")
	var waits []time.Duration
	p.sleep = func(d time.Duration) { waits = append(waits, d) }
	if _, err := p.Complete("hello", llm.Options{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(calls) != 4 || len(waits) != 3 {
		t.Fatalf("calls %d, waits %v", atomic.LoadInt32(calls), waits)
	}
	for i, w := range waits {
		base := firstBackoff << i
		if w < base/2 || w >= base/2+base {
			t.Fatalf("wait %d = %v, want within [%v, %v)", i, w, base/2, base/2+base)
		}
	}
}

// The 429 that says this request alone is larger than the limit is a
// refusal for length: it is never retried.
func TestRequestLargerThanTheRateLimitIsNotRetried(t *testing.T) {
	server, calls := rateLimitedServer(t, 5, "Retry-After", "1", "This request would exceed your rate limit of 2,000,000 input tokens per minute")
	p := NewProvider("test-key", server.URL, "test-model")
	p.sleep = func(time.Duration) { t.Fatal("a refusal for length was waited on") }
	_, err := p.Complete("hello", llm.Options{})
	if _, ok := llm.AsPromptTooLong(err); !ok || atomic.LoadInt32(calls) != 1 {
		t.Fatalf("err %v, calls %d: want one refusal for length", err, atomic.LoadInt32(calls))
	}
}

// The retries end when the next wait would pass the provider timeout.
func TestRateLimitRetriesEndAtTheProviderTimeout(t *testing.T) {
	t.Setenv("AURUMCODE_LLM_TIMEOUT_SECONDS", "5")
	server, calls := rateLimitedServer(t, 100, "Retry-After", "60", "too many requests")
	p := NewProvider("test-key", server.URL, "test-model")
	p.sleep = func(time.Duration) { t.Fatal("a wait past the provider timeout was taken") }
	if _, err := p.Complete("hello", llm.Options{}); err == nil || atomic.LoadInt32(calls) != 1 {
		t.Fatalf("err %v, calls %d: want the 429 back at once", err, atomic.LoadInt32(calls))
	}
}
