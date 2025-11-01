package main

import (
	"aurumcode/internal/git/webhook"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Realistic GitHub webhook payloads
const pullRequestOpenedPayload = `{
  "action": "opened",
  "number": 42,
  "pull_request": {
    "number": 42,
    "state": "open",
    "head": {
      "ref": "feature",
      "sha": "abc123"
    },
    "base": {
      "ref": "main"
    }
  },
  "repository": {
    "full_name": "owner/repo"
  }
}`

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "healthy") {
		t.Errorf("expected body to contain 'healthy', got: %s", body)
	}
}

func TestMetricsHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	MetricsHandler(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Metrics") {
		t.Errorf("expected body to contain 'Metrics', got: %s", body)
	}
}

func TestWebhookHandler_ValidSignature(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "test-secret",
	}

	// Use realistic PR payload
	payload := []byte(pullRequestOpenedPayload)

	// Compute valid signature
	signature := computeTestSignature(payload, cfg.WebhookSecret)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "test-delivery-1")
	w := httptest.NewRecorder()

	cache := createTestCache()
	handler := WebhookHandler(cfg, cache)
	handler(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "received") {
		t.Errorf("expected body to contain 'received', got: %s", body)
	}
}

func TestWebhookHandler_MissingSignature(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "test-secret",
	}

	payload := []byte(`{"test": "payload"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	// No signature header
	w := httptest.NewRecorder()

	cache := createTestCache()
	handler := WebhookHandler(cfg, cache)
	handler(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "missing signature") {
		t.Errorf("expected body to contain 'missing signature', got: %s", body)
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "test-secret",
	}

	payload := []byte(`{"test": "payload"}`)

	// Compute signature with wrong secret
	signature := computeTestSignature(payload, "wrong-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", signature)
	w := httptest.NewRecorder()

	cache := createTestCache()
	handler := WebhookHandler(cfg, cache)
	handler(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "invalid signature") {
		t.Errorf("expected body to contain 'invalid signature', got: %s", body)
	}
}

func TestWebhookHandler_MalformedSignature(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "test-secret",
	}

	payload := []byte(`{"test": "payload"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", "invalid-signature-format")
	w := httptest.NewRecorder()

	cache := createTestCache()
	handler := WebhookHandler(cfg, cache)
	handler(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_DuplicateDelivery(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "test-secret",
	}

	payload := []byte(pullRequestOpenedPayload)
	signature := computeTestSignature(payload, cfg.WebhookSecret)

	cache := createTestCache()
	handler := WebhookHandler(cfg, cache)

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req1.Header.Set("X-Hub-Signature-256", signature)
	req1.Header.Set("X-GitHub-Event", "pull_request")
	req1.Header.Set("X-GitHub-Delivery", "duplicate-test")
	w1 := httptest.NewRecorder()
	handler(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("expected status 200 for first request, got %d", w1.Code)
	}

	// Second request (duplicate)
	req2 := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req2.Header.Set("X-Hub-Signature-256", signature)
	req2.Header.Set("X-GitHub-Event", "pull_request")
	req2.Header.Set("X-GitHub-Delivery", "duplicate-test")
	w2 := httptest.NewRecorder()
	handler(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200 for duplicate, got %d", w2.Code)
	}

	body := w2.Body.String()
	if !strings.Contains(body, "duplicate") {
		t.Errorf("expected body to contain 'duplicate', got: %s", body)
	}
}

func TestWebhookHandler_UnsupportedEvent(t *testing.T) {
	cfg := &ServerConfig{
		WebhookSecret: "test-secret",
	}

	payload := []byte(`{"action":"opened"}`)
	signature := computeTestSignature(payload, cfg.WebhookSecret)

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "unsupported-test")
	w := httptest.NewRecorder()

	cache := createTestCache()
	handler := WebhookHandler(cfg, cache)
	handler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204 for unsupported event, got %d", w.Code)
	}
}

func createTestCache() *webhook.IdempotencyCache {
	return webhook.NewIdempotencyCache(1*time.Minute, 0)
}

// Helper function to compute test signatures
func computeTestSignature(payload []byte, secret string) string {
	// Import from webhook package
	return "sha256=" + computeHMACSHA256(payload, secret)
}

func computeHMACSHA256(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
