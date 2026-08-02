package httpbase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestClientDo_EveryRetryAttemptCarriesFullBody is a REGRESSION GUARD, not a
// red-first test: the client already builds a fresh reader per attempt.
//
// It exists because the failure it guards against is invisible to every other
// test in this package. A single io.Reader shared across attempts is drained by
// the first one, so retry 2 and 3 send an empty body. The gateway answers 400,
// the client reports "gave up after 4 attempts", and nothing in the error says
// the payload was dropped. Reverting doAttempt to a shared reader turns this
// test red and leaves the rest of the suite green.
func TestClientDo_EveryRetryAttemptCarriesFullBody(t *testing.T) {
	payload := map[string]any{
		"model":  "gpt-4o-mini",
		"prompt": "the body that must survive a retry",
	}

	want, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var mu sync.Mutex
	var received [][]byte

	const failures = 2

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("server could not read body: %v", err)
		}

		mu.Lock()
		received = append(received, body)
		n := len(received)
		mu.Unlock()

		if n <= failures {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.backoffBase = time.Millisecond

	resp, err := c.Do(context.Background(), &Request{
		Method: http.MethodPost,
		Path:   "/v1/chat/completions",
		Body:   payload,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()

	// A guard that inspected zero attempts must not report success.
	if len(received) != failures+1 {
		t.Fatalf("expected %d attempts to reach the server, got %d", failures+1, len(received))
	}

	for i, got := range received {
		if len(got) == 0 {
			t.Errorf("attempt %d sent an EMPTY body; the payload was consumed by an earlier attempt", i+1)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("attempt %d body = %q, want %q", i+1, got, want)
		}
	}
}
