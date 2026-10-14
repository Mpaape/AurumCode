package config

import "fmt"

// BatchesConfig is the batches section (nil = not declared): the optional
// ceilings of a review whose diff the model refused as too long and that is
// reviewed in batches (internal/review/split.go). Zero, or absent, bounds
// nothing: every batch is reviewed. Files beyond a ceiling that is set are
// left out, declared, and the approval is withheld; the ceilings never make a
// file count as reviewed.
type BatchesConfig struct {
	MaxBatches      int `yaml:"max_batches"`
	MaxPromptTokens int `yaml:"max_prompt_tokens"`
}

// Limits returns the declared ceilings, zero where not declared.
func (b *BatchesConfig) Limits() (maxBatches, maxPromptTokens int) {
	if b == nil {
		return 0, 0
	}
	return b.MaxBatches, b.MaxPromptTokens
}

// Validate refuses a negative ceiling (zero means no ceiling).
func (b *BatchesConfig) Validate() error {
	if b == nil {
		return nil
	}
	if b.MaxBatches < 0 {
		return fmt.Errorf("batches.max_batches: must be positive (got %d)", b.MaxBatches)
	}
	if b.MaxPromptTokens < 0 {
		return fmt.Errorf("batches.max_prompt_tokens: must be positive (got %d)", b.MaxPromptTokens)
	}
	return nil
}

// mergeBatches governs the section like deliberation: a policy that declares
// it decides alone; a policy silent on it leaves the repository's.
func mergeBatches(effective *Config, repo, central *Config) []ProviderWarning {
	if central.Batches == nil {
		return nil
	}
	effective.Batches = central.Batches
	if repo.Batches == nil {
		return nil
	}
	return []ProviderWarning{{
		Provider: centralPolicyProvider,
		Reason:   "batches do config do repositório foi ignorado: a política central decide sozinha",
	}}
}
