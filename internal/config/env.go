package config

import (
	"aurumcode/pkg/types"
	"os"
	"strconv"
)

// applyEnvOverrides applies environment variable overrides to the configuration
func applyEnvOverrides(cfg *types.Config) *types.Config {
	result := *cfg

	// LLM_PROVIDER
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		result.LLM.Provider = provider
	}

	// LLM_MODEL
	if model := os.Getenv("LLM_MODEL"); model != "" {
		result.LLM.Model = model
	}

	// LLM_TEMPERATURE
	if tempStr := os.Getenv("LLM_TEMPERATURE"); tempStr != "" {
		if temp, err := strconv.ParseFloat(tempStr, 64); err == nil {
			result.LLM.Temperature = temp
		}
	}

	// LLM_MAX_TOKENS
	if tokensStr := os.Getenv("LLM_MAX_TOKENS"); tokensStr != "" {
		if tokens, err := strconv.Atoi(tokensStr); err == nil {
			result.LLM.MaxTokens = tokens
		}
	}

	// LLM_BASE_URL (for custom endpoints like Ollama)
	if baseURL := os.Getenv("LLM_BASE_URL"); baseURL != "" {
		// Store in a custom map for now
		if result.Prompts == nil {
			result.Prompts = make(map[string]string)
		}
		result.Prompts["_internal_base_url"] = baseURL
	}

	// LLM_API_KEY should be handled separately for security
	// We don't store it in Config for security reasons
	apiKey := os.Getenv("LLM_API_KEY")
	_ = apiKey // Used later for provider initialization

	return &result
}

