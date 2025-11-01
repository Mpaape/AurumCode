# AurumCode Documentation

> AI-powered code review automation with LLM-based analysis, documentation, and test generation.

## 📚 Documentation Index

### Getting Started
- [**Quickstart Guide**](./QUICKSTART.md) - Get up and running in 5 minutes
- [**Installation**](./INSTALLATION.md) - Detailed setup instructions
- [**Configuration**](./CONFIGURATION.md) - Configure LLM providers and settings

### User Guides
- [**Using the Review System**](./USING_REVIEWS.md) - How to run code reviews
- [**Documentation Generation**](./USING_DOCS.md) - Generate docs from code changes
- [**Test Generation**](./USING_TESTS.md) - Auto-generate unit tests

### Developer Guides
- [**Architecture Overview**](./ARCHITECTURE.md) - System design and components
- [**Development Setup**](./DEVELOPMENT.md) - Contributing and development workflow
- [**Testing Guide**](./TESTING.md) - How to write and run tests
- [**Adding New Features**](./EXTENDING.md) - Extend AurumCode functionality

### Reference
- [**API Reference**](./API_REFERENCE.md) - Go package documentation
- [**Configuration Reference**](./CONFIG_REFERENCE.md) - All configuration options
- [**LLM Providers**](./LLM_PROVIDERS.md) - Supported LLM providers

## 🚀 Quick Links

**For Users:**
- Want to start using AurumCode? → [Quickstart Guide](./QUICKSTART.md)
- Setting up webhooks? → [GitHub Integration](./GITHUB_INTEGRATION.md)
- Configuring LLMs? → [LLM Providers](./LLM_PROVIDERS.md)

**For Developers:**
- Contributing code? → [Development Setup](./DEVELOPMENT.md)
- Understanding the codebase? → [Architecture Overview](./ARCHITECTURE.md)
- Writing tests? → [Testing Guide](./TESTING.md)

## 📦 What is AurumCode?

AurumCode is a comprehensive code review automation system that leverages Large Language Models (LLMs) to:

- **Automated Code Reviews** - AI-powered analysis with ISO/IEC 25010 quality scoring
- **Documentation Generation** - Automatically generate markdown documentation from code changes
- **Test Generation** - Create unit tests for changed code in multiple languages
- **Multi-LLM Support** - Works with OpenAI, Anthropic, Ollama, and more
- **GitHub Integration** - Seamless webhook integration for PR reviews

## 🏗️ Architecture Highlights

```
┌─────────────────┐
│  GitHub Webhook │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────┐
│  HTTP Server    │────▶│  Diff Parser │
└────────┬────────┘     └──────┬───────┘
         │                     │
         ▼                     ▼
┌─────────────────┐     ┌──────────────┐
│ LLM Orchestrator│────▶│ Prompt Builder│
└────────┬────────┘     └──────────────┘
         │
         ▼
┌─────────────────────────────┐
│  Review / Docs / Tests Gen  │
└─────────────────────────────┘
```

## 🎯 Key Features

### Hexagonal Architecture
Clean separation of concerns with ports and adapters pattern for maximum flexibility.

### Provider Agnostic
Plug in any LLM provider with automatic fallback chains and cost tracking.

### Production Ready
- Comprehensive test coverage (>80%)
- Docker-based deployment
- Rate limiting and retry logic
- ETag caching for GitHub API
- Idempotent webhook processing

### Multi-Language Support
Built-in language detection and analysis for:
- Go, Python, JavaScript/TypeScript
- Java, Rust, Ruby, C/C++, C#
- And 20+ more languages

## 📊 Status

| Component | Status | Coverage |
|-----------|--------|----------|
| Core Types | ✅ Complete | 100% |
| Config Loader | ✅ Complete | 79.4% |
| LLM Abstraction | ✅ Complete | 78.2% |
| HTTP Server | ✅ Complete | 96.7% |
| GitHub Client | ✅ Complete | 80.9% |
| Diff Analyzer | ✅ Complete | 83.2% |
| Prompt Builder | ✅ Complete | 83.0% |
| Review Generator | ✅ Complete | 83.3% |
| Doc Generator | ✅ Complete | 100% |
| Test Generator | ✅ Complete | 100% |

## 🤝 Contributing

See [DEVELOPMENT.md](./DEVELOPMENT.md) for setup instructions and [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

## 📄 License

See [LICENSE](../LICENSE) file for details.

## 🔗 Links

- [GitHub Repository](https://github.com/yourusername/aurumcode)
- [Issue Tracker](https://github.com/yourusername/aurumcode/issues)
- [Discussions](https://github.com/yourusername/aurumcode/discussions)
