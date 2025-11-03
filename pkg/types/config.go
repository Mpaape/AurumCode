package types

// Config represents the complete AurumCode configuration
type Config struct {
	Version       string                 `json:"version" yaml:"version"`
	LLM           LLMConfig              `json:"llm" yaml:"llm"`
	Prompts       map[string]string      `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Rules         map[string]string      `json:"rules,omitempty" yaml:"rules,omitempty"`
	Outputs       OutputConfig           `json:"outputs" yaml:"outputs"`
	Features      FeaturesConfig         `json:"features" yaml:"features"`
	Documentation DocumentationConfig    `json:"documentation,omitempty" yaml:"documentation,omitempty"`
}

// LLMConfig configures the LLM provider and parameters
type LLMConfig struct {
	Provider    string  `json:"provider" yaml:"provider"` // "auto", "litellm", "openai", "anthropic", "ollama"
	Model       string  `json:"model" yaml:"model"`
	Temperature float64 `json:"temperature" yaml:"temperature"`
	MaxTokens   int     `json:"max_tokens" yaml:"max_tokens"`
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

// DocumentationConfig configures documentation generation behavior
type DocumentationConfig struct {
	// Enabled controls whether documentation generation is active
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Mode determines generation strategy: "full" or "incremental"
	Mode string `json:"mode" yaml:"mode"`

	// OutputDirectory is where documentation will be generated
	OutputDirectory string `json:"output_directory" yaml:"output_directory"`

	// Languages specifies which languages to document (empty = all detected)
	Languages []string `json:"languages,omitempty" yaml:"languages,omitempty"`

	// SiteGenerator specifies the static site generator: "jekyll" (default)
	SiteGenerator string `json:"site_generator" yaml:"site_generator"`

	// Theme specifies the documentation theme (e.g., "just-the-docs")
	Theme string `json:"theme" yaml:"theme"`

	// Deploy configuration for automated deployment
	Deploy DeployConfig `json:"deploy" yaml:"deploy"`

	// Features controls specific documentation features
	Features DocFeaturesConfig `json:"features" yaml:"features"`

	// Categories controls which documentation categories to generate
	Categories DocCategoriesConfig `json:"categories" yaml:"categories"`

	// Cache configuration for incremental builds
	Cache CacheConfig `json:"cache" yaml:"cache"`
}

// DeployConfig configures documentation deployment
type DeployConfig struct {
	// Enabled controls whether auto-deployment is active
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Target specifies deployment target: "github-pages", "netlify", "vercel"
	Target string `json:"target" yaml:"target"`

	// Branch specifies git branch for deployment (e.g., "gh-pages")
	Branch string `json:"branch" yaml:"branch"`

	// BaseURL is the site base URL for deployment
	BaseURL string `json:"base_url,omitempty" yaml:"base_url,omitempty"`
}

// DocFeaturesConfig controls specific documentation features
type DocFeaturesConfig struct {
	// WelcomePage enables AI-powered welcome page generation from README
	WelcomePage bool `json:"welcome_page" yaml:"welcome_page"`

	// APIReference enables API documentation generation
	APIReference bool `json:"api_reference" yaml:"api_reference"`

	// Tutorials includes tutorial documentation
	Tutorials bool `json:"tutorials" yaml:"tutorials"`

	// Architecture includes architecture documentation
	Architecture bool `json:"architecture" yaml:"architecture"`

	// Changelog includes changelog generation
	Changelog bool `json:"changelog" yaml:"changelog"`

	// Search enables search functionality in generated site
	Search bool `json:"search" yaml:"search"`
}

// DocCategoriesConfig controls documentation category visibility
type DocCategoriesConfig struct {
	// API controls API documentation category
	API bool `json:"api" yaml:"api"`

	// Tutorials controls tutorials category
	Tutorials bool `json:"tutorials" yaml:"tutorials"`

	// Architecture controls architecture docs category
	Architecture bool `json:"architecture" yaml:"architecture"`

	// Guides controls how-to guides category
	Guides bool `json:"guides" yaml:"guides"`

	// Reference controls reference documentation category
	Reference bool `json:"reference" yaml:"reference"`

	// Roadmap controls roadmap/changelog category
	Roadmap bool `json:"roadmap" yaml:"roadmap"`
}

// CacheConfig configures incremental build caching
type CacheConfig struct {
	// Enabled controls whether caching is active
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Directory specifies cache storage location
	Directory string `json:"directory" yaml:"directory"`

	// MaxAge specifies maximum cache age in hours (0 = no limit)
	MaxAge int `json:"max_age,omitempty" yaml:"max_age,omitempty"`
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
		Documentation: DocumentationConfig{
			Enabled:         true,
			Mode:            "incremental",
			OutputDirectory: "docs",
			Languages:       []string{}, // Empty = detect all
			SiteGenerator:   "jekyll",
			Theme:           "just-the-docs",
			Deploy: DeployConfig{
				Enabled: true,
				Target:  "github-pages",
				Branch:  "gh-pages",
				BaseURL: "",
			},
			Features: DocFeaturesConfig{
				WelcomePage:  true,
				APIReference: true,
				Tutorials:    true,
				Architecture: true,
				Changelog:    true,
				Search:       true,
			},
			Categories: DocCategoriesConfig{
				API:          true,
				Tutorials:    true,
				Architecture: true,
				Guides:       true,
				Reference:    true,
				Roadmap:      true,
			},
			Cache: CacheConfig{
				Enabled:   true,
				Directory: ".aurumcode/cache",
				MaxAge:    168, // 1 week in hours
			},
		},
	}
}

