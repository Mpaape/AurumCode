package litellm

import (
	"aurumcode/internal/llm"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderName(t *testing.T) {
	provider := NewProvider("test-key", "http://localhost", "test-model")
	if provider.Name() != "litellm" {
		t.Errorf("expected name 'litellm', got '%s'", provider.Name())
	}
}

func TestProviderTokens(t *testing.T) {
	provider := NewProvider("test-key", "http://localhost", "test-model")
	tokens, err := provider.Tokens("test input")
	if err != nil {
		t.Fatalf("Tokens failed: %v", err)
	}
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

func TestProviderComplete(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", auth)
		}

		// Parse request
		var req completionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}

		// Send response
		resp := completionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "test-model",
			Choices: []choice{
				{
					Index: 0,
					Message: message{
						Role:    "assistant",
						Content: "Test response",
					},
					FinishReason: "stop",
				},
			},
			Usage: usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider
	provider := NewProvider("test-key", server.URL, "test-model")

	// Test completion
	response, err := provider.Complete("hello", llm.Options{
		Temperature: 0.7,
		MaxTokens:   100,
	})

	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if response.Text != "Test response" {
		t.Errorf("expected 'Test response', got '%s'", response.Text)
	}

	if response.TokensIn != 10 {
		t.Errorf("expected 10 input tokens, got %d", response.TokensIn)
	}

	if response.TokensOut != 20 {
		t.Errorf("expected 20 output tokens, got %d", response.TokensOut)
	}

	if response.Model != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", response.Model)
	}
}

func TestProviderComplete_WithSystem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req completionRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify system message included
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages (system + user), got %d", len(req.Messages))
		}

		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message to be system, got %s", req.Messages[0].Role)
		}

		if req.Messages[0].Content != "You are a helpful assistant" {
			t.Errorf("unexpected system content: %s", req.Messages[0].Content)
		}

		resp := completionResponse{
			Choices: []choice{{Message: message{Content: "OK"}}},
			Usage:   usage{PromptTokens: 10, CompletionTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewProvider("test-key", server.URL, "test-model")

	_, err := provider.Complete("hello", llm.Options{
		System: "You are a helpful assistant",
	})

	if err != nil {
		t.Fatalf("Complete with system failed: %v", err)
	}
}

func TestProviderComplete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid API key"}`))
	}))
	defer server.Close()

	provider := NewProvider("invalid-key", server.URL, "test-model")

	_, err := provider.Complete("hello", llm.Options{})

	if err == nil {
		t.Fatal("expected error for invalid API key")
	}

	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestProviderComplete_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := completionResponse{
			Choices: []choice{}, // Empty choices
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewProvider("test-key", server.URL, "test-model")

	_, err := provider.Complete("hello", llm.Options{})

	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestProviderComplete_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	provider := NewProvider("test-key", server.URL, "test-model")

	_, err := provider.Complete("hello", llm.Options{})

	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
