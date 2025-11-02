---
layout: default
title: API REFERENCE
parent: Documentation
nav_order: 10
---

# API Reference

Quick reference for AurumCode Go packages.

## HTTP Endpoints

### GET /healthz

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### GET /metrics

Metrics endpoint (placeholder).

**Response:**
```json
{
  "status": "ok"
}
```

### POST /webhook/github

GitHub webhook receiver.

**Headers:**
- `Content-Type: application/json`
- `X-GitHub-Event: pull_request` or `push`
- `X-GitHub-Delivery: unique-delivery-id`
- `X-Hub-Signature-256: sha256=...`

**Body:** GitHub webhook payload

**Response:**
- `200 OK` - Webhook processed successfully
- `401 Unauthorized` - Invalid signature
- `400 Bad Request` - Malformed payload

## Core Packages

### pkg/types

Shared domain types used across the application.

#### Config

```go
type Config struct {
    LLM LLMConfig
    Output OutputConfig
}

type LLMConfig struct {
    Provider    string
    Model       string
    Temperature float64
    MaxTokens   int
    Budgets     BudgetConfig
}
```

#### Diff

```go
type Diff struct {
    Files []DiffFile
}

type DiffFile struct {
    Path  string
    Hunks []DiffHunk
}

type DiffHunk struct {
    OldStart int
    OldLines int
    NewStart int
    NewLines int
    Lines    []string
}
```

#### ReviewResult

```go
type ReviewResult struct {
    Issues    []ReviewIssue
    ISOScores ISOScores
    Summary   string
    Cost      CostSummary
}

type ReviewIssue struct {
    File       string
    Line       int
    Severity   string // "error", "warning", "info"
    RuleID     string
    Message    string
    Suggestion string
}

type ISOScores struct {
    Functionality   int // 1-10
    Reliability     int
    Usability       int
    Efficiency      int
    Maintainability int
    Portability     int
    Security        int
    Compatibility   int
}
```

## Internal Packages

### internal/llm

LLM provider abstraction and orchestration.

#### Provider Interface

```go
type Provider interface {
    Complete(prompt string, opts Options) (Response, error)
    Tokens(input string) (int, error)
    Name() string
}

type Options struct {
    MaxTokens   int
    Temperature float64
    System      string
}

type Response struct {
    Text      string
    TokensIn  int
    TokensOut int
    Model     string
}
```

#### Orchestrator

```go
type Orchestrator struct {}

func NewOrchestrator(
    primary Provider,
    fallbacks []Provider,
    tracker *cost.Tracker,
) *Orchestrator

func (o *Orchestrator) Complete(
    ctx context.Context,
    prompt string,
    opts Options,
) (Response, error)
```

**Example:**

```go
// Create provider
provider := openai.NewProvider(apiKey, "gpt-4")

// Create cost tracker
tracker := cost.NewTracker(1.0, 10.0, priceMap)

// Create orchestrator with fallbacks
orchestrator := llm.NewOrchestrator(
    provider,
    []llm.Provider{fallbackProvider},
    tracker,
)

// Use orchestrator
resp, err := orchestrator.Complete(ctx, prompt, llm.Options{
    MaxTokens:   4000,
    Temperature: 0.3,
})
```

### internal/llm/cost

Budget tracking and enforcement.

#### CostTracker

```go
type Tracker struct {}

func NewTracker(
    perRunUSD float64,
    dailyUSD float64,
    prices map[string]PriceMap,
) *Tracker

func (t *Tracker) Allow(tokensIn, tokensOut int, model string) bool
func (t *Tracker) Spend(tokensIn, tokensOut int, model string) error
func (t *Tracker) Remaining() (float64, float64)
func (t *Tracker) ResetPerRun()
```

**Example:**

```go
// Create tracker
priceMap := map[string]cost.PriceMap{
    "gpt-4": {
        InputPer1K:  0.03,
        OutputPer1K: 0.06,
    },
}
tracker := cost.NewTracker(1.0, 10.0, priceMap)

// Check budget before request
if !tracker.Allow(1000, 2000, "gpt-4") {
    return errors.New("budget exceeded")
}

// Record actual spend
tracker.Spend(900, 1800, "gpt-4")

// Check remaining
perRun, daily := tracker.Remaining()
```

### internal/reviewer

Review generation orchestration.

#### Reviewer

```go
type Reviewer struct {}

func NewReviewer(orchestrator *llm.Orchestrator) *Reviewer

func (r *Reviewer) Review(
    ctx context.Context,
    diff *types.Diff,
) (*types.ReviewResult, error)
```

**Example:**

```go
// Create reviewer
reviewer := NewReviewer(orchestrator)

// Generate review
result, err := reviewer.Review(ctx, diff)
if err != nil {
    log.Fatal(err)
}

// Use result
for _, issue := range result.Issues {
    fmt.Printf("%s:%d [%s] %s\n",
        issue.File, issue.Line, issue.Severity, issue.Message)
}
```

### internal/docgen

Documentation generation.

#### Generator

```go
type Generator struct {}

func NewGenerator(orchestrator *llm.Orchestrator) *Generator

func (g *Generator) Generate(
    ctx context.Context,
    diff *types.Diff,
) (string, error)
```

**Example:**

```go
// Create generator
generator := NewGenerator(orchestrator)

// Generate docs
docs, err := generator.Generate(ctx, diff)
if err != nil {
    log.Fatal(err)
}

// Write to file
os.WriteFile("DOCS.md", []byte(docs), 0644)
```

### internal/testgen

Test generation.

#### Generator

```go
type Generator struct {}

func NewGenerator(orchestrator *llm.Orchestrator) *Generator

func (g *Generator) Generate(
    ctx context.Context,
    diff *types.Diff,
) (string, error)
```

**Example:**

```go
// Create generator
generator := NewGenerator(orchestrator)

// Generate tests
tests, err := generator.Generate(ctx, diff)
if err != nil {
    log.Fatal(err)
}

// Write to test file
os.WriteFile("service_test.go", []byte(tests), 0644)
```

### internal/analyzer

Code analysis and diff parsing.

#### DiffAnalyzer

```go
type DiffAnalyzer struct {}

func NewDiffAnalyzer() *DiffAnalyzer

func (a *DiffAnalyzer) AnalyzeDiff(diff *types.Diff) *DiffMetrics

type DiffMetrics struct {
    TotalFiles        int
    LinesAdded        int
    LinesDeleted      int
    TestFiles         int
    ConfigFiles       int
    LanguageBreakdown map[string]int
    ChangedFunctions  []string
}
```

**Example:**

```go
// Create analyzer
analyzer := NewDiffAnalyzer()

// Analyze diff
metrics := analyzer.AnalyzeDiff(diff)

fmt.Printf("Files: %d, +%d -%d lines\n",
    metrics.TotalFiles,
    metrics.LinesAdded,
    metrics.LinesDeleted,
)
```

#### LanguageDetector

```go
type LanguageDetector struct {}

func NewLanguageDetector() *LanguageDetector

func (d *LanguageDetector) DetectLanguage(filePath string) string
func (d *LanguageDetector) IsTestFile(filePath string) bool
func (d *LanguageDetector) IsConfigFile(filePath string) bool
```

**Example:**

```go
// Create detector
detector := NewLanguageDetector()

// Detect language
lang := detector.DetectLanguage("main.go")  // "go"
isTest := detector.IsTestFile("main_test.go")  // true
```

### internal/prompt

Prompt building and response parsing.

#### PromptBuilder

```go
type PromptBuilder struct {}

func NewPromptBuilder() *PromptBuilder

func (b *PromptBuilder) BuildReviewPrompt(
    diff *types.Diff,
    metrics *analyzer.DiffMetrics,
) string

func (b *PromptBuilder) BuildDocumentationPrompt(
    diff *types.Diff,
    language string,
) string

func (b *PromptBuilder) BuildTestPrompt(
    diff *types.Diff,
    language string,
) string
```

#### ResponseParser

```go
type ResponseParser struct {}

func NewResponseParser() *ResponseParser

func (p *ResponseParser) ParseReviewResponse(
    response string,
) (*types.ReviewResult, error)

func (p *ResponseParser) ParseDocumentationResponse(
    response string,
) (string, error)

func (p *ResponseParser) ParseTestResponse(
    response string,
    language string,
) (string, error)
```

### internal/git/githubclient

GitHub API client.

#### Client

```go
type Client struct {}

func NewClient(token, baseURL string) *Client

func (c *Client) GetPullRequestDiff(
    ctx context.Context,
    owner, repo string,
    number int,
) (*types.Diff, error)

func (c *Client) ListChangedFiles(
    ctx context.Context,
    owner, repo string,
    number int,
) ([]string, error)

func (c *Client) PostReviewComment(
    ctx context.Context,
    owner, repo string,
    number int,
    comment ReviewComment,
    idempotencyKey string,
) error

func (c *Client) SetStatus(
    ctx context.Context,
    owner, repo, sha string,
    status CommitStatus,
) error
```

**Example:**

```go
// Create client
client := githubclient.NewClient(token, "")

// Get PR diff
diff, err := client.GetPullRequestDiff(ctx, "owner", "repo", 123)

// Post comment
err = client.PostReviewComment(ctx, "owner", "repo", 123,
    githubclient.ReviewComment{
        Body: "Issue found here",
        Path: "main.go",
        Line: 42,
    },
    "review-123-issue-1",
)

// Set status
err = client.SetStatus(ctx, "owner", "repo", "abc123",
    githubclient.CommitStatus{
        State:       "success",
        Context:     "aurumcode/review",
        Description: "Review complete",
    },
)
```

### internal/git/webhook

Webhook parsing and validation.

#### Parser

```go
func Parse(
    eventType string,
    deliveryID string,
    payload []byte,
) (*types.Event, error)

func ValidateGitHubSignature(
    payload []byte,
    signature string,
    secret string,
) error
```

**Example:**

```go
// Validate signature
err := webhook.ValidateGitHubSignature(
    payloadBytes,
    r.Header.Get("X-Hub-Signature-256"),
    secret,
)
if err != nil {
    http.Error(w, "invalid signature", 401)
    return
}

// Parse event
event, err := webhook.Parse(
    r.Header.Get("X-GitHub-Event"),
    r.Header.Get("X-GitHub-Delivery"),
    payloadBytes,
)
```

#### IdempotencyCache

```go
type IdempotencyCache struct {}

func NewIdempotencyCache(
    maxSize int,
    ttl time.Duration,
) *IdempotencyCache

func (c *IdempotencyCache) SeenOrAdd(id string) bool
func (c *IdempotencyCache) Contains(id string) bool
func (c *IdempotencyCache) Clear()
```

**Example:**

```go
// Create cache
cache := webhook.NewIdempotencyCache(1000, 5*time.Minute)

// Check and add
if cache.SeenOrAdd(deliveryID) {
    // Already processed, skip
    return
}

// Process webhook
processWebhook(event)
```

### internal/config

Configuration management.

#### Loader

```go
func Load(path string) (types.Config, error)
func LoadDefaults() types.Config
func Validate(cfg types.Config) error
```

**Example:**

```go
// Load config
cfg, err := config.Load(".aurumcode/config.yml")
if err != nil {
    log.Fatal(err)
}

// Access config
provider := cfg.LLM.Provider
model := cfg.LLM.Model
```

## Error Handling

All packages return standard Go errors. Use `errors.Is()` and `errors.As()` for error checking:

```go
if errors.Is(err, llm.ErrBudgetExceeded) {
    // Handle budget exceeded
}

var apiErr *githubclient.APIError
if errors.As(err, &apiErr) {
    // Handle GitHub API error
    fmt.Printf("Status: %d, Message: %s\n", apiErr.StatusCode, apiErr.Message)
}
```

## Context Handling

All long-running operations accept `context.Context`:

```go
// Set timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Use context
result, err := reviewer.Review(ctx, diff)
if errors.Is(err, context.DeadlineExceeded) {
    // Handle timeout
}
```

## Logging

Use standard library `log` package. Structured logging coming soon:

```go
log.Printf("[INFO] Processing PR #%d", prNumber)
log.Printf("[ERROR] LLM request failed: %v", err)
log.Printf("[DEBUG] Metrics: %+v", metrics)
```

## Testing Utilities

### Mock Provider

```go
type mockProvider struct {
    response string
    err      error
}

func (m *mockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
    if m.err != nil {
        return llm.Response{}, m.err
    }
    return llm.Response{
        Text:      m.response,
        TokensIn:  50,
        TokensOut: 50,
        Model:     "test",
    }, nil
}
```

### Test Helpers

```go
// Load test fixtures
func loadTestDiff(t *testing.T) *types.Diff {
    data, err := os.ReadFile("testdata/simple.diff")
    if err != nil {
        t.Fatal(err)
    }
    // Parse and return
}

// Create test config
func testConfig() types.Config {
    return types.Config{
        LLM: types.LLMConfig{
            Provider:    "test",
            Model:       "test-model",
            Temperature: 0.0,
        },
    }
}
```

## Version Compatibility

- **Go**: 1.21 or higher required
- **Docker**: 20.10+ recommended
- **GitHub API**: v3 (REST API)

## Rate Limits

- **GitHub API**: 5000 requests/hour (authenticated)
- **LLM providers**: Varies by provider
- **Cost budget**: Configurable in `.aurumcode/config.yml`

## Further Reading

- [Architecture Overview](./ARCHITECTURE.md)
- [Development Guide](./DEVELOPMENT.md)
- [Testing Guide](./TESTING.md)
- [Go Package Documentation](https://pkg.go.dev/aurumcode)
