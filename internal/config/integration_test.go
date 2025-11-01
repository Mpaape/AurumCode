package config

import (
	"os"
	"testing"
)

func TestLoadFromRepoConfig(t *testing.T) {
	// We might be in /build/internal/config or /build
	// Try paths relative to both
	fixtureConfig := "../../tests/fixtures/repo1/.aurumcode/config.yml"
	if _, err := os.Stat(fixtureConfig); os.IsNotExist(err) {
		fixtureConfig = "tests/fixtures/repo1/.aurumcode/config.yml"
		if _, err := os.Stat(fixtureConfig); os.IsNotExist(err) {
			t.Skipf("Skipping: fixture not found")
		}
	}

	loader := NewLoader()
	cfg, err := loader.LoadFromPath(fixtureConfig)
	if err != nil {
		t.Fatalf("Failed to load config from fixture: %v", err)
	}

	// Verify repo config overrides
	if cfg.LLM.Provider != "openai" {
		t.Errorf("Expected provider 'openai' from repo config, got %s", cfg.LLM.Provider)
	}

	if cfg.LLM.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4' from repo config, got %s", cfg.LLM.Model)
	}

	if cfg.LLM.Temperature != 0.5 {
		t.Errorf("Expected temperature 0.5 from repo config, got %f", cfg.LLM.Temperature)
	}

	// Test prompt loading - need to adjust working directory
	originalDir, err := os.Getwd()
	if err == nil {
		// We're already in internal/config, so need to go up and into fixtures
		os.Chdir("../../tests/fixtures/repo1")
		defer func() {
			os.Chdir(originalDir)
		}()
		
		promptData, err := loader.LoadPrompt(cfg, "code_review")
		if err != nil {
			t.Fatalf("Failed to load prompt: %v", err)
		}

		if len(promptData) == 0 {
			t.Error("Expected prompt data to be non-empty")
		}

		// Test rule loading
		ruleData, err := loader.LoadRule(cfg, "code_standards")
		if err != nil {
			t.Fatalf("Failed to load rule: %v", err)
		}

		if len(ruleData) == 0 {
			t.Error("Expected rule data to be non-empty")
		}
	}
}

