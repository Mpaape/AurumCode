package config

import (
	"aurumcode/pkg/types"
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	loader := NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", cfg.Version)
	}

	if cfg.LLM.Provider != "auto" {
		t.Errorf("Expected provider 'auto', got %s", cfg.LLM.Provider)
	}

	if cfg.LLM.Model != "sonnet-like" {
		t.Errorf("Expected model 'sonnet-like', got %s", cfg.LLM.Model)
	}
}

func TestEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("LLM_PROVIDER", "openai")
	os.Setenv("LLM_TEMPERATURE", "0.7")
	os.Setenv("LLM_MAX_TOKENS", "8000")
	defer func() {
		os.Unsetenv("LLM_PROVIDER")
		os.Unsetenv("LLM_TEMPERATURE")
		os.Unsetenv("LLM_MAX_TOKENS")
	}()

	loader := NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.LLM.Provider != "openai" {
		t.Errorf("Expected provider 'openai' from env, got %s", cfg.LLM.Provider)
	}

	if cfg.LLM.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7 from env, got %f", cfg.LLM.Temperature)
	}

	if cfg.LLM.MaxTokens != 8000 {
		t.Errorf("Expected max_tokens 8000 from env, got %d", cfg.LLM.MaxTokens)
	}
}

func TestValidation(t *testing.T) {
	loader := NewLoader()
	
	// Test invalid provider
	os.Setenv("LLM_PROVIDER", "invalid-provider")
	defer os.Unsetenv("LLM_PROVIDER")
	
	_, err := loader.Load()
	if err == nil {
		t.Error("Expected validation error for invalid provider")
	}

	// Test invalid temperature
	os.Unsetenv("LLM_PROVIDER")
	os.Setenv("LLM_TEMPERATURE", "2.0")
	defer os.Unsetenv("LLM_TEMPERATURE")
	
	_, err = loader.Load()
	if err == nil {
		t.Error("Expected validation error for temperature > 1")
	}
}

func TestMergeConfig(t *testing.T) {
	a := &types.Config{
		Version: "1.0",
		LLM: types.LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4",
			Temperature: 0.5,
			MaxTokens:   2000,
		},
	}

	b := &types.Config{
		LLM: types.LLMConfig{
			Temperature: 0.8,
			MaxTokens:   4000,
		},
	}

	result := mergeConfig(a, b)

	if result.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", result.Version)
	}

	if result.LLM.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got %s", result.LLM.Provider)
	}

	if result.LLM.Temperature != 0.8 {
		t.Errorf("Expected temperature 0.8, got %f", result.LLM.Temperature)
	}

	if result.LLM.MaxTokens != 4000 {
		t.Errorf("Expected max_tokens 4000, got %d", result.LLM.MaxTokens)
	}
}

