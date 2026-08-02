package httpbase

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	fn()
	return buf.String()
}

// TestRequestLogNeverPrintsHeaderValues: the consumer's key travels in an
// auth header whose value carries no marker RedactSecret can recognise.
func TestRequestLogNeverPrintsHeaderValues(t *testing.T) {
	const key = "9f8e7d6c5b4a3z2y1x0w"

	c := NewClient("https://example.invalid")
	out := captureLog(t, func() {
		c.logRequest(&Request{
			Method:  http.MethodPost,
			Path:    "/chat/completions",
			Headers: map[string]string{"X-Api-Key": key, "Authorization": "Bearer " + key},
		})
	})

	if strings.Contains(out, key) {
		t.Fatalf("api key leaked into log output: %q", out)
	}
}

// TestRequestLogNeverPrintsBody: request bodies carry the prompt, which in this
// product is repository content supplied by the consumer.
func TestRequestLogNeverPrintsBody(t *testing.T) {
	const payload = "internal-source-code-that-must-not-be-logged"

	c := NewClient("https://example.invalid")
	out := captureLog(t, func() {
		c.logRequest(&Request{
			Method: http.MethodPost,
			Path:   "/chat/completions",
			Body:   map[string]string{"prompt": payload},
		})
	})

	if strings.Contains(out, payload) {
		t.Fatalf("request body leaked into log output: %q", out)
	}
}

// TestRedactSecretKeepsValidUTF8: the offset is searched in a case-folded copy
// and then applied to the original, and case folding can change byte length.
func TestRedactSecretKeepsValidUTF8(t *testing.T) {
	input := strings.Repeat("İ", 6) + "sk-SUPERSECRETKEY"

	got := RedactSecret(input)

	if !utf8.ValidString(got) {
		t.Errorf("RedactSecret produced invalid UTF-8: %q", got)
	}
	if strings.Contains(got, "SUPERSECRETKEY") {
		t.Errorf("RedactSecret leaked the secret: %q", got)
	}
}

// TestDoPreservesStatusAfterRetriesExhausted: a caller that cannot see 429 vs
// 503 cannot decide whether to back off or fail the run.
func TestDoPreservesStatusAfterRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.maxRetries = 1
	c.backoffBase = time.Millisecond

	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/v1"})
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected an error once retries are exhausted")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error lost the HTTP status that caused it: %v", err)
	}

	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected a *StatusError, got %T: %v", err, err)
	}
	if statusErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want 429", statusErr.StatusCode)
	}
	if statusErr.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", statusErr.Attempts)
	}
}

// TestDoRetriesTransportErrors: a dropped connection is the most common
// transient failure against a remote gateway, and Do advertises retries.
func TestDoRetriesTransportErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			panic(http.ErrAbortHandler)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.backoffBase = time.Millisecond

	resp, err := c.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/v1"})
	if err != nil {
		t.Fatalf("expected recovery after transport errors, got: %v", err)
	}
	defer resp.Body.Close()

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

// TestDecodeJSONAcceptsAllSuccessStatuses: the OpenAI-compatible contract is
// 2xx, not literally 200.
func TestDecodeJSONAcceptsAllSuccessStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"message":"hello"}`))
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	var result map[string]interface{}
	if err := DecodeJSON(resp, &result); err != nil {
		t.Fatalf("DecodeJSON rejected a 201 success: %v", err)
	}
	if result["message"] != "hello" {
		t.Errorf("got %v, want hello", result["message"])
	}
}

// TestDecodeJSONBoundsErrorBody: the error body is remote-controlled and ends
// up in logs and in the Action output.
func TestDecodeJSONBoundsErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(bytes.Repeat([]byte("A"), 1<<20))
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	err = DecodeJSON(resp, &struct{}{})
	if err == nil {
		t.Fatal("expected an error for HTTP 500")
	}
	if len(err.Error()) > 4096 {
		t.Fatalf("error message is unbounded: %d bytes", len(err.Error()))
	}
}
