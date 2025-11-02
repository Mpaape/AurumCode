---
layout: default
title: ARCHITECTURE
parent: Documentation
nav_order: 3
---

# Architecture Overview

AurumCode follows a **Hexagonal Architecture** (Ports and Adapters) pattern for clean separation of concerns and testability.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        External Layer                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   GitHub     │  │  LLM APIs    │  │   Storage    │      │
│  │   Webhooks   │  │  (OpenAI,    │  │  (Future)    │      │
│  │              │  │  Anthropic)  │  │              │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
└─────────┼──────────────────┼──────────────────┼─────────────┘
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼─────────────┐
│         │   Adapter Layer (Infrastructure)    │             │
│  ┌──────▼───────┐  ┌──────▼───────┐  ┌───────▼──────┐      │
│  │  GitClient   │  │   Provider   │  │  (Reserved)  │      │
│  │  Adapter     │  │   Adapters   │  │              │      │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘      │
└─────────┼──────────────────┼──────────────────────────────┐
          │                  │                               │
┌─────────┼──────────────────┼───────────────────────────────┤
│         │      Core Domain (Application Layer)             │
│  ┌──────▼─────────────────┐                                │
│  │   Orchestrator         │   ┌──────────────────┐         │
│  │   (Coordinates)        │◀──│  Cost Tracker    │         │
│  └──────┬─────────────────┘   └──────────────────┘         │
│         │                                                   │
│  ┌──────▼─────────────────┐                                │
│  │   Review Pipeline      │                                │
│  │  ┌─────────────────┐   │   ┌──────────────────┐        │
│  │  │ Diff Analyzer   │   │   │  Prompt Builder  │        │
│  │  └─────────────────┘   │   └──────────────────┘        │
│  │  ┌─────────────────┐   │   ┌──────────────────┐        │
│  │  │ Language Detect │   │   │ Response Parser  │        │
│  │  └─────────────────┘   │   └──────────────────┘        │
│  └────────────────────────┘                                │
│                                                             │
│  ┌──────────────────┐   ┌──────────────────┐              │
│  │  Doc Generator   │   │  Test Generator  │              │
│  └──────────────────┘   └──────────────────┘              │
└─────────────────────────────────────────────────────────────┘
          │
┌─────────▼───────────────────────────────────────────────────┐
│                      Types Layer                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  Config  │  │   Diff   │  │  Review  │  │  Event   │   │
│  │          │  │          │  │  Result  │  │          │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Core Principles

### 1. Dependency Inversion

All dependencies point inward. Domain logic never depends on infrastructure.

```go
// ✅ Good: Domain defines interface
package reviewer

type LLMProvider interface {
    Complete(ctx context.Context, prompt string, opts Options) (Response, error)
}

type Reviewer struct {
    llm LLMProvider  // Depends on interface, not concrete type
}

// ✅ Good: Infrastructure implements interface
package openai

type Provider struct { /* ... */ }

func (p *Provider) Complete(ctx context.Context, prompt string, opts Options) (Response, error) {
    // Implementation details
}
```

```go
// ❌ Bad: Domain depends on infrastructure
package reviewer

import "aurumcode/internal/llm/provider/openai"  // Direct dependency!

type Reviewer struct {
    llm *openai.Provider  // Coupled to concrete type
}
```

### 2. Ports and Adapters

**Ports** = Interfaces defined in domain
**Adapters** = Concrete implementations in infrastructure

```go
// Port (defined in domain)
type GitClient interface {
    GetPullRequestDiff(ctx context.Context, owner, repo string, number int) (*Diff, error)
    PostReviewComment(ctx context.Context, owner, repo string, number int, comment ReviewComment) error
}

// Adapter (infrastructure)
type GitHubClient struct {
    token  string
    client *http.Client
}

func (c *GitHubClient) GetPullRequestDiff(...) (*Diff, error) {
    // GitHub-specific implementation
}
```

### 3. Separation of Concerns

Each component has a single, well-defined responsibility.

## Package Structure

```
aurumcode/
├── cmd/
│   ├── server/          # HTTP server entry point
│   └── cli/             # CLI tool (future)
├── internal/
│   ├── git/
│   │   ├── githubclient/   # GitHub API adapter
│   │   └── webhook/        # Webhook parser
│   ├── llm/
│   │   ├── orchestrator.go # LLM orchestration
│   │   ├── cost/           # Budget tracking
│   │   └── provider/       # LLM adapters
│   │       ├── openai/
│   │       ├── anthropic/
│   │       └── ollama/
│   ├── analyzer/
│   │   ├── diff.go         # Diff parsing
│   │   └── language.go     # Language detection
│   ├── prompt/
│   │   ├── builder.go      # Prompt construction
│   │   └── parser.go       # Response parsing
│   ├── reviewer/           # Review orchestration
│   ├── docgen/             # Doc generation
│   ├── testgen/            # Test generation
│   └── config/             # Configuration
├── pkg/
│   └── types/              # Shared domain types
└── tests/
    ├── fixtures/           # Test data
    └── integration/        # Integration tests
```

## Component Deep Dive

### 1. HTTP Server

Entry point for GitHub webhooks.

```go
// cmd/server/main.go
func main() {
    cfg := config.Load(".aurumcode/config.yml")

    // Create dependencies
    tracker := cost.NewTracker(cfg.Budgets.PerRun, cfg.Budgets.Daily, priceMap)
    primary := openai.NewProvider(cfg.LLM.APIKey, cfg.LLM.Model)
    orchestrator := llm.NewOrchestrator(primary, fallbacks, tracker)

    // Create handlers
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", healthHandler)
    mux.HandleFunc("/webhook/github", webhookHandler(orchestrator, cfg))

    // Wrap with middleware
    handler := requestID(logging(recovery(mux)))

    server := &http.Server{
        Addr:    ":8080",
        Handler: handler,
    }

    log.Fatal(server.ListenAndServe())
}
```

**Key Features:**
- Middleware chain (request ID, logging, recovery)
- HMAC signature validation
- Idempotency via delivery ID cache
- Graceful shutdown

### 2. LLM Orchestrator

Manages LLM provider chains with fallbacks and budgets.

```go
// internal/llm/orchestrator.go
type Orchestrator struct {
    primary   Provider
    fallbacks []Provider
    tracker   *cost.Tracker
}

func (o *Orchestrator) Complete(ctx context.Context, prompt string, opts Options) (Response, error) {
    // Estimate tokens
    tokensIn, _ := o.primary.Tokens(prompt)
    tokensOut := opts.MaxTokens

    // Check budget
    if !o.tracker.Allow(tokensIn, tokensOut, opts.Model) {
        return Response{}, ErrBudgetExceeded
    }

    // Try primary provider
    resp, err := o.primary.Complete(prompt, opts)
    if err == nil {
        o.tracker.Spend(resp.TokensIn, resp.TokensOut, resp.Model)
        return resp, nil
    }

    // Try fallbacks
    for _, fallback := range o.fallbacks {
        resp, err = fallback.Complete(prompt, opts)
        if err == nil {
            o.tracker.Spend(resp.TokensIn, resp.TokensOut, resp.Model)
            return resp, nil
        }
    }

    return Response{}, fmt.Errorf("all providers failed: %w", err)
}
```

**Key Features:**
- Provider abstraction
- Fallback chain
- Budget enforcement
- Token tracking
- Context propagation

### 3. Review Pipeline

Orchestrates the complete review flow.

```go
// internal/reviewer/reviewer.go
type Reviewer struct {
    orchestrator  *llm.Orchestrator
    diffAnalyzer  *analyzer.DiffAnalyzer
    promptBuilder *prompt.PromptBuilder
    parser        *prompt.ResponseParser
}

func (r *Reviewer) Review(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
    // 1. Analyze the diff
    metrics := r.diffAnalyzer.AnalyzeDiff(diff)

    // 2. Build the prompt
    reviewPrompt := r.promptBuilder.BuildReviewPrompt(diff, metrics)

    // 3. Call LLM
    resp, err := r.orchestrator.Complete(ctx, reviewPrompt, llm.Options{
        MaxTokens:   4000,
        Temperature: 0.3,
    })
    if err != nil {
        return nil, fmt.Errorf("LLM request failed: %w", err)
    }

    // 4. Parse response
    result, err := r.parser.ParseReviewResponse(resp.Text)
    if err != nil {
        return nil, fmt.Errorf("parse failed: %w", err)
    }

    // 5. Add cost info
    result.Cost = types.CostSummary{
        Tokens:   resp.TokensIn + resp.TokensOut,
        CostUSD:  float64(resp.TokensIn+resp.TokensOut) * 0.0001,
        Provider: "llm",
        Model:    resp.Model,
    }

    return result, nil
}
```

**Flow:**
1. **Diff Analysis** → Extract metrics, functions, complexity
2. **Prompt Building** → Construct structured prompt with context
3. **LLM Call** → Execute via orchestrator with fallbacks
4. **Response Parsing** → Extract JSON, validate structure
5. **Result Assembly** → Add cost, metadata, return

### 4. Diff Analyzer

Parses unified diffs and extracts metrics.

```go
// internal/analyzer/diff.go
type DiffAnalyzer struct {
    languageDetector *LanguageDetector
}

func (a *DiffAnalyzer) AnalyzeDiff(diff *types.Diff) *DiffMetrics {
    metrics := &DiffMetrics{
        TotalFiles:        len(diff.Files),
        LanguageBreakdown: make(map[string]int),
    }

    for _, file := range diff.Files {
        // Detect language
        lang := a.languageDetector.DetectLanguage(file.Path)
        metrics.LanguageBreakdown[lang]++

        // Count line changes
        for _, hunk := range file.Hunks {
            for _, line := range hunk.Lines {
                switch line[0] {
                case '+':
                    metrics.LinesAdded++
                case '-':
                    metrics.LinesDeleted++
                }
            }
        }

        // Extract functions
        if !a.languageDetector.IsTestFile(file.Path) {
            functions := a.ExtractChangedFunctions(&file)
            metrics.ChangedFunctions = append(metrics.ChangedFunctions, functions...)
        }
    }

    return metrics
}
```

**Capabilities:**
- Language detection (30+ languages)
- Line change counting
- Function extraction (Go, JS, Python, Java)
- Test file detection
- Config file detection
- Complexity scoring

### 5. Prompt Builder

Constructs structured prompts with token budgeting.

```go
// internal/prompt/builder.go
func (b *PromptBuilder) BuildReviewPrompt(diff *types.Diff, metrics *DiffMetrics) string {
    languages := b.getLanguageList(metrics)

    prompt := fmt.Sprintf(`You are an expert code reviewer analyzing changes in: %s

## Code Review Task

Analyze the following diff and provide:
1. Issues with severity (error/warning/info)
2. ISO/IEC 25010 quality scores (1-10)
3. Brief summary

## Diff Summary
- Total files: %d
- Lines added: %d
- Lines deleted: %d
- Languages: %s

## Diff Content
%s

## Response Format

Respond with JSON only:
{
  "issues": [
    {
      "file": "path/to/file",
      "line": 42,
      "severity": "error",
      "rule_id": "security/sql-injection",
      "message": "Issue description",
      "suggestion": "How to fix"
    }
  ],
  "iso_scores": {
    "functionality": 8,
    "reliability": 7,
    "usability": 9,
    "efficiency": 8,
    "maintainability": 7,
    "portability": 9,
    "security": 6,
    "compatibility": 8
  },
  "summary": "Overall assessment..."
}
`,
        strings.Join(languages, ", "),
        metrics.TotalFiles,
        metrics.LinesAdded,
        metrics.LinesDeleted,
        strings.Join(languages, ", "),
        b.formatDiff(diff),
    )

    return prompt
}
```

**Features:**
- Structured templates
- Token estimation
- Context prioritization
- Schema enforcement
- Multiple prompt types (review, docs, tests)

### 6. Response Parser

Extracts and validates LLM responses.

```go
// internal/prompt/parser.go
func (p *ResponseParser) ParseReviewResponse(response string) (*types.ReviewResult, error) {
    // Extract JSON from markdown code blocks
    jsonContent := p.extractJSON(response)

    if jsonContent == "" {
        return nil, fmt.Errorf("no JSON found in response")
    }

    // Parse JSON
    var result types.ReviewResult
    if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
        return nil, fmt.Errorf("failed to parse JSON: %w", err)
    }

    // Validate
    if err := p.validateReviewResult(&result); err != nil {
        return nil, err
    }

    return &result, nil
}
```

**Features:**
- JSON extraction from markdown
- Code fence handling
- Schema validation
- Error recovery
- Deterministic parsing

## Data Flow Example

Let's trace a complete PR review:

```
1. GitHub sends webhook
   POST /webhook/github
   {
     "action": "opened",
     "pull_request": { ... }
   }
        │
        ▼
2. Webhook handler validates signature
   HMAC-SHA256 with secret
        │
        ▼
3. Parse event into types.Event
   Extract: repo, PR number, SHA
        │
        ▼
4. Check idempotency cache
   Skip if already processed
        │
        ▼
5. Fetch PR diff via GitHub API
   GET /repos/{owner}/{repo}/pulls/{number}
   Accept: application/vnd.github.v3.diff
        │
        ▼
6. Parse diff into types.Diff
   Extract files, hunks, lines
        │
        ▼
7. Analyze diff
   • Detect languages
   • Count changes
   • Extract functions
   • Calculate complexity
        │
        ▼
8. Build review prompt
   • Add context
   • Format diff
   • Include instructions
        │
        ▼
9. Send to LLM orchestrator
   • Check budget
   • Try primary provider
   • Fallback if needed
   • Track cost
        │
        ▼
10. Parse LLM response
    • Extract JSON
    • Validate schema
    • Map issues to lines
        │
        ▼
11. Post review to GitHub
    • Create review comments
    • Set commit status
    • Update PR
        │
        ▼
12. Return response
    200 OK
```

## Testing Strategy

### Unit Tests

Test individual components in isolation with mocks:

```go
func TestReviewer_Review(t *testing.T) {
    // Mock LLM provider
    mock := &mockProvider{
        response: `{"issues":[],"iso_scores":{...},"summary":"Good"}`,
    }

    // Create dependencies
    tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
    orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
    reviewer := NewReviewer(orchestrator)

    // Test
    result, err := reviewer.Review(context.Background(), testDiff)
    assert.NoError(t, err)
    assert.Equal(t, 100, result.Cost.Tokens)
}
```

### Integration Tests

Test component interactions with real implementations:

```go
func TestIntegration_ReviewPipeline(t *testing.T) {
    // Real components
    cfg := config.LoadDefaults()
    provider := openai.NewProvider(testAPIKey, "gpt-4")
    orchestrator := llm.NewOrchestrator(provider, nil, tracker)

    // Real flow
    reviewer := NewReviewer(orchestrator)
    result, err := reviewer.Review(ctx, realDiff)

    // Verify end-to-end
    assert.NoError(t, err)
    assert.NotEmpty(t, result.Issues)
}
```

### Docker-Based Tests

All tests run in Docker for consistency:

```bash
docker-compose run --rm aurumcode go test ./...
```

## Configuration System

Layered configuration with precedence:

```
1. Defaults (built-in)
   ↓
2. File (.aurumcode/config.yml)
   ↓
3. Environment variables
   ↓
4. Runtime overrides
```

```go
// internal/config/loader.go
func Load(path string) (Config, error) {
    // 1. Start with defaults
    cfg := LoadDefaults()

    // 2. Merge file config
    if fileExists(path) {
        fileCfg := parseYAML(path)
        cfg = Merge(cfg, fileCfg)
    }

    // 3. Apply environment overrides
    cfg = ApplyEnv(cfg)

    // 4. Validate
    if err := Validate(cfg); err != nil {
        return Config{}, err
    }

    return cfg, nil
}
```

## Extensibility Points

### Adding a New LLM Provider

1. Implement the `Provider` interface:

```go
package myprovider

import "aurumcode/internal/llm"

type MyProvider struct {
    apiKey string
    model  string
}

func (p *MyProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
    // Your API call here
}

func (p *MyProvider) Tokens(input string) (int, error) {
    // Your token counting here
}

func (p *MyProvider) Name() string {
    return "myprovider"
}
```

2. Register in orchestrator:

```go
provider := myprovider.NewProvider(apiKey, model)
orchestrator := llm.NewOrchestrator(provider, fallbacks, tracker)
```

### Adding a New Language

1. Add to language detector:

```go
// internal/analyzer/language.go
func (d *LanguageDetector) DetectLanguage(filePath string) string {
    ext := filepath.Ext(filePath)
    switch ext {
    case ".mynew":
        return "mynewlang"
    // ...
    }
}
```

2. Add function extraction pattern (optional):

```go
func (a *DiffAnalyzer) extractMyNewLangFunctions(file *types.DiffFile) []string {
    pattern := regexp.MustCompile(`func\s+(\w+)`)
    // Extract function names
}
```

## Performance Considerations

### Caching

- **ETag caching** for GitHub API responses
- **Idempotency cache** for webhook deduplication
- **Token count cache** for prompt reuse (future)

### Rate Limiting

- **GitHub API**: Respects `Retry-After` headers
- **LLM providers**: Exponential backoff with jitter
- **Budget limits**: Per-run and daily caps

### Concurrency

- **Webhook processing**: Concurrent handler invocations
- **Thread-safe**: All caches use `sync.RWMutex`
- **Context propagation**: Cancellation support throughout

## Security

### Input Validation

- HMAC signature verification for webhooks
- Schema validation for all inputs
- Sanitization of user-provided data

### Secret Management

- Never log API keys or secrets
- Redaction in HTTP request/response logs
- Environment-based configuration

### Rate Limiting

- Budget enforcement prevents runaway costs
- Idempotency prevents duplicate processing
- Timeout on all external calls

## Next Steps

- [Development Setup](./DEVELOPMENT.md)
- [Testing Guide](./TESTING.md)
- [Adding Features](./EXTENDING.md)
- [API Reference](./API_REFERENCE.md)
