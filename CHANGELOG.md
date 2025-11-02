# Changelog

All notable changes to the AurumCode project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-11-02

### Added - Use Case #3: QA Testing Pipeline

#### Features
- **Multi-Language Test Execution**
  - Go: `go test` with coverage (`coverage.out`)
  - Python: `pytest` with `coverage.py` (XML format)
  - JavaScript/TypeScript: `jest` with LCOV coverage
  - Automatic language detection from PR diffs
  - Parallel execution with duration tracking

- **Coverage Analysis & Enforcement**
  - Language-specific coverage parsers for all formats
  - Aggregate coverage across multiple languages
  - Configurable coverage gates (default: 80% line coverage)
  - PR fails if coverage gates not met
  - Detailed coverage breakdown (line/branch percentages)

- **Comprehensive QA Reporting**
  - Overall test status with emoji indicators
  - Test counts per language (passed/failed/skipped)
  - Coverage metrics (line/branch/total/covered)
  - Failed test output in formatted code blocks
  - Professional markdown formatting
  - Posted as PR comment with commit status

- **Test Executor Architecture**
  - `GoExecutor`: Runs `go test` and parses coverage.out
  - `PythonExecutor`: Runs `pytest` and parses coverage.xml
  - `JSExecutor`: Runs `npm test` and parses lcov.info
  - Extensible executor interface for additional languages

#### Implementation
- `internal/pipeline/qa_pipeline.go` (357 lines)
  - Complete `Run()` workflow: PR event → diff analysis → test execution → coverage parsing → reporting
  - `detectLanguages()`: Maps changed files to executor languages
  - `parseCoverage()`: Parses language-specific coverage reports
  - `checkCoverageGates()`: Enforces 80% threshold per language
  - `aggregateCoverage()`: Combines coverage from all languages
  - `postQAReport()`: Generates and posts comprehensive markdown report

#### Status
- Use Case #3: 90% Complete (Docker orchestration pending)
- All 3 use cases now operational!

### Added - Use Case #2: Documentation Pipeline

#### Features
- **Conventional Commit Changelog**
  - Automatic parsing of git log (last 50 commits)
  - Groups commits by type: feat, fix, docs, style, refactor, perf, test, build, ci, chore
  - Automatic version numbering with timestamps
  - Optional author attribution and commit hashes
  - Writes to `CHANGELOG.md` in keep-a-changelog format

- **Safe README Updates**
  - Marker-based section replacement (preserves unmarked content)
  - Updates Status section with build info and last updated date
  - Dry-run support for testing changes
  - Change tracking and logging
  - No destructive edits to existing content

- **API Documentation Generation**
  - Auto-detects OpenAPI specs (`.yaml`, `.yml`, `.json`)
  - Supports OpenAPI 2.0 and 3.0
  - Generates markdown documentation from specs
  - Endpoint documentation with parameters and responses
  - Schema definitions included
  - Writes `.md` files next to spec files

- **Static Site Generation**
  - Hugo integration for building static documentation sites
  - Pagefind integration for full-text search indexing
  - Minified output for production
  - Graceful degradation (skips if tools not installed)
  - Configurable via `config.Outputs.DeploySite`

#### Implementation
- `internal/pipeline/docs_pipeline.go` (280 lines)
  - Complete `Run()` workflow: event check → fetch commits → changelog → README → API docs → static site
  - `shouldGenerateDocs()`: Triggers on push to main or merged PR
  - `fetchRecentCommits()`: Uses `git log` with custom format parsing
  - `generateChangelog()`: Groups and writes conventional commits
  - `updateREADME()`: Safe marker-based section updates
  - `generateAPIDocs()`: OpenAPI spec detection and markdown generation
  - `buildStaticSite()`: Hugo + Pagefind build pipeline

#### Configuration
- `config.Outputs.UpdateDocs`: Enable changelog and README updates
- `config.Outputs.DeploySite`: Enable Hugo static site build

#### Status
- Use Case #2: 80% Complete (investigation mode pending)

### Added - Demo Documentation

#### Files
- `docs/DEMO_SETUP_GUIDE.md` (505 lines)
  - Complete step-by-step setup instructions (10 steps)
  - Prerequisites (Go, GitHub, API keys, ngrok)
  - Environment configuration (.env file)
  - AurumCode configuration (.aurumcode/config.yml)
  - GitHub webhook setup
  - 3 comprehensive test scenarios
  - Expected performance benchmarks
  - Troubleshooting guide
  - Cost estimates per PR size

- `docs/CURRENT_STATUS.md` (565 lines)
  - Complete implementation summary
  - All 3 use cases status breakdown
  - Repository structure overview
  - What works right now (full workflows)
  - Test coverage metrics (78-96%)
  - Recent commit history with descriptions
  - TaskMaster status tracking
  - Success criteria checklist
  - Immediate next steps for users

### Added - Pipeline Orchestrator Integration

#### Webhook Handler Integration
- `cmd/server/config.go`
  - Added `GitHubToken` field to ServerConfig
  - Added `OpenAIKey` field to ServerConfig
  - Load tokens from environment variables

- `cmd/server/handlers.go`
  - Implemented `processEvent()` function
  - Creates GitHub client with token
  - Creates OpenAI LLM provider
  - Creates cost tracker with price map
  - Creates LLM orchestrator
  - Loads `.aurumcode/config.yml` configuration
  - Creates Main Pipeline Orchestrator
  - Processes events through all enabled pipelines
  - Comprehensive logging at each step
  - Async event processing in goroutine

#### Impact
- **Use Case #1 (Code Review) now 100% FUNCTIONAL**
- Complete webhook → pipeline → GitHub PR comment workflow
- All components integrated and operational

### Added - Initial Implementation

#### Core Architecture
- **Hexagonal Architecture** with clean separation of concerns
- **Pipeline Orchestrator Pattern** coordinating 3 parallel pipelines
- **Provider-agnostic LLM integration** (OpenAI, Anthropic, Ollama, LiteLLM)
- **Event-driven processing** via GitHub webhooks
- **Customizable configuration** via `.aurumcode/` directory

#### Use Case #1: Code Review Pipeline (95% → 100%)
- `internal/pipeline/review_pipeline.go` (178 lines)
- `internal/review/reviewer.go` - Review orchestration
- `internal/review/iso25010/` - ISO/IEC 25010 quality scoring
- `internal/analyzer/diff.go` - Diff parsing and language detection
- `internal/prompt/builder.go` - Token budgeting and prompt construction
- Features:
  - Fetch PR diffs from GitHub API
  - AI-powered code analysis via LLM
  - Post inline review comments on specific lines
  - Generate ISO/IEC 25010 quality scores
  - Track cost (tokens + USD)
  - Update commit status (success/failure)

#### Configuration & Customization
- `internal/config/` - YAML/JSON config loader with env overrides
- `configs/.aurumcode/config.example.yml` - Complete config template
- `.aurumcode/prompts/` - Markdown prompt templates
- `.aurumcode/rules/` - YAML rule definitions
- `pkg/types/config.go` - Config types with FeaturesConfig

#### LLM Integration
- `internal/llm/orchestrator.go` - Provider-agnostic orchestration
- `internal/llm/provider/openai/` - OpenAI adapter
- `internal/llm/provider/anthropic/` - Anthropic adapter
- `internal/llm/provider/ollama/` - Ollama adapter
- `internal/llm/provider/litellm/` - LiteLLM adapter
- `internal/llm/cost/` - Budget tracking and enforcement
- `internal/llm/httpbase/` - HTTP client with retries and backoff

#### GitHub Integration
- `internal/git/githubclient/` - REST API client (diffs, comments, status)
- `internal/git/webhook/` - Webhook receiver with HMAC validation
- `cmd/server/` - HTTP server with middleware
- Support for webhook signature validation
- Idempotency cache for duplicate delivery prevention

#### Analysis & Review Components
- `internal/analyzer/` - Diff parsing and language detection
- `internal/prompt/` - Prompt building with token budgeting
- `internal/review/iso25010/` - Quality scoring system
- Support for multiple programming languages

#### Testing Infrastructure
- High test coverage (78-96%) across all components
- Integration tests for critical paths
- Fixtures and golden files for regression testing
- Mock implementations for external dependencies

#### Documentation
- `README.md` - Project overview and quickstart
- `docs/QUICKSTART.md` - Getting started guide
- `docs/DEMO.md` - Demo scenarios
- `docs/PRODUCT_VISION.md` - Complete vision with 3 use cases
- `docs/ARCHITECTURE.md` - System architecture
- `docs/ARCHITECTURE_AUDIT.md` - Technical audit and decisions
- `docs/IMPLEMENTATION_STATUS.md` - Current status and roadmap

#### Infrastructure
- `Dockerfile` - Container image definition
- `docker-compose.yml` - Local development setup
- `Makefile` - Build, test, and lint targets
- `.env.example` - Required environment variables
- `.gitignore` - Comprehensive exclusion patterns

#### Statistics
- **189 files** committed
- **34,341 lines of code**
  - Core implementation: ~15,000 lines
  - Tests: ~8,000 lines
  - Documentation: ~3,000 lines
  - Configuration/Tools: ~8,000 lines

## Project Status

### Use Cases
- **Use Case #1: Code Review** - 100% Complete ✅
- **Use Case #2: Documentation** - 80% Complete ✅
- **Use Case #3: QA Testing** - 90% Complete ✅

### Test Coverage
- HTTP Server: 96.7%
- Config Loader: 79.4%
- LLM Orchestrator: 78.2%
- GitHub Client: 80.9%
- Diff Analyzer: 83.2%
- Prompt Builder: 83.0%
- Reviewer: 83.3%

### Contributors
- AurumCode Developer <dev@aurumcode.io>
- Claude (AI Assistant) <noreply@anthropic.com>

## Roadmap

### Future Enhancements
- **Documentation Pipeline**
  - Investigation mode with RAG for deep codebase analysis
  - LLM-based comprehensive documentation generation

- **QA Testing Pipeline**
  - Docker orchestration for isolated test environments
  - Automatic Dockerfile generation via LLM
  - Container lifecycle management

- **Production Features**
  - Database for event history and analytics
  - Authentication and authorization
  - Monitoring and alerting
  - Rate limiting
  - CI/CD pipeline
  - Multi-tenant support

[1.0.0]: https://github.com/Mpaape/AurumCode/releases/tag/v1.0.0
