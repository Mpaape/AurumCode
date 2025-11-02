package types

// Options represents LLM request options
type Options struct {
	Temperature float64            `json:"temperature"`
	MaxTokens   int                `json:"max_tokens"`
	Model       string             `json:"model,omitempty"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
}

// Response represents an LLM response
type Response struct {
	Content     string `json:"content"`
	TokensUsed  int    `json:"tokens_used"`
	Model       string `json:"model"`
	Provider    string `json:"provider"`
}

// Provider defines the interface for LLM providers
type Provider interface {
	Complete(prompt string, opts Options) (Response, error)
	Tokens(input string) (int, error)
	Name() string
}

// GitClient defines the interface for Git provider interactions
type GitClient interface {
	GetPullRequestDiff(repo, owner string, prNumber int) (*Diff, error)
	ListChangedFiles(repo, owner string, prNumber int) ([]string, error)
	PostReviewComment(repo, owner string, prNumber int, comment ReviewComment) error
	SetStatus(repo, owner, sha, status, context, description string) error
}

// ReviewComment is defined in types.go

// CostTracker defines the interface for budget management
type CostTracker interface {
	Allow(costUSD float64) bool
	Spend(costUSD float64) error
	Remaining() float64
}

// PromptBuilder defines the interface for constructing LLM prompts
type PromptBuilder interface {
	Build(diff *Diff, config *Config) (string, error)
	EstimateTokens(prompt string) int
}

// ResponseParser defines the interface for parsing LLM responses
type ResponseParser interface {
	ParseJSON(content string, schema interface{}) error
	ParseMarkdown(content string) (string, error)
}

