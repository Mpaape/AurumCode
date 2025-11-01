package config

import (
	"aurumcode/pkg/types"
	"fmt"
)

// validProviders is the set of valid LLM provider values
var validProviders = map[string]bool{
	"auto":       true,
	"litellm":    true,
	"openai":     true,
	"anthropic":  true,
	"ollama":     true,
}

// validate performs schema validation on the configuration
func validate(cfg *types.Config) error {
	// Validate LLM configuration
	if cfg.LLM.Provider != "" && !validProviders[cfg.LLM.Provider] {
		return fmt.Errorf("invalid LLM provider '%s', must be one of: auto, litellm, openai, anthropic, ollama", cfg.LLM.Provider)
	}

	if cfg.LLM.Model == "" {
		return fmt.Errorf("LLM model is required")
	}

	if cfg.LLM.Temperature < 0 || cfg.LLM.Temperature > 1 {
		return fmt.Errorf("LLM temperature must be between 0 and 1, got %f", cfg.LLM.Temperature)
	}

	if cfg.LLM.MaxTokens <= 0 {
		return fmt.Errorf("LLM max_tokens must be > 0, got %d", cfg.LLM.MaxTokens)
	}

	// Validate budgets
	if cfg.LLM.Budgets.DailyUSD < 0 {
		return fmt.Errorf("daily budget must be >= 0, got %f", cfg.LLM.Budgets.DailyUSD)
	}

	if cfg.LLM.Budgets.PerReviewTokens <= 0 {
		return fmt.Errorf("per_review_tokens must be > 0, got %d", cfg.LLM.Budgets.PerReviewTokens)
	}

	return nil
}

