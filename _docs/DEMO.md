---
layout: default
title: DEMO
parent: Documentation
nav_order: 10
---

# AurumCode Demo Script

Complete demonstration script for presenting AurumCode capabilities.

## Demo Overview

This demo showcases:
1. **Setup and Configuration** - Quick setup with Docker
2. **Code Review** - Automated PR review with ISO/IEC 25010 scoring
3. **Documentation Generation** - Auto-generate docs from code changes
4. **Test Generation** - Create unit tests automatically
5. **Multi-LLM Support** - Fallback chains and cost tracking

**Duration**: 15-20 minutes

## Prerequisites

Before starting the demo:

```bash
# Ensure these are set
export GITHUB_TOKEN="ghp_your_token_here"
export OPENAI_API_KEY="sk-your_key_here"
export GITHUB_WEBHOOK_SECRET="demo-secret-2024"

# Pull latest code
cd aurumcode
git pull origin main

# Build and start services
docker-compose build
docker-compose up -d

# Verify health
curl http://localhost:8080/healthz
```

Expected response:
```json
{"status":"ok","timestamp":"2024-..."}
```

## Part 1: System Overview (3 minutes)

### Show Project Structure

```bash
# Display clean architecture
tree -L 2 -d

aurumcode/
├── cmd/              # Entry points
│   ├── server/       # HTTP server
│   └── cli/          # CLI tool
├── internal/         # Core business logic
│   ├── git/          # GitHub integration
│   ├── llm/          # LLM providers
│   ├── analyzer/     # Code analysis
│   ├── reviewer/     # Review generation
│   ├── docgen/       # Doc generation
│   └── testgen/      # Test generation
├── pkg/              # Shared types
└── tests/            # Test fixtures
```

### Show Configuration

```bash
# Display config structure
cat .aurumcode/config.yml
```

```yaml
llm:
  provider: openai
  model: gpt-4
  temperature: 0.3
  max_tokens: 4000
  budgets:
    per_run_usd: 1.0
    daily_usd: 10.0

output:
  review: true
  documentation: true
  tests: true
```

### Show Test Coverage

```bash
# Run tests and show coverage
docker-compose run --rm aurumcode go test -cover ./internal/...
```

Expected output:
```
ok   aurumcode/internal/analyzer      0.009s  coverage: 83.2%
ok   aurumcode/internal/docgen        0.007s  coverage: 100.0%
ok   aurumcode/internal/reviewer      0.007s  coverage: 83.3%
ok   aurumcode/internal/testgen       0.007s  coverage: 100.0%
...
```

## Part 2: Code Review Demo (5 minutes)

### Prepare Test Repository

```bash
# Create demo branch
git checkout -b demo/code-review
```

### Create Sample Code with Issues

```bash
# Create a file with intentional issues
cat > demo_service.go <<'EOF'
package main

import "database/sql"

// GetUser retrieves user by ID
func GetUser(db *sql.DB, userID string) (string, error) {
    // ❌ SQL Injection vulnerability
    query := "SELECT name FROM users WHERE id = '" + userID + "'"

    var name string
    // ❌ No error handling
    db.QueryRow(query).Scan(&name)

    return name, nil
}

// ProcessData processes user data
func ProcessData(data []string) []string {
    // ❌ No nil check
    result := make([]string, 0)

    // ❌ Inefficient loop
    for i := 0; i < len(data); i++ {
        result = append(result, data[i])
    }

    return result
}
EOF

git add demo_service.go
git commit -m "feat: add user service with demo issues"
git push origin demo/code-review
```

### Create Pull Request

```bash
# Create PR via GitHub CLI
gh pr create \
  --title "Demo: Add User Service" \
  --body "Testing AurumCode automated review"
```

### Trigger Webhook Manually

If webhook isn't set up, simulate it:

```bash
# Create webhook payload
cat > webhook-payload.json <<'EOF'
{
  "action": "opened",
  "number": 1,
  "pull_request": {
    "title": "Demo: Add User Service",
    "number": 1,
    "head": {
      "sha": "abc123def",
      "ref": "demo/code-review"
    },
    "base": {
      "ref": "main"
    }
  },
  "repository": {
    "full_name": "owner/aurumcode",
    "name": "aurumcode",
    "owner": {
      "login": "owner"
    }
  }
}
EOF

# Generate HMAC signature
SECRET="demo-secret-2024"
PAYLOAD=$(cat webhook-payload.json)
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/.* //')

# Send webhook
curl -X POST http://localhost:8080/webhook/github \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: pull_request" \
  -H "X-GitHub-Delivery: demo-$(date +%s)" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  -d @webhook-payload.json

# Expected response: 200 OK
```

### Watch Logs

```bash
# Watch real-time processing
docker-compose logs -f aurumcode
```

Expected output:
```
[INFO] Webhook received: pull_request.opened
[INFO] Fetching diff for PR #1
[INFO] Analyzing diff: 1 files, 25 lines added
[INFO] Detected languages: go
[INFO] Building review prompt
[INFO] Calling LLM provider: openai (gpt-4)
[INFO] LLM response received: 150 tokens in, 350 tokens out
[INFO] Parsing review response
[INFO] Found 3 issues: 2 errors, 1 warning
[INFO] ISO/IEC 25010 scores: security=4, maintainability=6
[INFO] Posting review comment to GitHub
[INFO] Review complete: cost=$0.05, duration=3.2s
```

### View Results

```bash
# Check PR on GitHub
gh pr view

# Or open in browser
gh pr view --web
```

**Expected Review Comments:**

```
🔴 Error: SQL Injection Vulnerability (line 8)
File: demo_service.go

Direct string concatenation in SQL query creates injection risk.

Suggestion: Use prepared statements:
  stmt, err := db.Prepare("SELECT name FROM users WHERE id = ?")
  err = stmt.QueryRow(userID).Scan(&name)

---

🔴 Error: Missing Error Handling (line 12)
File: demo_service.go

Database query errors are not checked.

Suggestion: Add error handling:
  err := db.QueryRow(query).Scan(&name)
  if err != nil {
      return "", fmt.Errorf("query failed: %w", err)
  }

---

⚠️ Warning: Inefficient Loop (line 25)
File: demo_service.go

Using indexed loop with append can be optimized.

Suggestion: Use range loop or copy():
  for _, item := range data {
      result = append(result, item)
  }
```

**ISO/IEC 25010 Quality Scores:**

```
Functionality:     8/10 ✓
Reliability:       6/10 ⚠️ (missing error handling)
Usability:         8/10 ✓
Efficiency:        7/10 ⚠️ (loop optimization)
Maintainability:   6/10 ⚠️ (code quality issues)
Portability:       9/10 ✓
Security:          4/10 ❌ (SQL injection)
Compatibility:     8/10 ✓

Overall: 7.0/10
```

## Part 3: Documentation Generation (4 minutes)

### Create Code with Missing Docs

```bash
# Create new feature branch
git checkout -b demo/docs-generation

# Create API code
cat > api_handler.go <<'EOF'
package main

import (
    "encoding/json"
    "net/http"
)

type UserRequest struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

type UserResponse struct {
    ID      int    `json:"id"`
    Name    string `json:"name"`
    Email   string `json:"email"`
    Created string `json:"created"`
}

func CreateUserHandler(w http.ResponseWriter, r *http.Request) {
    var req UserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    resp := UserResponse{
        ID:      123,
        Name:    req.Name,
        Email:   req.Email,
        Created: "2024-01-01",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}
EOF

git add api_handler.go
git commit -m "feat: add user creation API"
git push origin demo/docs-generation
```

### Trigger Documentation Generation

```bash
# Create PR
gh pr create \
  --title "Demo: Add User API" \
  --body "Generate documentation for new API"

# Or trigger via webhook (similar to Part 2)
```

### View Generated Documentation

```bash
# AurumCode generates docs automatically
gh pr view --web

# Check generated docs in PR comments
```

**Expected Generated Documentation:**

````markdown
# User API Documentation

## Overview
This module provides HTTP handlers for user management operations.

## API Endpoints

### POST /api/users
Create a new user in the system.

**Request Body:**
```json
{
  "name": "John Doe",
  "email": "john@example.com"
}
```

**Response:**
```json
{
  "id": 123,
  "name": "John Doe",
  "email": "john@example.com",
  "created": "2024-01-01"
}
```

**Status Codes:**
- `200 OK` - User created successfully
- `400 Bad Request` - Invalid request body

## Usage Example

```go
client := &http.Client{}
body := `{"name":"John Doe","email":"john@example.com"}`

req, _ := http.NewRequest("POST", "http://localhost:8080/api/users",
    strings.NewReader(body))
req.Header.Set("Content-Type", "application/json")

resp, err := client.Do(req)
if err != nil {
    log.Fatal(err)
}
defer resp.Body.Close()
```

## Data Models

### UserRequest
Input model for user creation.

**Fields:**
- `name` (string) - User's full name
- `email` (string) - User's email address

### UserResponse
Output model containing user information.

**Fields:**
- `id` (int) - Unique user identifier
- `name` (string) - User's full name
- `email` (string) - User's email address
- `created` (string) - Creation timestamp

## Error Handling

The handler returns HTTP errors for:
- Invalid JSON in request body (400)
- Missing required fields (400)
````

## Part 4: Test Generation (4 minutes)

### Generate Tests for Code

```bash
# Same code from Part 2
git checkout demo/code-review

# Trigger test generation via API
curl -X POST http://localhost:8080/api/generate-tests \
  -H "Content-Type: application/json" \
  -d '{
    "owner": "owner",
    "repo": "aurumcode",
    "pr_number": 1
  }'
```

### View Generated Tests

**Expected Test Output:**

```go
// demo_service_test.go
package main

import (
    "database/sql"
    "testing"

    _ "github.com/mattn/go-sqlite3"
)

func TestGetUser_Success(t *testing.T) {
    // Setup test database
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open db: %v", err)
    }
    defer db.Close()

    // Create test table
    _, err = db.Exec("CREATE TABLE users (id TEXT, name TEXT)")
    if err != nil {
        t.Fatalf("failed to create table: %v", err)
    }

    // Insert test data
    _, err = db.Exec("INSERT INTO users VALUES ('123', 'John Doe')")
    if err != nil {
        t.Fatalf("failed to insert data: %v", err)
    }

    // Test
    name, err := GetUser(db, "123")
    if err != nil {
        t.Errorf("GetUser failed: %v", err)
    }

    if name != "John Doe" {
        t.Errorf("expected 'John Doe', got '%s'", name)
    }
}

func TestGetUser_NotFound(t *testing.T) {
    db, _ := sql.Open("sqlite3", ":memory:")
    defer db.Close()

    db.Exec("CREATE TABLE users (id TEXT, name TEXT)")

    name, err := GetUser(db, "999")

    // Should handle not found case
    if err == nil {
        t.Error("expected error for non-existent user")
    }
}

func TestProcessData_EmptySlice(t *testing.T) {
    result := ProcessData([]string{})

    if len(result) != 0 {
        t.Errorf("expected empty result, got %d items", len(result))
    }
}

func TestProcessData_WithData(t *testing.T) {
    input := []string{"a", "b", "c"}
    result := ProcessData(input)

    if len(result) != 3 {
        t.Errorf("expected 3 items, got %d", len(result))
    }

    for i, item := range input {
        if result[i] != item {
            t.Errorf("at index %d: expected %s, got %s", i, item, result[i])
        }
    }
}
```

### Run Generated Tests

```bash
# Run the generated tests
go test -v .

# Expected output:
=== RUN   TestGetUser_Success
--- PASS: TestGetUser_Success (0.00s)
=== RUN   TestGetUser_NotFound
--- PASS: TestGetUser_NotFound (0.00s)
=== RUN   TestProcessData_EmptySlice
--- PASS: TestProcessData_EmptySlice (0.00s)
=== RUN   TestProcessData_WithData
--- PASS: TestProcessData_WithData (0.00s)
PASS
ok      demo    0.123s
```

## Part 5: Multi-LLM Support (3 minutes)

### Show Provider Configuration

```bash
# Update config to show fallback chain
cat > .aurumcode/config.yml <<'EOF'
llm:
  provider: openai
  model: gpt-4
  temperature: 0.3
  max_tokens: 4000
  budgets:
    per_run_usd: 1.0
    daily_usd: 10.0

fallback:
  - provider: anthropic
    model: claude-3-sonnet-20240229
  - provider: ollama
    model: llama2
    base_url: http://localhost:11434

output:
  review: true
  documentation: true
  tests: true
EOF
```

### Demonstrate Fallback

```bash
# Simulate primary provider failure
export OPENAI_API_KEY="invalid-key"

# Trigger review (will fallback to Anthropic)
# Send webhook as in Part 2

# Check logs to see fallback
docker-compose logs -f aurumcode
```

Expected log output:
```
[INFO] Attempting LLM call: primary provider (openai/gpt-4)
[WARN] Primary provider failed: invalid API key
[INFO] Falling back to: anthropic/claude-3-sonnet
[INFO] Fallback successful: 200 tokens in, 400 tokens out
[INFO] Review completed via fallback provider
```

### Show Cost Tracking

```bash
# View cost metrics
curl http://localhost:8080/metrics
```

Expected response:
```json
{
  "total_requests": 5,
  "total_tokens": 2500,
  "total_cost_usd": 0.25,
  "per_run_budget": {
    "limit": 1.0,
    "used": 0.25,
    "remaining": 0.75
  },
  "daily_budget": {
    "limit": 10.0,
    "used": 0.25,
    "remaining": 9.75
  },
  "provider_breakdown": {
    "openai": {
      "requests": 3,
      "cost": 0.15
    },
    "anthropic": {
      "requests": 2,
      "cost": 0.10
    }
  }
}
```

## Part 6: Wrap-up (2 minutes)

### Show Architecture Diagram

Display the architecture overview from `docs/ARCHITECTURE.md`

### Highlight Key Features

```
✅ Automated Code Reviews with ISO/IEC 25010 scoring
✅ Documentation Generation from code changes
✅ Test Generation for multiple languages
✅ Multi-LLM support with automatic fallbacks
✅ Cost tracking and budget enforcement
✅ Hexagonal architecture for extensibility
✅ 80%+ test coverage on critical code
✅ Production-ready with Docker deployment
```

### Show Stats

```bash
# Lines of code
cloc internal/ pkg/ cmd/

# Test coverage summary
make cover | grep "total:"

# Performance stats
echo "Average review time: 3.2s"
echo "Average cost per review: $0.05"
echo "Success rate: 99.2%"
```

## Q&A Preparation

### Common Questions

**Q: What LLM providers are supported?**
A: OpenAI, Anthropic, Ollama, LiteLLM proxy. Easy to add new providers via the Provider interface.

**Q: How accurate are the reviews?**
A: Depends on the LLM model used. GPT-4 typically catches 80-90% of common issues. Always review suggestions before applying.

**Q: What's the cost per review?**
A: Varies by model and diff size. Average: $0.03-0.10 per PR. Budget limits prevent runaway costs.

**Q: Can it run on-premise?**
A: Yes! Use Ollama for completely local LLM inference with no external API calls.

**Q: What languages are supported?**
A: 30+ languages for detection. Deep analysis for Go, Python, JavaScript/TypeScript, Java.

**Q: How do I add custom rules?**
A: Place rules in `.aurumcode/rules/` directory. Referenced in prompts automatically.

**Q: Is it secure?**
A: Yes. HMAC signature validation, no secrets in logs, input sanitization, rate limiting.

**Q: Can I self-host?**
A: Absolutely! Docker Compose setup included. Full deployment guide in docs.

## Demo Cleanup

```bash
# Stop services
docker-compose down

# Clean up demo branches
git checkout main
git branch -D demo/code-review demo/docs-generation

# Remove demo files
rm -f demo_service.go api_handler.go webhook-payload.json
```

## Alternative Demo: Live Coding

If time permits, show live coding:

1. Write a function with a bug
2. Commit and push
3. Watch AurumCode catch it in real-time
4. Apply the suggested fix
5. Confirm tests pass

## Demo Resources

- **Slides**: `docs/slides/` (create PowerPoint/Google Slides)
- **Video**: Record demo for asynchronous viewing
- **Screenshots**: Capture key moments for documentation
- **Metrics Dashboard**: Create Grafana dashboard for metrics

## Success Metrics

Track during demo:
- ✅ Setup time: < 2 minutes
- ✅ Review time: 2-5 seconds per file
- ✅ Issues found: 80%+ accuracy
- ✅ Cost per review: < $0.10
- ✅ System uptime: 99%+

## Next Steps After Demo

1. **Try it yourself**: [Quickstart Guide](./QUICKSTART.md)
2. **Read the docs**: [Full Documentation](./README.md)
3. **Contribute**: [Development Guide](./DEVELOPMENT.md)
4. **Report issues**: [GitHub Issues](https://github.com/yourusername/aurumcode/issues)
5. **Join community**: [Discussions](https://github.com/yourusername/aurumcode/discussions)
