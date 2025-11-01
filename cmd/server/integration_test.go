package main

import (
	"aurumcode/internal/git/webhook"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// End-to-end integration tests

func TestWebhookIntegration_PullRequestFlow(t *testing.T) {
	// Setup server
	cfg := &ServerConfig{
		WebhookSecret: "integration-secret",
	}
	cache := webhook.NewIdempotencyCache(1*time.Minute, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", HealthHandler)
	mux.HandleFunc("/webhook/github", WebhookHandler(cfg, cache))

	handler := RequestIDMiddleware(
		LoggingMiddleware(
			RecoveryMiddleware(mux),
		),
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	// Test PR opened
	payload := []byte(`{
		"action": "opened",
		"number": 1,
		"pull_request": {
			"head": {"ref": "feature", "sha": "abc123"},
			"base": {"ref": "main"}
		},
		"repository": {"full_name": "test/repo"}
	}`)

	signature := computeHMACSHA256(payload, cfg.WebhookSecret)

	req, _ := http.NewRequest("POST", server.URL+"/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "integration-pr-1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "pull_request.opened") {
		t.Errorf("expected event type in response, got: %s", string(body))
	}
}

func TestWebhookIntegration_PushFlow(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "integration-secret",
	}
	cache := webhook.NewIdempotencyCache(1*time.Minute, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", WebhookHandler(cfg, cache))

	handler := RequestIDMiddleware(mux)
	server := httptest.NewServer(handler)
	defer server.Close()

	payload := []byte(`{
		"ref": "refs/heads/main",
		"repository": {"full_name": "test/repo"}
	}`)

	signature := computeHMACSHA256(payload, cfg.WebhookSecret)

	req, _ := http.NewRequest("POST", server.URL+"/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "integration-push-1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "push") {
		t.Errorf("expected push event in response, got: %s", string(body))
	}
}

func TestWebhookIntegration_HealthCheck(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", HealthHandler)

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "healthy") {
		t.Errorf("expected 'healthy' in response, got: %s", string(body))
	}
}

func TestWebhookIntegration_FullMiddlewareStack(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "middleware-test",
	}
	cache := webhook.NewIdempotencyCache(1*time.Minute, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", WebhookHandler(cfg, cache))

	// Full middleware stack
	handler := RequestIDMiddleware(
		LoggingMiddleware(
			RecoveryMiddleware(mux),
		),
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	payload := []byte(`{
		"action": "opened",
		"number": 999,
		"pull_request": {
			"head": {"ref": "test", "sha": "xyz"},
			"base": {"ref": "main"}
		},
		"repository": {"full_name": "org/project"}
	}`)

	signature := computeHMACSHA256(payload, cfg.WebhookSecret)

	req, _ := http.NewRequest("POST", server.URL+"/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "middleware-test-1")
	req.Header.Set("X-Request-ID", "custom-request-id")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check middleware headers
	requestID := resp.Header.Get("X-Request-ID")
	if requestID != "custom-request-id" {
		t.Errorf("expected custom request ID to be preserved, got: %s", requestID)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestWebhookIntegration_ConcurrentRequests(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "concurrent-test",
	}
	cache := webhook.NewIdempotencyCache(1*time.Minute, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", WebhookHandler(cfg, cache))

	handler := RequestIDMiddleware(mux)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send multiple concurrent requests
	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			payload := []byte(`{
				"action": "opened",
				"number": ` + string(rune('0'+id)) + `,
				"pull_request": {
					"head": {"ref": "concurrent", "sha": "test"},
					"base": {"ref": "main"}
				},
				"repository": {"full_name": "test/concurrent"}
			}`)

			signature := computeHMACSHA256(payload, cfg.WebhookSecret)

			req, _ := http.NewRequest("POST", server.URL+"/webhook/github", strings.NewReader(string(payload)))
			req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
			req.Header.Set("X-GitHub-Event", "pull_request")
			req.Header.Set("X-GitHub-Delivery", "concurrent-"+string(rune('A'+id)))

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("request %d failed: %v", id, err)
				done <- false
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("request %d: expected status 200, got %d", id, resp.StatusCode)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Wait for all requests
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

func TestWebhookIntegration_ErrorRecovery(t *testing.T) {
	// Test that server recovers from panics
	mux := http.NewServeMux()
	mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("intentional panic for testing")
	})

	handler := RecoveryMiddleware(mux)
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/panic")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 500, not crash
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500 after panic, got %d", resp.StatusCode)
	}
}
