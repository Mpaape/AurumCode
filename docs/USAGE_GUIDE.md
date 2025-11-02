# AurumCode Usage Guide

## How to Activate AurumCode on Your Project

AurumCode can be activated in your projects through **three different approaches**, depending on your needs:

---

## ⚡ Approach #1: GitHub Action (Recommended)

**Best for:** Public repos, projects already using GitHub Actions, serverless deployment

### Quick Start (2 minutes)

1. **Add workflow file** to your repo:

```bash
mkdir -p .github/workflows
curl -o .github/workflows/aurumcode.yml https://raw.githubusercontent.com/Mpaape/AurumCode/main/.github/workflows/examples/all-pipelines.yml
```

2. **Configure secrets** in your GitHub repo:
   - Go to: `Settings → Secrets and variables → Actions`
   - Add: `OPENAI_API_KEY` (or `ANTHROPIC_API_KEY`)

3. **Done!** Open a PR and watch AurumCode analyze it automatically.

### Example Workflows

#### Code Review Only
```yaml
name: AurumCode Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run AurumCode
        uses: Mpaape/AurumCode@main
        with:
          mode: 'review'
          llm_api_key: ${{ secrets.OPENAI_API_KEY }}
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

#### Documentation Only
```yaml
name: AurumCode Docs

on:
  push:
    branches: [main]

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate Documentation
        uses: Mpaape/AurumCode@main
        with:
          mode: 'documentation'
          llm_api_key: ${{ secrets.OPENAI_API_KEY }}
          documentation_mode: 'incremental'
```

#### Complete Pipeline
```yaml
name: AurumCode Complete

on:
  pull_request:
  push:
    branches: [main]

jobs:
  aurumcode:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run All Pipelines
        uses: Mpaape/AurumCode@main
        with:
          mode: 'all'
          llm_api_key: ${{ secrets.OPENAI_API_KEY }}
          coverage_threshold: '80'
```

---

## 🖥️ Approach #2: Self-Hosted Webhook Server

**Best for:** Private repos, enterprise deployments, real-time PR reviews

### Architecture

```
Your GitHub Repo → Webhook → AurumCode Server → Pipelines → Results
```

### Setup (10 minutes)

1. **Clone and build AurumCode:**

```bash
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode
make docker-build
```

2. **Configure environment:**

```bash
cp .env.example .env
# Edit .env with your credentials:
#   GITHUB_TOKEN=ghp_...
#   OPENAI_API_KEY=sk-...
```

3. **Run the server:**

```bash
docker-compose up -d
```

4. **Expose to internet** (for GitHub webhooks):

**Development:**
```bash
ngrok http 8080
# Use the HTTPS URL for webhook
```

**Production:**
- Deploy to cloud provider (AWS, GCP, Azure)
- Use a real domain with HTTPS
- Configure firewall rules

5. **Configure GitHub webhook:**

In your project repo:
- Go to `Settings → Webhooks → Add webhook`
- Payload URL: `https://your-domain.com/webhook`
- Content type: `application/json`
- Secret: (from your `.env` file)
- Events: `Pull requests`, `Pushes`

### Server Management

```bash
# View logs
docker-compose logs -f

# Stop server
docker-compose down

# Restart server
docker-compose restart

# Update to latest
git pull && docker-compose up -d --build
```

---

## 🔄 Approach #3: Hybrid (Best of Both Worlds)

**Use webhook server for PRs, GitHub Actions for documentation.**

### Why Hybrid?

- **Webhook server:** Real-time PR reviews with full context
- **GitHub Actions:** Documentation generation (doesn't need real-time)

### Setup

1. Set up webhook server for code review (Approach #2)
2. Add documentation workflow (from Approach #1)

```yaml
# .github/workflows/aurumcode-docs.yml
on:
  push:
    branches: [main]

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Mpaape/AurumCode@main
        with:
          mode: 'documentation'
          llm_api_key: ${{ secrets.OPENAI_API_KEY }}
```

---

## 📊 Configuration

### Minimal Configuration

Create `.aurumcode/config.yml` in your project root:

```yaml
version: "2.0"

llm:
  provider: "openai"
  model: "gpt-4"
  temperature: 0.3

features:
  code_review: true
  documentation: true
  qa_testing: true

github:
  post_comments: true
  set_status: true
```

### Full Configuration

See [configs/.aurumcode/config.example.yml](../configs/.aurumcode/config.example.yml) for all options.

---

## 🔑 API Keys & Secrets

### Required Secrets

**For GitHub Actions:**
- `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` or `TOTVS_DTA_API_KEY`
- `GITHUB_TOKEN` (automatically provided)

**For Webhook Server:**
- `GITHUB_TOKEN` - Create at https://github.com/settings/tokens
  - Permissions: `repo`, `write:discussion`
- `GITHUB_WEBHOOK_SECRET` - Random string for webhook validation
- LLM API Key (OpenAI/Anthropic/etc.)

### Supported LLM Providers

| Provider | Environment Variable | Models |
|----------|---------------------|--------|
| OpenAI | `OPENAI_API_KEY` | gpt-4, gpt-3.5-turbo |
| Anthropic | `ANTHROPIC_API_KEY` | claude-3-5-sonnet, claude-3-opus |
| TOTVS DTA | `TOTVS_DTA_API_KEY` | Custom (OpenAI-compatible) |
| LiteLLM | `LITELLM_API_KEY` | Multiple providers |
| Ollama | `OLLAMA_API_KEY` | Local models |

---

## 🚀 Quick Start Cheat Sheet

### For Open Source Projects (GitHub Actions)

```bash
# 1. Copy example workflow
curl -o .github/workflows/aurumcode.yml \
  https://raw.githubusercontent.com/Mpaape/AurumCode/main/.github/workflows/examples/all-pipelines.yml

# 2. Add secret in GitHub UI
# Settings → Secrets → New: OPENAI_API_KEY

# 3. Commit and push
git add .github/workflows/aurumcode.yml
git commit -m "Add AurumCode automation"
git push

# 4. Open a PR - AurumCode will review it!
```

### For Private/Enterprise Projects (Webhook)

```bash
# 1. Deploy AurumCode server
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode
docker-compose up -d

# 2. Expose to internet (ngrok for dev)
ngrok http 8080

# 3. Configure webhook in GitHub repo
# Settings → Webhooks → Add webhook → Use ngrok URL

# 4. Open PR - server will review it!
```

---

## 📚 Examples

### Real-World Projects Using AurumCode

- **AurumCode itself** - Uses GitHub Actions for docs, webhook for reviews
- See [.github/workflows/](../.github/workflows/) for our setup

### Pre-configured Workflows

Copy these directly to `.github/workflows/` in your repo:

- [Code Review](.github/workflows/examples/code-review.yml)
- [Documentation](.github/workflows/examples/documentation.yml)
- [QA Testing](.github/workflows/examples/qa-testing.yml)
- [All Pipelines](.github/workflows/examples/all-pipelines.yml)

---

## 🔧 Troubleshooting

### GitHub Action not running

**Check:**
1. Workflow file is in `.github/workflows/`
2. Secrets are configured correctly
3. Permissions are set in workflow (`permissions:` section)

### Webhook not receiving events

**Check:**
1. Server is accessible from internet (test with `curl`)
2. Webhook URL is correct in GitHub settings
3. Webhook secret matches `.env` file
4. Check server logs: `docker-compose logs -f`

### LLM calls failing

**Check:**
1. API key is valid and has credits
2. Model name is correct (e.g., `gpt-4`, not `gpt4`)
3. Check rate limits
4. View logs for detailed error messages

---

## 💡 Best Practices

1. **Start with GitHub Actions** - Easiest to set up
2. **Use incremental docs** - Faster and cheaper than full regeneration
3. **Set coverage thresholds** - Enforce quality gates (80%+)
4. **Monitor costs** - Review LLM usage in provider dashboard
5. **Customize prompts** - Add `.aurumcode/prompts/` for specialized reviews

---

## 📞 Support

- **Issues:** https://github.com/Mpaape/AurumCode/issues
- **Discussions:** https://github.com/Mpaape/AurumCode/discussions
- **Documentation:** https://mpaape.github.io/AurumCode/

---

## 🎯 Next Steps

1. Choose your deployment approach (GitHub Action recommended)
2. Follow the quick start guide above
3. Customize configuration for your needs
4. Open a test PR to see AurumCode in action!

**Welcome to automated code quality! 🤖✨**
