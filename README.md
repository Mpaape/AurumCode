# AurumCode

🤖 **Automated AI-Powered Code Quality Platform**

Automated, provider-agnostic code review, documentation, and test generation with strict QA gates.

[![Status](https://img.shields.io/badge/Status-Production%20Ready-brightgreen)](https://github.com/Mpaape/AurumCode)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://go.dev)
[![License](https://img.shields.io/badge/License-TBD-lightgrey)](LICENSE)

## 🚀 Current Status

**Version:** 1.0.0
**Last Updated: 2025-11-02
**Build:** ✅ All 3 Use Cases Operational

| Use Case | Status | Completeness |
|----------|--------|--------------|
| 🔍 **Code Review** | ✅ Production Ready | 100% |
| 📚 **Documentation** | ✅ Functional | 80% |
| 🧪 **QA Testing** | ✅ Functional | 90% |

**Quick Links:**
- 🏢 [**Enterprise Deployment**](docs/ENTERPRISE_DEPLOYMENT.md) - Deploy to 100+ repos in 30 minutes
- 📖 [Demo Setup Guide](docs/DEMO_SETUP_GUIDE.md) - Run a live demo in 30 minutes
- 📊 [Current Status Report](docs/CURRENT_STATUS.md) - Comprehensive project status
- 📝 [Changelog](CHANGELOG.md) - All changes and features
- 🏗️ [Architecture](docs/PRODUCT_VISION.md) - Complete system architecture

## Overview

AurumCode delivers automated, reliable pipelines that review code, update docs, and generate tests with enterprise-grade QA and cost controls. It supports multiple Git providers (GitHub, Gitea, generic Git) and various LLM backends (OpenAI, Anthropic, LiteLLM, Ollama).

## 🎯 Goals & Metrics

### Quality Goals
- ✅ **≥ 90%** of actionable issues caught automatically before human review
- ✅ **Documentation sync in < 5 minutes** after merges
- ✅ **+20% test coverage** growth on new/changed code
- ✅ **Zero-config** for 80% of standard repos
- ✅ **Per-repo cost under budget**, tracked by tokens/requests

### Current Performance
- **Response Time:** 10-20 seconds per PR review
- **Cost per PR:** $0.01-$0.50 USD (based on size)
- **Test Coverage:** 78-96% across components
- **Supported Languages:** Go, Python, JavaScript/TypeScript (extensible)

## Architecture

AurumCode follows a **hexagonal architecture** with:
- **Provider-agnostic LLM** (pluggable adapters)
- **Language-agnostic** inputs/outputs (diff-centric)
- **QA required per phase** (unit, integration, regression, manual sign-off)
- **Everything auditable**: configs in repo, docs versioned, RAG artifacts git-tracked

## Repository Layout

```
aurumcode/
├── cmd/
│   ├── server/                  # HTTP webhook/API server
│   └── cli/                     # Local runs / dry-runs
├── internal/
│   ├── git/                     # Git provider adapters
│   ├── config/                  # Configuration loading
│   ├── llm/                     # LLM provider abstraction
│   ├── analysis/                # Diff parsing & analysis
│   ├── review/                  # Review generation
│   ├── documentation/           # Doc generation
│   ├── testing/                 # Test generation & execution
│   └── deploy/                  # Deployment packaging
├── pkg/types/                   # Shared types/interfaces
├── docs/                        # Hugo site (built to gh-pages)
├── rag/                         # RAG artifacts (JSONL, Parquet)
├── configs/
│   └── default-config.yml       # Default configuration
└── tests/                       # Unit, integration, fixtures
```

## Requirements

- **Go 1.21+**
- **Docker** (for development)

## 🌟 Features

### Use Case #1: Automated Code Review ✅
- **AI-Powered Analysis:** Leverages GPT-4, Claude, or other LLMs for intelligent code review
- **Inline PR Comments:** Posts specific issues on exact lines of code
- **Quality Scoring:** ISO/IEC 25010 quality metrics (security, maintainability, reliability)
- **Cost Tracking:** Real-time token usage and USD cost per review
- **Commit Status:** Automatically sets PR status (success/failure)
- **Customizable Rules:** Define your own review rules in YAML

### Use Case #2: Documentation Generation ✅
- **Conventional Commits:** Auto-generates CHANGELOG.md from commit messages
- **README Updates:** Safe marker-based section updates without breaking content
- **API Documentation:** Generates markdown docs from OpenAPI/Swagger specs
- **Static Sites:** Hugo + Pagefind integration for searchable documentation
- **RAG Investigation:** Deep codebase analysis with retrieval-augmented generation (planned)

### Use Case #3: QA Testing Automation ✅
- **Multi-Language Testing:** Runs tests for Go, Python, JavaScript/TypeScript
- **Coverage Analysis:** Parses and aggregates coverage from multiple frameworks
- **Coverage Gates:** Enforces minimum coverage thresholds (default 80%)
- **Comprehensive Reports:** Detailed QA reports posted to PRs
- **Test Generation:** LLM-based test generation for uncovered code
- **Docker Orchestration:** Isolated test environments (planned)

## 🚀 Quick Start

### Use AurumCode in Your Project (2 minutes) ⚡

Add automated code review, documentation, and QA testing to any GitHub repository:

```bash
# 1. Copy workflow file to your repo
mkdir -p .github/workflows
curl -o .github/workflows/aurumcode.yml \
  https://raw.githubusercontent.com/Mpaape/AurumCode/main/.github/workflows/examples/all-pipelines.yml

# 2. Add API key secret in GitHub
# Settings → Secrets → New: OPENAI_API_KEY

# 3. Open a PR - AurumCode reviews it automatically! 🤖
```

**That's it!** See [Usage Guide](docs/USAGE_GUIDE.md) for detailed setup options.

### Three Ways to Activate AurumCode

| Method | Best For | Setup Time | Cost |
|--------|----------|------------|------|
| **GitHub Action** | Public repos, serverless | 2 min | GitHub Actions minutes |
| **Webhook Server** | Private repos, enterprise | 10 min | Self-hosted infrastructure |
| **Hybrid** | Best of both worlds | 15 min | Combined |

📖 **Full guide:** [Usage Guide](docs/USAGE_GUIDE.md)

### Run AurumCode Demo Locally (30 minutes)

Want to try AurumCode's server mode? Follow our [Demo Setup Guide](docs/DEMO_SETUP_GUIDE.md):
1. Build and run AurumCode server locally
2. Configure GitHub webhook
3. Create test PR
4. Watch automated code review in action

### Prerequisites for Local Development
- **Go 1.21+** - [Download](https://go.dev/dl/)
- **Git** - [Download](https://git-scm.com/downloads)
- **Docker** - [Download](https://docker.com)
- **API Key** - OpenAI, Anthropic, or PROVEDOR DTA

## 🏢 Enterprise Deployment (100+ Repositories)

Deploy AurumCode across your entire organization with **one command**:

```bash
# Run the interactive setup wizard
./scripts/setup-github-app.sh

# Or bulk-enable for all repos
./scripts/bulk-enable-repos.sh
```

**Result:** All 100+ repositories automatically get:
- ✅ Automated code review on every PR
- ✅ Documentation auto-generation on merge
- ✅ QA testing with coverage enforcement
- ✅ Centralized configuration management

**📚 Complete Guide:** [Enterprise Deployment Guide](docs/ENTERPRISE_DEPLOYMENT.md)

### Four Deployment Options:

| Option | Best For | Setup Time |
|--------|----------|------------|
| **GitHub App** ⭐ | 100+ repos, centralized | 30 min |
| **Workflow Templates** | GitHub Actions users | 45 min |
| **Self-Hosted Server** | On-premise/security | 30 min |
| **API Integration** | Existing CI/CD | Custom |

### Using Docker (Recommended)

```bash
# Build the container
make docker-build

# Enter development shell
make docker-shell

# Inside container: run tests
make test

# Inside container: build
make build
```

### Local Development

```bash
# Initialize Go module (if starting fresh)
make init

# Install dependencies
go mod tidy

# Run tests
make test

# Build
make build

# Run linter
make lint

# Generate coverage
make cover
```

## ⚙️ Configuration

AurumCode uses `.aurumcode/config.yml` in your repository root. See `configs/.aurumcode/config.example.yml` for the full configuration schema.

### Environment Variables

Create a `.env` file in your project root:

```bash
# GitHub Configuration
GITHUB_TOKEN=ghp_your_github_token_here
GITHUB_WEBHOOK_SECRET=your_webhook_secret_here

# LLM Provider (choose one)
OPENAI_API_KEY=sk-your_openai_key_here
# OR
ANTHROPIC_API_KEY=sk-ant-your_anthropic_key_here

# Server Configuration
PORT=8080
DEBUG_LOGS=true
```

### Minimal Configuration

```yaml
version: "2.0"

llm:
  provider: "openai"              # or "anthropic"
  model: "gpt-4"                 # or "claude-3-5-sonnet-20241022"
  temperature: 0.3
  max_tokens: 4000
  budgets:
    daily_usd: 50.0
    per_review_tokens: 8000

# Enable/disable features
features:
  code_review: true
  code_review_on_push: false
  documentation: true
  qa_testing: true

# GitHub integration
github:
  post_comments: true
  set_status: true

# Output configuration
outputs:
  comment_on_pr: true
  update_docs: true
  generate_tests: true
  deploy_site: true
```

## 📚 Documentation

### User Guides
- [Demo Setup Guide](docs/DEMO_SETUP_GUIDE.md) - Complete setup walkthrough
- [Quick Start Guide](docs/QUICKSTART.md) - Getting started quickly
- [Current Status](docs/CURRENT_STATUS.md) - Project status and roadmap

### Technical Documentation
- [Product Vision](docs/PRODUCT_VISION.md) - Architecture and use cases
- [Architecture](docs/ARCHITECTURE.md) - System architecture details
- [Implementation Status](docs/IMPLEMENTATION_STATUS.md) - Current implementation state
- [Architecture Audit](docs/ARCHITECTURE_AUDIT.md) - Technical decisions

### Reference
- [Changelog](CHANGELOG.md) - All changes and version history
- [API Reference](docs/API_REFERENCE.md) - API documentation
- [Testing Guide](docs/TESTING.md) - Testing strategies

## 🏗️ Architecture

AurumCode follows a **hexagonal architecture** (ports and adapters) with:

```
┌─────────────────────────────────────────────────────┐
│              Pipeline Orchestrator                   │
│  ┌─────────────┐ ┌──────────────┐ ┌──────────────┐ │
│  │   Review    │ │     Docs     │ │      QA      │ │
│  │  Pipeline   │ │   Pipeline   │ │   Pipeline   │ │
│  └─────────────┘ └──────────────┘ └──────────────┘ │
└─────────────────────────────────────────────────────┘
         ↓                  ↓                  ↓
┌─────────────────────────────────────────────────────┐
│                Core Components                       │
│  • LLM Orchestrator (multi-provider)                │
│  • GitHub Client (webhooks, API)                    │
│  • Analyzer (diff, language detection)              │
│  • Cost Tracker (budgets, limits)                   │
└─────────────────────────────────────────────────────┘
```

**Key Principles:**
- **Provider-agnostic:** Supports OpenAI, Anthropic, Ollama, LiteLLM
- **Language-agnostic:** Works with Go, Python, JavaScript, and more
- **Event-driven:** GitHub webhooks trigger pipelines
- **Customizable:** Markdown prompts + YAML rules in `.aurumcode/`

## 🧪 Testing

```bash
# Run all tests
make test

# Run with coverage
make cover

# Run specific package
go test ./internal/pipeline/...

# Run integration tests
make test-integration
```

**Test Coverage:**
- HTTP Server: 96.7%
- Config Loader: 79.4%
- LLM Orchestrator: 78.2%
- GitHub Client: 80.9%
- Diff Analyzer: 83.2%
- Reviewer: 83.3%

## 🤝 Contributing

We welcome contributions! Please:

1. Review the [architecture documentation](docs/)
2. Read the [PRD](.taskmaster/docs/prd.txt)
3. Follow the hexagonal architecture pattern
4. Maintain ≥ 80% test coverage
5. Run `make lint` before committing
6. Write clear commit messages (conventional commits)

### Development Workflow

```bash
# Clone repository
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint

# Build
make build

# Run server locally
go run cmd/server/main.go
```

## 📊 Project Statistics

- **Total Files:** 189+
- **Lines of Code:** 34,000+
- **Languages:** Go, Python, JavaScript/TypeScript
- **Test Coverage:** 78-96%
- **Commits:** 6 major releases
- **Contributors:** 2 (human + AI pair programming)

## 🚀 Roadmap

### ✅ Completed
- [x] Use Case #1: Code Review (100%)
- [x] Use Case #2: Documentation (80%)
- [x] Use Case #3: QA Testing (90%)
- [x] Pipeline Orchestrator
- [x] Multi-provider LLM support
- [x] GitHub webhook integration
- [x] Cost tracking
- [x] Comprehensive documentation

### 🚧 In Progress
- [ ] Investigation mode for documentation (RAG)
- [ ] Docker orchestration for QA testing

### 📅 Planned
- [ ] Multi-tenant support
- [ ] Database for event history
- [ ] Web dashboard
- [ ] Slack/Discord notifications
- [ ] GitLab and Gitea support
- [ ] Additional language support

## 📝 License

[To be determined]

## 💬 Support

- **Issues:** [GitHub Issues](https://github.com/Mpaape/AurumCode/issues)
- **Discussions:** [GitHub Discussions](https://github.com/Mpaape/AurumCode/discussions)
- **Documentation:** [docs/](docs/)

## 🙏 Acknowledgments

Built with:
- [Go](https://go.dev) - Programming language
- [OpenAI](https://openai.com) / [Anthropic](https://anthropic.com) - LLM providers
- [Hugo](https://gohugo.io) - Static site generator
- [Pagefind](https://pagefind.app) - Search indexing
- [Claude Code](https://claude.com/claude-code) - AI pair programming

---

**Made with ❤️ by the AurumCode team**

🤖 *AI-powered code quality, automated*

# Documentation generated by AurumCode's pipeline
