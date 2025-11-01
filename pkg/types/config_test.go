package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()
	
	if cfg.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", cfg.Version)
	}
	
	if cfg.LLM.Provider != "auto" {
		t.Errorf("Expected provider 'auto', got %s", cfg.LLM.Provider)
	}
	
	if cfg.LLM.Temperature != 0.3 {
		t.Errorf("Expected temperature 0.3, got %f", cfg.LLM.Temperature)
	}
	
	if cfg.LLM.MaxTokens != 4000 {
		t.Errorf("Expected max_tokens 4000, got %d", cfg.LLM.MaxTokens)
	}
	
	if cfg.LLM.Budgets.DailyUSD != 10.0 {
		t.Errorf("Expected daily budget 10.0, got %f", cfg.LLM.Budgets.DailyUSD)
	}
}

func TestConfigYAMLRoundTrip(t *testing.T) {
	original := NewDefaultConfig()
	
	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	
	// Unmarshal back
	var unmarshaled Config
	err = yaml.Unmarshal(yamlBytes, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}
	
	// Verify key fields
	if unmarshaled.Version != original.Version {
		t.Errorf("Version mismatch: got %s, want %s", unmarshaled.Version, original.Version)
	}
	
	if unmarshaled.LLM.Provider != original.LLM.Provider {
		t.Errorf("LLM.Provider mismatch: got %s, want %s", unmarshaled.LLM.Provider, original.LLM.Provider)
	}
	
	if unmarshaled.LLM.Temperature != original.LLM.Temperature {
		t.Errorf("LLM.Temperature mismatch: got %f, want %f", unmarshaled.LLM.Temperature, original.LLM.Temperature)
	}
}

