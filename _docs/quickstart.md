---
layout: default
title: QUICKSTART
parent: Documentation
nav_order: 1
---

# Quickstart Guide

Get AurumCode running in 5 minutes.

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- GitHub account with a repository
- API key for at least one LLM provider (OpenAI, Anthropic, etc.)

## 1. Clone and Build

```bash
# Clone the repository
git clone https://github.com/yourusername/aurumcode.git
cd aurumcode

# Build the project
make build

# Or use Docker
docker-compose build
```

## 2. Configure Environment

Create a `.env` file in the project root:

```bash
# Required: At least one LLM provider
OPENAI_API_KEY=sk-...
# OR
ANTHROPIC_API_KEY=sk-ant-...
# OR
OLLAMA_BASE_URL=http://localhost:11434

# Required: GitHub integration
GITHUB_TOKEN=ghp_...
GITHUB_WEBHOOK_SECRET=your-secret-here

# Optional: Override defaults
LLM_PROVIDER=openai
LLM_MODEL=gpt-4
```

## 3. Create Configuration

Create `.aurumcode/config.yml` in your repository:

```yaml
llm:
  provider: openai  # or anthropic, ollama, litellm
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

prompts:
  review_system: "You are an expert code reviewer..."
```

## 4. Run the Server

### Using Docker (Recommended)

```bash
# Start the server
docker-compose up -d

# Check logs
docker-compose logs -f aurumcode

# Server runs on http://localhost:8080
```

### Using Go Directly

```bash
# Run the server
go run cmd/server/main.go

# Or use the built binary
./bin/server
```

## 5. Test the Setup

### Health Check

```bash
curl http://localhost:8080/healthz
```

Expected response:
```json
{
  "status": "ok",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### Simulate a Webhook

```bash
# Create a test payload
cat > test-webhook.json <<EOF
{
  "action": "opened",
  "number": 1,
  "pull_request": {
    "title": "Test PR",
    "head": {
      "sha": "abc123",
      "ref": "feature-branch"
    },
    "base": {
      "ref": "main"
    }
  },
  "repository": {
    "full_name": "owner/repo"
  }
}
EOF

# Generate signature
SIGNATURE=$(echo -n "$(cat test-webhook.json)" | openssl dgst -sha256 -hmac "your-secret-here" | sed 's/.* //')

# Send the webhook
curl -X POST http://localhost:8080/webhook/github \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: pull_request" \
  -H "X-GitHub-Delivery: test-delivery-1" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  -d @test-webhook.json
```

## 6. Set Up GitHub Webhook

1. Go to your GitHub repository
2. Navigate to **Settings** → **Webhooks** → **Add webhook**
3. Configure:
   - **Payload URL**: `https://your-server.com/webhook/github`
   - **Content type**: `application/json`
   - **Secret**: Same as `GITHUB_WEBHOOK_SECRET`
   - **Events**: Select "Pull requests" and "Pushes"
4. Click **Add webhook**

## 7. Test with a Real PR

```bash
# Create a test branch
git checkout -b test-review

# Make some changes
echo "package main" > test.go
echo "func Add(a, b int) int { return a + b }" >> test.go

# Commit and push
git add test.go
git commit -m "Add test function"
git push origin test-review

# Create a PR on GitHub
gh pr create --title "Test Code Review" --body "Testing AurumCode"
```

AurumCode will automatically:
1. Receive the webhook
2. Fetch the PR diff
3. Analyze the code
4. Generate a review with the LLM
5. Post review comments
6. Set commit status

## 8. View Results

Check your PR on GitHub for:
- **Review comments** on changed lines
- **Commit status** (pending → success/failure)
- **ISO/IEC 25010 scores** in review summary

## Next Steps

- [Configure advanced settings](./CONFIGURATION.md)
- [Learn about the architecture](./ARCHITECTURE.md)
- [Set up local development](./DEVELOPMENT.md)
- [Run the test suite](./TESTING.md)

## Troubleshooting

### Server won't start

```bash
# Check logs
docker-compose logs aurumcode

# Common issues:
# - Port 8080 already in use
# - Missing environment variables
# - Invalid config.yml
```

### Webhook not received

```bash
# Check GitHub webhook delivery logs
# Settings → Webhooks → Recent Deliveries

# Verify signature
echo "Check GITHUB_WEBHOOK_SECRET matches in both places"

# Test connectivity
curl -I https://your-server.com/webhook/github
```

### LLM requests failing

```bash
# Verify API key
echo $OPENAI_API_KEY

# Check budget limits
# .aurumcode/config.yml → llm.budgets

# Test provider directly
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"
```

### Tests not running

```bash
# Run tests with Docker
docker-compose run --rm aurumcode go test ./...

# Check coverage
docker-compose run --rm aurumcode go test -cover ./internal/...

# Run specific test
docker-compose run --rm aurumcode go test -v ./internal/reviewer/...
```

## Quick Reference

| Command | Description |
|---------|-------------|
| `make build` | Build the project |
| `make test` | Run all tests |
| `make cover` | Generate coverage report |
| `make lint` | Run linters |
| `docker-compose up` | Start server |
| `docker-compose logs -f` | View logs |
| `docker-compose down` | Stop server |

## Getting Help

- [Full Documentation](./README.md)
- [GitHub Issues](https://github.com/yourusername/aurumcode/issues)
- [Discussions](https://github.com/yourusername/aurumcode/discussions)
