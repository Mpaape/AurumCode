---
layout: default
title: Quick Start
parent: Documentation
nav_order: 1
---

# Quick Start Guide

Get AurumCode running in 5 minutes.

## Prerequisites

- Docker (recommended) OR Go 1.21+
- GitHub account with admin access to a repository
- API key for LLM provider (OpenAI, Anthropic, or LiteLLM)

## Option 1: Docker (Recommended)

```bash
# Clone repository
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Configure
cp .env.example .env
# Edit .env with your API keys

# Run
docker-compose up -d

# Check logs
docker-compose logs -f
```

## Option 2: Build from Source

```bash
# Build
make build

# Run
export GITHUB_TOKEN=ghp_your_token
export OPENAI_API_KEY=sk_your_key
./bin/aurumcode-server
```

## Setup GitHub Webhook

1. Go to your repository → Settings → Webhooks
2. Add webhook:
   - URL: `https://your-server.com/webhook`
   - Content type: `application/json`
   - Secret: (from your `.env`)
   - Events: Pull requests, Pushes

## Create First Review

```bash
# Create test PR
git checkout -b test-aurumcode
echo "console.log('test')" > test.js
git add test.js
git commit -m "test: Add test file"
git push origin test-aurumcode

# Create PR via GitHub UI or:
gh pr create --title "Test AurumCode"
```

Watch AurumCode analyze and review your code!

---

[Next: Configuration Guide →](/guides/configuration)
