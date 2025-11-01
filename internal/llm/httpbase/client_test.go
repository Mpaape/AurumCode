package httpbase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientRetryOn500(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
		}
	}))
	defer server.Close()
	
	client := NewClient(server.URL)
	req := &Request{
		Method: "GET",
		Path:   "/test",
	}
	
	ctx := context.Background()
	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("Request should succeed after retries: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}
	
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestClientRateLimitRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok": true}`))
		}
	}))
	defer server.Close()
	
	client := NewClient(server.URL)
	req := &Request{
		Method: "GET",
		Path:   "/test",
	}
	
	ctx := context.Background()
	resp, err := client.Do(ctx, req)
	if err != nil {
		t.Fatalf("Request should succeed after retries: %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestRedactSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sk-abc123def456", "sk-***REDACTED***"},
		{"Bearer token123", "Bearer ***REDACTED***"},
		{"X-API-Key: secret", "X-API-Key:***REDACTED***"},
		{"normal string", "normal string"},
	}
	
	for _, tt := range tests {
		result := RedactSecret(tt.input)
		if result != tt.expected {
			t.Errorf("RedactSecret(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDecodeJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "hello", "count": 42}`))
	}))
	defer server.Close()
	
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	var result map[string]interface{}
	err = DecodeJSON(resp, &result)
	if err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
	
	if result["message"] != "hello" {
		t.Errorf("Expected message 'hello', got %v", result["message"])
	}
	
	if result["count"] != float64(42) {
		t.Errorf("Expected count 42, got %v", result["count"])
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	
	client := NewClient(server.URL)
	client.timeout = 100 * time.Millisecond
	client.httpClient.Timeout = client.timeout
	
	req := &Request{
		Method: "GET",
		Path:   "/test",
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	
	_, err := client.Do(ctx, req)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

