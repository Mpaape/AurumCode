package openai

import (
	"aurumcode/internal/llm"
	"aurumcode/internal/llm/httpbase"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderName(t *testing.T) {
	p := NewProvider("test-key")
	if p.Name() != "openai" {
		t.Errorf("Expected provider name 'openai', got %s", p.Name())
	}
}

func TestProviderTokens(t *testing.T) {
	p := NewProvider("test-key")
	count, err := p.Tokens("hello world")
	if err != nil {
		t.Fatalf("Tokens() failed: %v", err)
	}
	
	// Should return heuristic approximation
	if count <= 0 {
		t.Errorf("Expected positive token count, got %d", count)
	}
}

func TestProviderComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"id": "chatcmpl-test",
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello! How can I help you?",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 12,
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	
	// Override base URL
	p := NewProvider("test-key")
	p.baseURL = server.URL
	p.client = httpbase.NewClient(server.URL)
	
	resp, err := p.Complete("hello", llm.Options{})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	
	if resp.Text != "Hello! How can I help you?" {
		t.Errorf("Expected response 'Hello! How can I help you?', got %s", resp.Text)
	}
	
	if resp.TokensIn != 10 {
		t.Errorf("Expected 10 input tokens, got %d", resp.TokensIn)
	}
	
	if resp.TokensOut != 12 {
		t.Errorf("Expected 12 output tokens, got %d", resp.TokensOut)
	}
}

