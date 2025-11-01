package config

import (
	"aurumcode/pkg/types"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigPath     = "configs/default-config.yml"
	RepoConfigPath        = ".aurumcode/config.yml"
	DefaultAurumCodeDir   = ".aurumcode"
)

// Loader handles configuration loading with caching
type Loader struct {
	cache map[string]*types.Config
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	return &Loader{
		cache: make(map[string]*types.Config),
	}
}

// Load loads configuration from the default paths
func (l *Loader) Load() (*types.Config, error) {
	return l.LoadFromPath("")
}

// LoadFromPath loads configuration from a specific path
func (l *Loader) LoadFromPath(path string) (*types.Config, error) {
	// Use default path if not specified
	if path == "" {
		path = RepoConfigPath
	}

	// Load defaults first
	defaultCfg, err := loadDefaults()
	if err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Load repo config if exists
	repoCfg, err := loadRepoConfig(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load repo config: %w", err)
	}

	// Merge configurations (repo overrides defaults)
	cfg := mergeConfig(defaultCfg, repoCfg)

	// Apply environment overrides
	cfg = applyEnvOverrides(cfg)

	// Validate configuration
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return cfg, nil
}

// loadDefaults loads the default configuration from configs/default-config.yml
func loadDefaults() (*types.Config, error) {
	data, err := os.ReadFile(DefaultConfigPath)
	if err != nil {
		// If default config doesn't exist, use programmatic defaults
		return types.NewDefaultConfig(), nil
	}

	var cfg types.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal default config: %w", err)
	}

	return &cfg, nil
}

// loadRepoConfig loads configuration from .aurumcode/config.yml
func loadRepoConfig(path string) (*types.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg types.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal repo config: %w", err)
	}

	return &cfg, nil
}

// mergeConfig merges two configs with b taking precedence over a
func mergeConfig(a, b *types.Config) *types.Config {
	if b == nil {
		return a
	}

	result := *a
	
	// Merge LLM config
	if b.LLM.Provider != "" {
		result.LLM.Provider = b.LLM.Provider
	}
	if b.LLM.Model != "" {
		result.LLM.Model = b.LLM.Model
	}
	if b.LLM.Temperature > 0 || b.LLM.Temperature < 0 {
		result.LLM.Temperature = b.LLM.Temperature
	}
	if b.LLM.MaxTokens > 0 {
		result.LLM.MaxTokens = b.LLM.MaxTokens
	}
	if b.LLM.Budgets.DailyUSD > 0 {
		result.LLM.Budgets.DailyUSD = b.LLM.Budgets.DailyUSD
	}
	if b.LLM.Budgets.PerReviewTokens > 0 {
		result.LLM.Budgets.PerReviewTokens = b.LLM.Budgets.PerReviewTokens
	}

	// Merge prompts
	if len(b.Prompts) > 0 {
		if result.Prompts == nil {
			result.Prompts = make(map[string]string)
		}
		for k, v := range b.Prompts {
			result.Prompts[k] = v
		}
	}

	// Merge rules
	if len(b.Rules) > 0 {
		if result.Rules == nil {
			result.Rules = make(map[string]string)
		}
		for k, v := range b.Rules {
			result.Rules[k] = v
		}
	}

	// Merge outputs
	result.Outputs.CommentOnPR = b.Outputs.CommentOnPR || result.Outputs.CommentOnPR
	result.Outputs.UpdateDocs = b.Outputs.UpdateDocs || result.Outputs.UpdateDocs
	result.Outputs.GenerateTests = b.Outputs.GenerateTests || result.Outputs.GenerateTests
	result.Outputs.DeploySite = b.Outputs.DeploySite || result.Outputs.DeploySite

	return &result
}

// LoadPrompt loads a prompt file by key
func (l *Loader) LoadPrompt(cfg *types.Config, key string) ([]byte, error) {
	path, ok := cfg.Prompts[key]
	if !ok {
		return nil, fmt.Errorf("prompt key '%s' not found in config", key)
	}

	// Resolve relative to .aurumcode directory
	fullPath := filepath.Join(DefaultAurumCodeDir, path)
	
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file '%s': %w", fullPath, err)
	}

	return data, nil
}

// LoadRule loads a rule file by key
func (l *Loader) LoadRule(cfg *types.Config, key string) ([]byte, error) {
	path, ok := cfg.Rules[key]
	if !ok {
		return nil, fmt.Errorf("rule key '%s' not found in config", key)
	}

	fullPath := filepath.Join(DefaultAurumCodeDir, path)
	
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read rule file '%s': %w", fullPath, err)
	}

	return data, nil
}

// LoadDocument loads a document file by key
func (l *Loader) LoadDocument(cfg *types.Config, key string) ([]byte, error) {
	// TODO: Implement document loading
	return nil, fmt.Errorf("document loading not yet implemented")
}

