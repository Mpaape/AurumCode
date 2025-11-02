---
layout: default
title: TESTING
parent: Documentation
nav_order: 10
---

# Testing Guide

Comprehensive guide to testing AurumCode.

## Overview

AurumCode uses a multi-layered testing strategy:

- **Unit Tests**: Test individual functions/methods in isolation
- **Integration Tests**: Test component interactions
- **Docker Tests**: Ensure consistency across environments
- **Golden Tests**: Validate deterministic outputs
- **Coverage**: Target ≥80% for critical packages

## Running Tests

### All Tests

```bash
# Using Docker (recommended)
docker-compose run --rm aurumcode go test ./...

# Using local Go
go test ./...

# With verbose output
go test -v ./...

# Specific package
go test ./internal/reviewer/...
```

### With Coverage

```bash
# Generate coverage report
docker-compose run --rm aurumcode go test -cover ./internal/...

# Detailed coverage by package
docker-compose run --rm aurumcode go test -coverprofile=coverage.out ./...
docker-compose run --rm aurumcode go tool cover -func=coverage.out

# HTML coverage report
docker-compose run --rm aurumcode go tool cover -html=coverage.out -o coverage.html
```

### Specific Tests

```bash
# Run specific test function
go test -run TestReviewer_Review ./internal/reviewer/...

# Run tests matching pattern
go test -run "TestReview*" ./...

# Skip long-running tests
go test -short ./...
```

## Writing Unit Tests

### Basic Test Structure

```go
package reviewer

import (
    "context"
    "testing"
)

func TestReview_Success(t *testing.T) {
    // Arrange: Set up test dependencies
    mock := &mockProvider{
        response: `{"issues":[],"iso_scores":{...},"summary":"Good"}`,
    }
    tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
    orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
    reviewer := NewReviewer(orchestrator)

    diff := &types.Diff{
        Files: []types.DiffFile{{Path: "test.go"}},
    }

    // Act: Execute the code under test
    result, err := reviewer.Review(context.Background(), diff)

    // Assert: Verify expectations
    if err != nil {
        t.Fatalf("Review failed: %v", err)
    }

    if result.Cost.Tokens != 100 {
        t.Errorf("expected 100 tokens, got %d", result.Cost.Tokens)
    }
}
```

### Table-Driven Tests

Best practice for testing multiple scenarios:

```go
func TestDetectLanguage(t *testing.T) {
    tests := []struct {
        name     string
        path     string
        expected string
    }{
        {"go file", "main.go", "go"},
        {"python file", "script.py", "python"},
        {"javascript", "app.js", "javascript"},
        {"typescript", "component.tsx", "typescript"},
        {"unknown", "file.xyz", "unknown"},
    }

    detector := NewLanguageDetector()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := detector.DetectLanguage(tt.path)
            if result != tt.expected {
                t.Errorf("expected %s, got %s", tt.expected, result)
            }
        })
    }
}
```

Output:
```
=== RUN   TestDetectLanguage
=== RUN   TestDetectLanguage/go_file
=== RUN   TestDetectLanguage/python_file
=== RUN   TestDetectLanguage/javascript
=== RUN   TestDetectLanguage/typescript
=== RUN   TestDetectLanguage/unknown
--- PASS: TestDetectLanguage (0.00s)
    --- PASS: TestDetectLanguage/go_file (0.00s)
    --- PASS: TestDetectLanguage/python_file (0.00s)
    --- PASS: TestDetectLanguage/javascript (0.00s)
    --- PASS: TestDetectLanguage/typescript (0.00s)
    --- PASS: TestDetectLanguage/unknown (0.00s)
```

### Mock Objects

Create simple mocks for testing:

```go
// Mock LLM provider
type mockProvider struct {
    response string
    err      error
    callCount int
}

func (m *mockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
    m.callCount++
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

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Tokens(input string) (int, error) {
    return len(input) / 4, nil
}

// Usage in test
func TestWithMock(t *testing.T) {
    mock := &mockProvider{
        response: `{"result": "success"}`,
    }

    // Use mock in test
    orchestrator := llm.NewOrchestrator(mock, nil, tracker)

    // Verify behavior
    if mock.callCount != expectedCalls {
        t.Errorf("expected %d calls, got %d", expectedCalls, mock.callCount)
    }
}
```

### Test Fixtures

Store test data in fixtures:

```go
// tests/fixtures/diffs/simple.diff
diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main

+func Add(a, b int) int { return a + b }
```

Load in tests:

```go
func TestParseDiff(t *testing.T) {
    // Load fixture
    data, err := os.ReadFile("../../tests/fixtures/diffs/simple.diff")
    if err != nil {
        t.Fatalf("failed to load fixture: %v", err)
    }

    // Parse
    diff, err := ParseDiff(string(data))
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    // Verify
    if len(diff.Files) != 1 {
        t.Errorf("expected 1 file, got %d", len(diff.Files))
    }
}
```

## Integration Tests

Test multiple components together:

```go
func TestIntegration_ReviewPipeline(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Setup real components
    cfg := config.LoadDefaults()
    cfg.LLM.Provider = "openai"
    cfg.LLM.Model = "gpt-3.5-turbo"

    // Create provider (requires API key)
    provider := openai.NewProvider(
        os.Getenv("OPENAI_API_KEY"),
        cfg.LLM.Model,
    )

    // Build full pipeline
    tracker := cost.NewTracker(10.0, 100.0, priceMap)
    orchestrator := llm.NewOrchestrator(provider, nil, tracker)
    reviewer := NewReviewer(orchestrator)

    // Load real diff
    diff := loadTestDiff(t)

    // Execute end-to-end
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    result, err := reviewer.Review(ctx, diff)

    // Verify
    if err != nil {
        t.Fatalf("review failed: %v", err)
    }

    if len(result.Issues) == 0 {
        t.Log("No issues found (this may be expected)")
    }

    if result.Cost.Tokens == 0 {
        t.Error("expected non-zero token count")
    }
}
```

### HTTP Integration Tests

Test HTTP endpoints with httptest:

```go
func TestWebhookIntegration_PullRequestFlow(t *testing.T) {
    // Create test server
    handler := createTestHandler(t)
    server := httptest.NewServer(handler)
    defer server.Close()

    // Load fixture payload
    payload, err := os.ReadFile("../../tests/fixtures/webhooks/pr_opened.json")
    if err != nil {
        t.Fatalf("failed to load fixture: %v", err)
    }

    // Compute signature
    secret := "test-secret"
    signature := computeHMAC(payload, secret)

    // Create request
    req, err := http.NewRequest("POST", server.URL+"/webhook/github", bytes.NewReader(payload))
    if err != nil {
        t.Fatalf("failed to create request: %v", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-GitHub-Event", "pull_request")
    req.Header.Set("X-GitHub-Delivery", "test-delivery-1")
    req.Header.Set("X-Hub-Signature-256", "sha256="+signature)

    // Send request
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("request failed: %v", err)
    }
    defer resp.Body.Close()

    // Verify response
    if resp.StatusCode != http.StatusOK {
        t.Errorf("expected status 200, got %d", resp.StatusCode)
    }
}
```

## Testing with Docker

### Docker Compose Test Runner

```yaml
# docker-compose.yml
services:
  aurumcode:
    build: .
    volumes:
      - .:/app
    environment:
      - GO_ENV=test
```

Run tests:

```bash
# All tests
docker-compose run --rm aurumcode go test ./...

# With coverage
docker-compose run --rm aurumcode go test -cover ./internal/...

# Specific package
docker-compose run --rm aurumcode go test -v ./internal/reviewer/...
```

### Test in Clean Environment

```bash
# Build fresh image
docker-compose build aurumcode

# Run tests in isolated container
docker-compose run --rm aurumcode sh -c "
  go test ./... &&
  go test -cover ./internal/... &&
  go vet ./...
"
```

## Coverage Targets

### Current Coverage

```bash
$ docker-compose run --rm aurumcode go test -cover ./internal/...
ok      aurumcode/internal/analyzer      0.009s  coverage: 83.2% of statements
ok      aurumcode/internal/config        0.020s  coverage: 79.4% of statements
ok      aurumcode/internal/docgen        0.007s  coverage: 100.0% of statements
ok      aurumcode/internal/git/githubclient  18.851s coverage: 80.9% of statements
ok      aurumcode/internal/git/webhook   0.268s  coverage: 96.7% of statements
ok      aurumcode/internal/llm           0.055s  coverage: 78.2% of statements
ok      aurumcode/internal/llm/cost      0.008s  coverage: 85.3% of statements
ok      aurumcode/internal/llm/httpbase  8.015s  coverage: 73.0% of statements
ok      aurumcode/internal/llm/provider/openai  0.010s  coverage: 83.3% of statements
ok      aurumcode/internal/prompt        0.009s  coverage: 83.0% of statements
ok      aurumcode/internal/reviewer      0.007s  coverage: 83.3% of statements
ok      aurumcode/internal/testgen       0.007s  coverage: 100.0% of statements
```

### Coverage Goals

- **Critical paths**: ≥90% (reviewers, parsers, security)
- **Business logic**: ≥80% (orchestration, analysis)
- **Infrastructure**: ≥70% (HTTP clients, adapters)

### Measuring Coverage

```bash
# Generate detailed coverage report
go test -coverprofile=coverage.out ./...

# View by function
go tool cover -func=coverage.out

# View by package
go tool cover -func=coverage.out | grep -E "^aurumcode"

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html

# Open in browser
open coverage.html  # macOS
xdg-open coverage.html  # Linux
start coverage.html  # Windows
```

## Test Organization

### File Naming

```
package_name/
├── file.go           # Implementation
├── file_test.go      # Unit tests
└── integration_test.go  # Integration tests
```

### Test Categories

Use build tags to categorize tests:

```go
//go:build integration
// +build integration

package reviewer_test

func TestIntegration_FullPipeline(t *testing.T) {
    // Integration test
}
```

Run by category:

```bash
# Unit tests only (default)
go test ./...

# Integration tests
go test -tags=integration ./...

# All tests
go test -tags=integration ./...
```

## Common Testing Patterns

### Testing Errors

```go
func TestReview_LLMFailure(t *testing.T) {
    mock := &mockProvider{
        err: errors.New("LLM timeout"),
    }

    orchestrator := llm.NewOrchestrator(mock, nil, tracker)
    reviewer := NewReviewer(orchestrator)

    _, err := reviewer.Review(context.Background(), testDiff)

    // Verify error occurred
    if err == nil {
        t.Fatal("expected error for LLM failure")
    }

    // Verify error message
    if !strings.Contains(err.Error(), "LLM request failed") {
        t.Errorf("expected 'LLM request failed' error, got: %v", err)
    }
}
```

### Testing Context Cancellation

```go
func TestReview_ContextTimeout(t *testing.T) {
    slowProvider := &mockProvider{
        delay: 5 * time.Second,  // Simulate slow response
    }

    orchestrator := llm.NewOrchestrator(slowProvider, nil, tracker)
    reviewer := NewReviewer(orchestrator)

    // Create context with short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    // Should timeout
    _, err := reviewer.Review(ctx, testDiff)

    if err == nil {
        t.Fatal("expected error for context timeout")
    }

    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected DeadlineExceeded, got: %v", err)
    }
}
```

### Testing Concurrency

```go
func TestIdempotencyCache_Concurrent(t *testing.T) {
    cache := NewIdempotencyCache(100, 5*time.Minute)

    const goroutines = 100
    const operations = 1000

    var wg sync.WaitGroup
    wg.Add(goroutines)

    for i := 0; i < goroutines; i++ {
        go func(id int) {
            defer wg.Done()
            for j := 0; j < operations; j++ {
                deliveryID := fmt.Sprintf("delivery-%d-%d", id, j)
                cache.SeenOrAdd(deliveryID)
                cache.Contains(deliveryID)
            }
        }(i)
    }

    wg.Wait()

    // Verify no race conditions (run with -race flag)
    if t.Failed() {
        t.Error("race condition detected")
    }
}
```

Run with race detector:

```bash
go test -race ./internal/git/webhook/...
```

### Testing Time-Dependent Code

Use a test clock:

```go
type testClock struct {
    now time.Time
}

func (c *testClock) Now() time.Time {
    return c.now
}

func TestCostTracker_DailyReset(t *testing.T) {
    clock := &testClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
    tracker := NewTrackerWithClock(100.0, 1000.0, priceMap, clock)

    // Use budget
    tracker.Spend(50, 50, "test-model")

    // Advance to next day
    clock.now = clock.now.Add(24 * time.Hour)

    // Budget should reset
    perRun, daily := tracker.Remaining()
    if daily >= 1000.0 {
        t.Error("expected daily budget to reset")
    }
}
```

## Debugging Tests

### Verbose Output

```bash
# Show all test output
go test -v ./...

# Show only failed tests
go test ./... | grep FAIL
```

### Run Single Test

```bash
# Run specific test function
go test -run TestReview_Success ./internal/reviewer/...

# With verbose output
go test -v -run TestReview_Success ./internal/reviewer/...
```

### Print Debug Info

```go
func TestDebug(t *testing.T) {
    result := someFunction()

    // Log for debugging (only shown if test fails or -v flag)
    t.Logf("Result: %+v", result)

    // Or use fmt.Printf (always shown)
    fmt.Printf("Debug: %+v\n", result)
}
```

### Test with Debugger

```bash
# Delve debugger
dlv test ./internal/reviewer/...

# Set breakpoint and run
(dlv) break TestReview_Success
(dlv) continue
```

## CI/CD Integration

### GitHub Actions Example

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run tests
        run: |
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
```

### Docker CI

```bash
# In CI pipeline
docker-compose run --rm aurumcode sh -c "
  go test -race ./... &&
  go test -cover ./internal/... &&
  go vet ./... &&
  gofmt -s -l . | grep -q . && exit 1 || exit 0
"
```

## Best Practices

### ✅ Do

- Write tests before fixing bugs
- Use table-driven tests for multiple scenarios
- Mock external dependencies
- Test error paths
- Use meaningful test names
- Keep tests fast
- Run tests with `-race` flag
- Maintain ≥80% coverage on critical code

### ❌ Don't

- Test implementation details
- Write tests that depend on external services
- Share state between tests
- Use `time.Sleep` for synchronization
- Skip error checking in tests
- Commit failing tests
- Test private functions directly

## Troubleshooting

### Tests Fail Locally But Pass in CI

```bash
# Ensure clean environment
go clean -testcache
go test ./...

# Check for race conditions
go test -race ./...

# Verify Go version matches CI
go version
```

### Tests Are Slow

```bash
# Identify slow tests
go test -v ./... | grep -E "PASS|FAIL" | grep -E "[0-9]+\.[0-9]+s"

# Run in parallel (default)
go test -parallel 4 ./...

# Skip slow tests
go test -short ./...
```

### Coverage Not Updated

```bash
# Clear test cache
go clean -testcache

# Regenerate coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Next Steps

- [Development Setup](./DEVELOPMENT.md)
- [Architecture Overview](./ARCHITECTURE.md)
- [Adding Features](./EXTENDING.md)
- [Contributing Guide](./CONTRIBUTING.md)
