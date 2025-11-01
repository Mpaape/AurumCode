# AurumCode

Automated, provider-agnostic code review, documentation, and test generation with strict QA gates.

## Overview

AurumCode delivers automated, reliable pipelines that review code, update docs, and generate tests with enterprise-grade QA and cost controls. It supports multiple Git providers (GitHub, Gitea, generic Git) and various LLM backends (OpenAI, Anthropic, LiteLLM, Ollama).

## Goals

- ≥ 90% of actionable issues caught automatically before human review
- Documentation sync in < 5 minutes after merges
- +20% test coverage growth on new/changed code
- Zero-config for 80% of standard repos
- Per-repo cost under budget, tracked by tokens/requests

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

## Quick Start

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

## Configuration

AurumCode uses `.aurumcode/config.yml` in your repository root. See `configs/default-config.yml` for the full configuration schema.

### Minimal Configuration

```yaml
version: "2.0"

llm:
  provider: "auto"
  model: "sonnet-like"
  temperature: 0.3
  max_tokens: 4000
  budgets:
    daily_usd: 10.0
    per_review_tokens: 8000

outputs:
  comment_on_pr: true
  update_docs: true
  generate_tests: true
  deploy_site: true
```

## Contributing

1. Review the architecture documentation in `docs/`
2. Read the PRD in `.taskmaster/docs/prd.txt`
3. Follow the hexagonal architecture pattern
4. Maintain ≥ 80% test coverage
5. Run `make lint` before committing

## License

[To be determined]

## Support

[To be determined]

