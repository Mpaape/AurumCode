package prompt

// PromptParts represents structured prompt components
type PromptParts struct {
	System string            // System message/instructions
	User   string            // User message/content
	Meta   map[string]string // Metadata for tracking
}

// BuildOptions configures prompt building
type BuildOptions struct {
	MaxTokens    int    // Maximum tokens for the entire prompt
	SchemaKind   string // Type of schema: "review", "test", "docs", "summary"
	Role         string // Role context: "reviewer", "tester", "documenter"
	ReserveReply int    // Tokens to reserve for the reply
	CIContext    string // Existing CI/check context, when the caller has it
	Language     string // Human-facing review language, e.g. "pt-BR"
}

// TokenEstimator estimates token counts for text
type TokenEstimator interface {
	// Estimate returns the estimated token count for the given text
	Estimate(text string) int
}

// PriorityTier defines priority levels for context segments
type PriorityTier int

const (
	// PriorityHigh for changed functions/critical code
	PriorityHigh PriorityTier = 1
	// PriorityMedium for headers and imports
	PriorityMedium PriorityTier = 2
	// PriorityLow for comments and documentation
	PriorityLow PriorityTier = 3
)

// ContextSegment represents a piece of context with priority
type ContextSegment struct {
	Content  string       // The actual content
	Priority PriorityTier // Priority level for trimming
	SortKey  string       // Stable sort key for deterministic ordering
	Tokens   int          // Estimated token count
	FilePath string       // Path of the diff file this segment came from (AUR-467)
	IsProse  bool         // True when FilePath classified as documentation, not code (AUR-467)
}

// LanguageRules contains language-specific review rules
type LanguageRules struct {
	Language string   // Programming language
	Rules    []string // List of rules to apply
}

// Document represents additional context documents
type Document struct {
	Path    string // Document path
	Content string // Document content
	Type    string // Document type: "style-guide", "standards", "examples"
}
