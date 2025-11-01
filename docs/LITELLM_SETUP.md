# LiteLLM Configuration Guide

Complete guide for using AurumCode with LiteLLM proxy.

## What is LiteLLM?

[LiteLLM](https://github.com/BerriAI/litellm) is a unified proxy that provides a single OpenAI-compatible API for 100+ LLM providers (OpenAI, Anthropic, Azure, Cohere, etc.).

**Benefits:**
- Single API for multiple providers
- Load balancing and fallbacks
- Cost tracking and rate limiting
- Self-hosted or cloud deployment

## Quick Setup (Recommended)

Since LiteLLM is OpenAI-compatible, you can use it immediately with the existing OpenAI provider:

### 1. Configure Environment

Create or edit `.env`:

```bash
# Use LiteLLM via OpenAI provider
OPENAI_API_KEY=your-litellm-api-key
OPENAI_BASE_URL=https://your-litellm-url.com

# Or if self-hosted
OPENAI_API_KEY=sk-1234  # Your LiteLLM key
OPENAI_BASE_URL=http://localhost:4000

# GitHub (required)
GITHUB_TOKEN=ghp_your_github_token
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# Provider selection
LLM_PROVIDER=openai
LLM_MODEL=gpt-4  # Model LiteLLM will proxy to
```

### 2. Configure AurumCode

Edit `.aurumcode/config.yml`:

```yaml
llm:
  provider: openai  # Uses OpenAI provider with LiteLLM URL
  model: gpt-4      # Model name that LiteLLM understands
  base_url: ${OPENAI_BASE_URL}  # Your LiteLLM endpoint
  api_key: ${OPENAI_API_KEY}    # Your LiteLLM API key
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

### 3. Start AurumCode

```bash
docker-compose up -d

# Verify connection
curl http://localhost:8080/healthz
```

## Advanced Setup (Dedicated Provider)

For better clarity, use the dedicated LiteLLM provider:

### 1. Configure Environment

```bash
# .env
LITELLM_API_KEY=your-litellm-api-key
LITELLM_BASE_URL=https://your-litellm-url.com

GITHUB_TOKEN=ghp_your_github_token
GITHUB_WEBHOOK_SECRET=your-webhook-secret

LLM_PROVIDER=litellm
LLM_MODEL=gpt-4
```

### 2. Update Server Code

Modify `cmd/server/main.go` to support LiteLLM provider:

```go
package main

import (
    "aurumcode/internal/config"
    "aurumcode/internal/llm"
    "aurumcode/internal/llm/cost"
    "aurumcode/internal/llm/provider/litellm"
    "aurumcode/internal/llm/provider/openai"
    "log"
    "os"
)

func main() {
    cfg, err := config.Load(".aurumcode/config.yml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Create provider based on config
    var provider llm.Provider

    switch cfg.LLM.Provider {
    case "litellm":
        apiKey := os.Getenv("LITELLM_API_KEY")
        baseURL := os.Getenv("LITELLM_BASE_URL")
        provider = litellm.NewProvider(apiKey, baseURL, cfg.LLM.Model)

    case "openai":
        apiKey := os.Getenv("OPENAI_API_KEY")
        baseURL := os.Getenv("OPENAI_BASE_URL")
        if baseURL == "" {
            baseURL = "https://api.openai.com"
        }
        provider = openai.NewProvider(apiKey, baseURL, cfg.LLM.Model)

    default:
        log.Fatalf("Unsupported provider: %s", cfg.LLM.Provider)
    }

    // Create cost tracker
    priceMap := map[string]cost.PriceMap{
        cfg.LLM.Model: {
            InputPer1K:  0.03,  // Adjust based on your model
            OutputPer1K: 0.06,
        },
    }
    tracker := cost.NewTracker(
        cfg.LLM.Budgets.PerRunUSD,
        cfg.LLM.Budgets.DailyUSD,
        priceMap,
    )

    // Create orchestrator
    orchestrator := llm.NewOrchestrator(provider, nil, tracker)

    // ... rest of server setup
}
```

## LiteLLM Deployment Options

### Option 1: Cloud Hosted

Use LiteLLM's hosted service:

```bash
# Sign up at https://litellm.ai
# Get your API key and URL

# Configure
LITELLM_API_KEY=sk-your-key
LITELLM_BASE_URL=https://api.litellm.ai
```

### Option 2: Self-Hosted with Docker

Run LiteLLM locally:

```bash
# Create litellm_config.yaml
cat > litellm_config.yaml <<EOF
model_list:
  - model_name: gpt-4
    litellm_params:
      model: openai/gpt-4
      api_key: ${OPENAI_API_KEY}

  - model_name: claude-3
    litellm_params:
      model: anthropic/claude-3-sonnet-20240229
      api_key: ${ANTHROPIC_API_KEY}

  - model_name: local-llama
    litellm_params:
      model: ollama/llama2
      api_base: http://localhost:11434
EOF

# Run LiteLLM
docker run -p 4000:4000 \
  -v $(pwd)/litellm_config.yaml:/app/config.yaml \
  ghcr.io/berriai/litellm:main-latest \
  --config /app/config.yaml \
  --port 4000

# Configure AurumCode to use local LiteLLM
LITELLM_API_KEY=sk-1234
LITELLM_BASE_URL=http://localhost:4000
```

### Option 3: Docker Compose Integration

Add LiteLLM to `docker-compose.yml`:

```yaml
version: '3.8'

services:
  aurumcode:
    build: .
    ports:
      - "8080:8080"
    environment:
      - LITELLM_API_KEY=sk-1234
      - LITELLM_BASE_URL=http://litellm:4000
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - GITHUB_WEBHOOK_SECRET=${GITHUB_WEBHOOK_SECRET}
    depends_on:
      - litellm

  litellm:
    image: ghcr.io/berriai/litellm:main-latest
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: --config /app/config.yaml --port 4000
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
```

Start everything:

```bash
docker-compose up -d
```

## Testing LiteLLM Connection

### 1. Test LiteLLM Directly

```bash
# Test LiteLLM endpoint
curl http://localhost:4000/v1/models \
  -H "Authorization: Bearer sk-1234"

# Test completion
curl http://localhost:4000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-1234" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### 2. Test AurumCode Integration

```bash
# Check health
curl http://localhost:8080/healthz

# Simulate webhook
./scripts/test-webhook.sh
```

## Model Configuration

LiteLLM supports unified model naming. Configure in `.aurumcode/config.yml`:

```yaml
llm:
  provider: openai  # or litellm
  model: gpt-4      # LiteLLM resolves this
  # OR use provider prefix:
  # model: openai/gpt-4
  # model: anthropic/claude-3-sonnet-20240229
  # model: ollama/llama2
```

### Available Models (via LiteLLM)

```yaml
# OpenAI
model: gpt-4
model: gpt-4-turbo-preview
model: gpt-3.5-turbo

# Anthropic
model: claude-3-opus-20240229
model: claude-3-sonnet-20240229
model: claude-3-haiku-20240307

# Azure OpenAI
model: azure/gpt-4

# Ollama (local)
model: ollama/llama2
model: ollama/codellama
model: ollama/mistral
```

## Load Balancing & Fallbacks

LiteLLM handles fallbacks automatically. Configure in `litellm_config.yaml`:

```yaml
model_list:
  - model_name: gpt-4
    litellm_params:
      model: openai/gpt-4
      api_key: ${OPENAI_API_KEY}

  - model_name: gpt-4  # Same name = fallback
    litellm_params:
      model: anthropic/claude-3-sonnet-20240229
      api_key: ${ANTHROPIC_API_KEY}

  - model_name: gpt-4  # Third fallback
    litellm_params:
      model: ollama/llama2
      api_base: http://localhost:11434

router_settings:
  routing_strategy: simple-shuffle  # or least-busy, latency-based
  allowed_fails: 3
```

## Cost Tracking

LiteLLM provides built-in cost tracking:

```yaml
# litellm_config.yaml
general_settings:
  master_key: sk-1234
  database_url: postgresql://...  # For persistent tracking

litellm_settings:
  success_callback: ["langfuse"]  # Track to Langfuse
  max_budget: 100  # USD per month
```

AurumCode also tracks costs independently in `.aurumcode/config.yml`:

```yaml
llm:
  budgets:
    per_run_usd: 1.0   # Max cost per review
    daily_usd: 10.0    # Max cost per day
```

## Troubleshooting

### Connection Issues

```bash
# Check LiteLLM is running
curl http://localhost:4000/health

# Check logs
docker logs litellm_container

# Test with verbose output
curl -v http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-1234" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}'
```

### API Key Issues

```bash
# Verify environment variables
echo $LITELLM_API_KEY
echo $LITELLM_BASE_URL

# Check LiteLLM config
cat litellm_config.yaml

# Verify underlying provider keys
echo $OPENAI_API_KEY
echo $ANTHROPIC_API_KEY
```

### Model Not Found

```bash
# List available models
curl http://localhost:4000/v1/models \
  -H "Authorization: Bearer sk-1234"

# Check model name in config
grep "model:" .aurumcode/config.yml
```

## Production Recommendations

### 1. Use Environment-Specific Configs

```bash
# Production
LITELLM_BASE_URL=https://litellm-prod.yourcompany.com
LITELLM_API_KEY=sk-prod-xxx

# Staging
LITELLM_BASE_URL=https://litellm-staging.yourcompany.com
LITELLM_API_KEY=sk-staging-xxx

# Development
LITELLM_BASE_URL=http://localhost:4000
LITELLM_API_KEY=sk-dev-xxx
```

### 2. Monitor Costs

LiteLLM provides cost tracking. Enable in config:

```yaml
# litellm_config.yaml
litellm_settings:
  success_callback: ["langfuse"]
  max_budget: 1000  # USD/month
  budget_duration: 30d
```

### 3. Set Rate Limits

```yaml
# litellm_config.yaml
model_list:
  - model_name: gpt-4
    litellm_params:
      model: openai/gpt-4
      api_key: ${OPENAI_API_KEY}
      rpm: 10  # Requests per minute
      tpm: 40000  # Tokens per minute
```

### 4. Enable Caching

```yaml
# litellm_config.yaml
litellm_settings:
  cache: true
  cache_params:
    type: redis
    host: redis
    port: 6379
```

## Example: Complete Setup

Here's a complete working example:

**Directory structure:**
```
aurumcode/
├── .env
├── .aurumcode/
│   └── config.yml
├── docker-compose.yml
└── litellm_config.yaml
```

**.env:**
```bash
OPENAI_API_KEY=sk-xxx
ANTHROPIC_API_KEY=sk-ant-xxx
LITELLM_API_KEY=sk-1234
GITHUB_TOKEN=ghp_xxx
GITHUB_WEBHOOK_SECRET=secret
```

**litellm_config.yaml:**
```yaml
model_list:
  - model_name: gpt-4
    litellm_params:
      model: openai/gpt-4
      api_key: ${OPENAI_API_KEY}

  - model_name: claude
    litellm_params:
      model: anthropic/claude-3-sonnet-20240229
      api_key: ${ANTHROPIC_API_KEY}
```

**docker-compose.yml:**
```yaml
version: '3.8'
services:
  litellm:
    image: ghcr.io/berriai/litellm:main-latest
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: --config /app/config.yaml
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}

  aurumcode:
    build: .
    ports:
      - "8080:8080"
    environment:
      - OPENAI_API_KEY=sk-1234
      - OPENAI_BASE_URL=http://litellm:4000
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - GITHUB_WEBHOOK_SECRET=${GITHUB_WEBHOOK_SECRET}
    depends_on:
      - litellm
```

**.aurumcode/config.yml:**
```yaml
llm:
  provider: openai  # Using OpenAI provider with LiteLLM URL
  model: gpt-4
  temperature: 0.3
  max_tokens: 4000
  budgets:
    per_run_usd: 1.0
    daily_usd: 10.0
```

**Start everything:**
```bash
docker-compose up -d
curl http://localhost:8080/healthz
```

## Summary

**Quick Start (Easiest):**
```bash
# Use existing OpenAI provider with LiteLLM URL
OPENAI_API_KEY=your-litellm-key
OPENAI_BASE_URL=https://your-litellm-url.com
LLM_PROVIDER=openai
```

**Production (Recommended):**
```bash
# Self-host LiteLLM with Docker Compose
# Configure fallbacks and load balancing
# Monitor costs with LiteLLM's built-in tracking
```

For questions or issues, see:
- [LiteLLM Documentation](https://docs.litellm.ai/)
- [AurumCode Issues](https://github.com/yourusername/aurumcode/issues)
