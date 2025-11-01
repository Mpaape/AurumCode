package llm

import (
	"encoding/json"
	"testing"
)

// fakeProvider is a simple mock Provider for testing
type fakeProvider struct {
	name      string
	returnText string
	returnErr error
}

func (f *fakeProvider) Complete(prompt string, opts Options) (Response, error) {
	if f.returnErr != nil {
		return Response{}, f.returnErr
	}
	return Response{
		Text:      f.returnText,
		TokensIn:  len(prompt),
		TokensOut: len(f.returnText),
	}, nil
}

func (f *fakeProvider) Tokens(input string) (int, error) {
	return len(input), nil
}

func (f *fakeProvider) Name() string {
	return f.name
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	
	if opts.Temperature != 0.3 {
		t.Errorf("Expected default temperature 0.3, got %f", opts.Temperature)
	}
	
	if opts.MaxTokens != 4000 {
		t.Errorf("Expected default max_tokens 4000, got %d", opts.MaxTokens)
	}
}

func TestProviderInterface(t *testing.T) {
	var p Provider = &fakeProvider{
		name:      "test-provider",
		returnText: "test response",
	}
	
	if p.Name() != "test-provider" {
		t.Errorf("Expected name 'test-provider', got %s", p.Name())
	}
	
	resp, err := p.Complete("test prompt", DefaultOptions())
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	
	if resp.Text != "test response" {
		t.Errorf("Expected response 'test response', got %s", resp.Text)
	}
}

func TestResponseJSONMarshal(t *testing.T) {
	resp := Response{
		Text:       "test",
		TokensIn:   100,
		TokensOut:  200,
		Model:      "gpt-4",
		FinishReason: "stop",
	}
	
	// Marshal to JSON to ensure determinism
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}
	
	data2, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal response second time: %v", err)
	}
	
	if string(data) != string(data2) {
		t.Errorf("Non-deterministic JSON marshaling")
	}
}

