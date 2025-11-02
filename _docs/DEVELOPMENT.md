---
layout: default
title: DEVELOPMENT
parent: Documentation
nav_order: 6
---

# Development Guide

Guide for developers contributing to AurumCode.

## Prerequisites

- **Go 1.21+** - [Download](https://go.dev/dl/)
- **Docker** - [Install Docker](https://docs.docker.com/get-docker/)
- **Docker Compose** - Usually bundled with Docker Desktop
- **Git** - For version control
- **Make** - Build automation (optional but recommended)
- **VS Code** or **GoLand** - Recommended IDEs

## Initial Setup

### 1. Clone Repository

```bash
git clone https://github.com/yourusername/aurumcode.git
cd aurumcode
```

### 2. Install Dependencies

```bash
# Download Go dependencies
go mod download

# Verify installation
go mod verify
```

### 3. Setup Environment

```bash
# Copy example environment file
cp .env.example .env

# Edit with your API keys
nano .env
```

Required environment variables:

```bash
# .env
# LLM Provider (at least one required)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
OLLAMA_BASE_URL=http://localhost:11434

# GitHub Integration
GITHUB_TOKEN=ghp_...
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# Optional: Override defaults
LLM_PROVIDER=openai
LLM_MODEL=gpt-4
LOG_LEVEL=debug
```

### 4. Build the Project

```bash
# Using Makefile
make build

# Or manually
go build -o bin/server cmd/server/main.go

# Using Docker
docker-compose build
```

### 5. Run Tests

```bash
# All tests
make test

# Or with Docker
docker-compose run --rm aurumcode go test ./...

# With coverage
make cover
```

## Development Workflow

### 1. Create Feature Branch

```bash
# Pull latest changes
git checkout main
git pull origin main

# Create feature branch
git checkout -b feature/your-feature-name

# Or bug fix branch
git checkout -b fix/bug-description
```

### 2. Make Changes

Follow the project structure:

```
internal/
├── git/           # GitHub integration
├── llm/           # LLM providers and orchestration
├── analyzer/      # Code analysis
├── prompt/        # Prompt building and parsing
├── reviewer/      # Review generation
├── docgen/        # Documentation generation
├── testgen/       # Test generation
└── config/        # Configuration management
```

### 3. Write Tests

**ALWAYS write tests for new code:**

```go
// internal/yourpackage/yourfile_test.go
package yourpackage

import (
    "testing"
)

func TestYourFunction(t *testing.T) {
    // Arrange
    input := "test input"

    // Act
    result := YourFunction(input)

    // Assert
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }
}
```

Run tests frequently:

```bash
# Run tests for package you're working on
go test ./internal/yourpackage/...

# With verbose output
go test -v ./internal/yourpackage/...

# Watch mode (using entr or similar)
find . -name "*.go" | entr -c go test ./internal/yourpackage/...
```

### 4. Check Code Quality

```bash
# Format code
gofmt -w .

# Run linter
go vet ./...

# Or use Makefile
make lint

# Check test coverage
make cover
```

### 5. Commit Changes

```bash
# Add files
git add .

# Commit with descriptive message
git commit -m "feat: add new feature description"

# Or for bug fixes
git commit -m "fix: resolve issue description"
```

Commit message format:

```
<type>: <description>

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `test`: Tests
- `refactor`: Code refactoring
- `perf`: Performance improvement
- `chore`: Maintenance tasks

### 6. Push and Create PR

```bash
# Push to your branch
git push origin feature/your-feature-name

# Create pull request on GitHub
gh pr create --title "Feature: Your Feature" --body "Description of changes"
```

## Running Locally

### Development Server

```bash
# Run server directly
go run cmd/server/main.go

# Or use built binary
./bin/server

# With hot reload (using air)
air

# Using Docker with live reload
docker-compose up --build
```

Server runs on `http://localhost:8080`

Endpoints:
- `GET /healthz` - Health check
- `GET /metrics` - Metrics (placeholder)
- `POST /webhook/github` - GitHub webhook receiver

### Testing Webhooks Locally

#### Option 1: Manual Curl

```bash
# Create test payload
cat > webhook-payload.json <<'EOF'
{
  "action": "opened",
  "number": 1,
  "pull_request": {
    "title": "Test PR",
    "head": {"sha": "abc123", "ref": "feature"},
    "base": {"ref": "main"}
  },
  "repository": {"full_name": "owner/repo"}
}
EOF

# Generate signature
PAYLOAD=$(cat webhook-payload.json)
SECRET="your-webhook-secret"
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/.* //')

# Send webhook
curl -X POST http://localhost:8080/webhook/github \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: pull_request" \
  -H "X-GitHub-Delivery: test-delivery-$(date +%s)" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  -d @webhook-payload.json
```

#### Option 2: Using ngrok

```bash
# Install ngrok
brew install ngrok  # macOS
# or download from ngrok.com

# Start ngrok tunnel
ngrok http 8080

# Use the ngrok URL in GitHub webhook settings
# https://xxxx-xx-xx-xx-xx.ngrok.io/webhook/github
```

#### Option 3: GitHub CLI

```bash
# Trigger workflow with gh CLI
gh workflow run test.yml
```

## Code Style Guidelines

### Go Code Style

Follow [Effective Go](https://go.dev/doc/effective_go):

```go
// ✅ Good: Clear, concise, idiomatic
func (r *Reviewer) Review(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
    metrics := r.diffAnalyzer.AnalyzeDiff(diff)
    prompt := r.promptBuilder.BuildReviewPrompt(diff, metrics)

    resp, err := r.orchestrator.Complete(ctx, prompt, llm.Options{
        MaxTokens:   4000,
        Temperature: 0.3,
    })
    if err != nil {
        return nil, fmt.Errorf("LLM request failed: %w", err)
    }

    return r.parser.ParseReviewResponse(resp.Text)
}

// ❌ Bad: No error wrapping, unclear variable names
func (r *Reviewer) Review(ctx context.Context, d *types.Diff) (*types.ReviewResult, error) {
    m := r.diffAnalyzer.AnalyzeDiff(d)
    p := r.promptBuilder.BuildReviewPrompt(d, m)
    resp, err := r.orchestrator.Complete(ctx, p, llm.Options{4000, 0.3})
    if err != nil {
        return nil, err  // Lost context!
    }
    return r.parser.ParseReviewResponse(resp.Text)
}
```

### Package Organization

```go
// ✅ Good: Clear package purpose
package reviewer

import (
    "aurumcode/internal/analyzer"
    "aurumcode/internal/llm"
    "aurumcode/pkg/types"
)

// Reviewer orchestrates code review generation
type Reviewer struct {
    orchestrator  *llm.Orchestrator
    diffAnalyzer  *analyzer.DiffAnalyzer
}

// ❌ Bad: Mixed responsibilities
package stuff

import "everything"

// Manager does everything
type Manager struct {
    // Too many responsibilities
}
```

### Error Handling

```go
// ✅ Good: Wrap errors with context
func (r *Reviewer) Review(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
    resp, err := r.orchestrator.Complete(ctx, prompt, opts)
    if err != nil {
        return nil, fmt.Errorf("LLM request failed: %w", err)
    }

    result, err := r.parser.ParseReviewResponse(resp.Text)
    if err != nil {
        return nil, fmt.Errorf("parse failed: %w", err)
    }

    return result, nil
}

// ❌ Bad: Lost error context
func (r *Reviewer) Review(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
    resp, err := r.orchestrator.Complete(ctx, prompt, opts)
    if err != nil {
        return nil, err  // Which request failed?
    }
    // ...
}
```

### Testing Style

```go
// ✅ Good: Table-driven with clear cases
func TestDetectLanguage(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"go file", "main.go", "go"},
        {"python file", "script.py", "python"},
        {"unknown", "file.xyz", "unknown"},
    }

    detector := NewLanguageDetector()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := detector.DetectLanguage(tt.input)
            if got != tt.expected {
                t.Errorf("DetectLanguage(%q) = %q, want %q",
                    tt.input, got, tt.expected)
            }
        })
    }
}

// ❌ Bad: Repetitive, unclear
func TestDetectLanguage(t *testing.T) {
    d := NewLanguageDetector()
    if d.DetectLanguage("main.go") != "go" {
        t.Error("failed")
    }
    if d.DetectLanguage("script.py") != "python" {
        t.Error("failed")
    }
    // ... more repetition
}
```

## Debugging

### Using Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug server
dlv debug cmd/server/main.go

# Debug tests
dlv test ./internal/reviewer/...

# In delve:
(dlv) break reviewer.go:42
(dlv) continue
(dlv) print result
(dlv) next
```

### VS Code Debug Configuration

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug Server",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/server/main.go",
      "env": {
        "GITHUB_TOKEN": "${env:GITHUB_TOKEN}",
        "OPENAI_API_KEY": "${env:OPENAI_API_KEY}"
      }
    },
    {
      "name": "Debug Test",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${workspaceFolder}/internal/reviewer",
      "args": ["-test.run", "TestReview"]
    }
  ]
}
```

### Logging

```go
import "log"

// Add debug logging
log.Printf("[DEBUG] Processing diff with %d files", len(diff.Files))

// Log errors with context
log.Printf("[ERROR] LLM request failed: %v", err)

// Structured logging (future: use slog)
log.Printf("[INFO] Review completed: tokens=%d cost=$%.4f",
    result.Cost.Tokens, result.Cost.CostUSD)
```

## Performance Profiling

### CPU Profiling

```bash
# Run with CPU profile
go test -cpuprofile=cpu.prof ./internal/reviewer/...

# Analyze profile
go tool pprof cpu.prof

# In pprof:
(pprof) top10
(pprof) list ReviewFunction
(pprof) web  # Opens graph in browser
```

### Memory Profiling

```bash
# Run with memory profile
go test -memprofile=mem.prof ./internal/analyzer/...

# Analyze
go tool pprof mem.prof

(pprof) top10
(pprof) list AnalyzeDiff
```

### Benchmarks

```go
// analyzer_bench_test.go
func BenchmarkAnalyzeDiff(b *testing.B) {
    analyzer := NewDiffAnalyzer()
    diff := loadLargeDiff()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        analyzer.AnalyzeDiff(diff)
    }
}
```

Run benchmarks:

```bash
go test -bench=. ./internal/analyzer/...

# With memory allocation stats
go test -bench=. -benchmem ./internal/analyzer/...
```

## Common Tasks

### Adding a New LLM Provider

1. Create provider package:

```go
// internal/llm/provider/newprovider/provider.go
package newprovider

import "aurumcode/internal/llm"

type Provider struct {
    apiKey  string
    baseURL string
}

func NewProvider(apiKey, baseURL string) *Provider {
    return &Provider{apiKey: apiKey, baseURL: baseURL}
}

func (p *Provider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
    // Implementation
}

func (p *Provider) Tokens(input string) (int, error) {
    // Token counting
}

func (p *Provider) Name() string {
    return "newprovider"
}
```

2. Add tests:

```go
// internal/llm/provider/newprovider/provider_test.go
func TestProvider_Complete(t *testing.T) {
    // Test implementation
}
```

3. Register in orchestrator:

```go
// cmd/server/main.go
provider := newprovider.NewProvider(apiKey, baseURL)
orchestrator := llm.NewOrchestrator(provider, fallbacks, tracker)
```

### Adding a New Language

1. Update language detector:

```go
// internal/analyzer/language.go
func (d *LanguageDetector) DetectLanguage(filePath string) string {
    ext := filepath.Ext(filePath)
    switch ext {
    case ".newlang":
        return "newlang"
    // ...
    }
}
```

2. Add function extraction (optional):

```go
func (a *DiffAnalyzer) extractNewLangFunctions(file *types.DiffFile) []string {
    pattern := regexp.MustCompile(`func\s+(\w+)`)
    // Extract functions
}
```

3. Add tests:

```go
func TestDetectLanguage_NewLang(t *testing.T) {
    detector := NewLanguageDetector()
    lang := detector.DetectLanguage("file.newlang")
    if lang != "newlang" {
        t.Errorf("expected 'newlang', got %s", lang)
    }
}
```

## Troubleshooting

### Build Failures

```bash
# Clear build cache
go clean -cache

# Update dependencies
go mod tidy
go mod download

# Verify module integrity
go mod verify
```

### Test Failures

```bash
# Clear test cache
go clean -testcache

# Run specific failing test
go test -v -run TestNameThatFails ./path/to/package/...

# With race detector
go test -race ./...
```

### Docker Issues

```bash
# Rebuild without cache
docker-compose build --no-cache

# Remove all containers and volumes
docker-compose down -v

# View logs
docker-compose logs -f aurumcode

# Access container shell
docker-compose run --rm aurumcode sh
```

## Resources

### Documentation
- [Go Documentation](https://go.dev/doc/)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Tools
- [Go Playground](https://go.dev/play/)
- [golangci-lint](https://golangci-lint.run/)
- [govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)

### Testing
- [Testing Package](https://pkg.go.dev/testing)
- [Testify](https://github.com/stretchr/testify) (optional)

## Getting Help

- **Project Issues**: [GitHub Issues](https://github.com/yourusername/aurumcode/issues)
- **Discussions**: [GitHub Discussions](https://github.com/yourusername/aurumcode/discussions)
- **Architecture Questions**: See [ARCHITECTURE.md](./ARCHITECTURE.md)
- **Testing Help**: See [TESTING.md](./TESTING.md)

## Next Steps

- Read the [Architecture Overview](./ARCHITECTURE.md)
- Review the [Testing Guide](./TESTING.md)
- Check out [open issues](https://github.com/yourusername/aurumcode/issues)
- Join our [discussions](https://github.com/yourusername/aurumcode/discussions)
