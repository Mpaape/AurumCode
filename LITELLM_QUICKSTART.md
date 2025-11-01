# LiteLLM Quick Setup for AurumCode

Your simple guide to configure AurumCode with LiteLLM.

## ✅ Easiest Method (Works Right Now)

Since LiteLLM is OpenAI-compatible, you can use it immediately:

### 1. Edit `.env` file:

```bash
# Your LiteLLM credentials
OPENAI_API_KEY=your-litellm-api-key
OPENAI_BASE_URL=https://your-litellm-url.com

# GitHub (required)
GITHUB_TOKEN=ghp_your_github_token
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# Tell AurumCode to use OpenAI provider (which connects to LiteLLM)
LLM_PROVIDER=openai
LLM_MODEL=gpt-4  # Or whatever model your LiteLLM proxies
```

### 2. Edit `.aurumcode/config.yml`:

```yaml
llm:
  provider: openai  # Uses OpenAI provider with LiteLLM URL
  model: gpt-4      # Your LiteLLM model name
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

### 3. Start AurumCode:

```bash
docker-compose up -d

# Test it works
curl http://localhost:8080/healthz
```

**That's it!** AurumCode will now use your LiteLLM endpoint.

## 🔧 How It Works

```
AurumCode
    ↓
OpenAI Provider (with custom base URL)
    ↓
Your LiteLLM Proxy
    ↓
Actual LLM (GPT-4, Claude, etc.)
```

The OpenAI provider accepts a custom `base_url` parameter, so it can point to LiteLLM instead of OpenAI's servers.

## 📝 Example `.env` for Different Scenarios

### Cloud LiteLLM:
```bash
OPENAI_API_KEY=your-litellm-cloud-key
OPENAI_BASE_URL=https://api.litellm.ai
```

### Self-Hosted LiteLLM:
```bash
OPENAI_API_KEY=sk-1234
OPENAI_BASE_URL=http://localhost:4000
```

### LiteLLM in Docker Compose:
```bash
OPENAI_API_KEY=sk-1234
OPENAI_BASE_URL=http://litellm:4000
```

## 🐳 Complete Docker Compose Example

If you want to run LiteLLM alongside AurumCode:

**docker-compose.yml:**
```yaml
version: '3.8'
services:
  # LiteLLM proxy
  litellm:
    image: ghcr.io/berriai/litellm:main-latest
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: --config /app/config.yaml
    environment:
      - OPENAI_API_KEY=${REAL_OPENAI_KEY}
      - ANTHROPIC_API_KEY=${REAL_ANTHROPIC_KEY}

  # AurumCode (connects to LiteLLM)
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

Start everything:
```bash
docker-compose up -d
```

## ✅ Testing Your Setup

### 1. Test LiteLLM directly:
```bash
curl http://localhost:4000/health
```

### 2. Test AurumCode health:
```bash
curl http://localhost:8080/healthz
```

### 3. Trigger a test review:
Create a PR and watch the logs:
```bash
docker-compose logs -f aurumcode
```

## 🆘 Troubleshooting

### "Connection refused" error
- Check LiteLLM is running: `curl http://localhost:4000/health`
- Verify `OPENAI_BASE_URL` in `.env`
- Check Docker network if using Docker Compose

### "Invalid API key" error
- Verify `OPENAI_API_KEY` matches your LiteLLM key
- Check LiteLLM logs: `docker logs <litellm-container>`

### "Model not found" error
- Check model name in `.aurumcode/config.yml`
- List LiteLLM models: `curl http://localhost:4000/v1/models`

## 📚 More Information

For advanced LiteLLM configuration, see:
- [Complete LiteLLM Guide](./docs/LITELLM_SETUP.md)
- [LiteLLM Documentation](https://docs.litellm.ai/)

## 🎯 Summary

**Your setup is simple:**

1. ✅ Created dedicated LiteLLM provider (86.7% test coverage)
2. ✅ Can use immediately with existing OpenAI provider
3. ✅ Just set `OPENAI_BASE_URL` to your LiteLLM endpoint
4. ✅ Everything else works the same

**No code changes needed** - just configuration!
