package types

// Config represents the complete AurumCode configuration
type Config struct {
	Version  string                 `json:"version" yaml:"version"`
	LLM      LLMConfig              `json:"llm" yaml:"llm"`
	Prompts  map[string]string      `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Rules    map[string]string      `json:"rules,omitempty" yaml:"rules,omitempty"`
	Outputs  OutputConfig           `json:"outputs" yaml:"outputs"`
	Features FeaturesConfig         `json:"features" yaml:"features"`
}

// LLMConfig configures the LLM provider and parameters
type LLMConfig struct {
	Provider  string         `json:"provider" yaml:"provider"` // "auto", "litellm", "openai", "anthropic", "ollama"
	Model     string         `json:"model" yaml:"model"`
	Temperature float64      `json:"temperature" yaml:"temperature"`
	MaxTokens int            `json:"max_tokens" yaml:"max_tokens"`
	Budgets   BudgetConfig   `json:"budgets" yaml:"budgets"`
}

// BudgetConfig defines cost controls
type BudgetConfig struct {
	DailyUSD          float64 `json:"daily_usd" yaml:"daily_usd"`
	PerReviewTokens   int     `json:"per_review_tokens" yaml:"per_review_tokens"`
}

// OutputConfig controls what AurumCode generates
type OutputConfig struct {
	CommentOnPR   bool `json:"comment_on_pr" yaml:"comment_on_pr"`
	UpdateDocs    bool `json:"update_docs" yaml:"update_docs"`
	GenerateTests bool `json:"generate_tests" yaml:"generate_tests"`
	DeploySite    bool `json:"deploy_site" yaml:"deploy_site"`
}

// FeaturesConfig enables/disables the 3 main use cases
type FeaturesConfig struct {
	CodeReview       bool `json:"code_review" yaml:"code_review"`
	CodeReviewOnPush bool `json:"code_review_on_push" yaml:"code_review_on_push"`
	Documentation    bool `json:"documentation" yaml:"documentation"`
	QATesting        bool `json:"qa_testing" yaml:"qa_testing"`
}

// NewDefaultConfig returns a configuration with sensible defaults
func NewDefaultConfig() *Config {
	return &Config{
		Version: "2.0",
		LLM: LLMConfig{
			Provider:    "auto",
			Model:       "sonnet-like",
			Temperature: 0.3,
			MaxTokens:   4000,
			Budgets: BudgetConfig{
				DailyUSD:        10.0,
				PerReviewTokens: 8000,
			},
		},
		Prompts: map[string]string{
			"code_review": "prompts/code-review/general.md",
		},
		Rules: map[string]string{
			"code_standards": "rules/code-standards.yml",
			"iso_compliance": "rules/iso-compliance.yml",
			"security":       "rules/security-rules.yml",
		},
		Outputs: OutputConfig{
			CommentOnPR:   true,
			UpdateDocs:    true,
			GenerateTests: true,
			DeploySite:    true,
		},
		Features: FeaturesConfig{
			CodeReview:       true,
			CodeReviewOnPush: false,
			Documentation:    true,
			QATesting:        true,
		},
	}
}

