---
title: "AurumCode Documentation"
date: 2025-11-02
draft: false
---

# Welcome to AurumCode

🤖 **Automated AI-Powered Code Quality Platform**

AurumCode delivers automated, reliable pipelines that review code, update docs, and generate tests with enterprise-grade QA and cost controls.

## Quick Links

- [Demo Setup Guide](/docs/demo-setup/) - Run a live demo in 30 minutes
- [Architecture](/architecture/) - Complete system architecture
- [API Reference](/api/) - API documentation
- [Changelog](/changelog/) - All changes and features

## Features

### 🔍 Use Case #1: Automated Code Review
- AI-Powered code analysis with GPT-4, Claude, or other LLMs
- Inline PR comments on specific lines of code
- ISO/IEC 25010 quality metrics
- Real-time cost tracking
- Automatic commit status updates

### 📚 Use Case #2: Documentation Generation
- Auto-generated CHANGELOG.md from conventional commits
- Safe README section updates
- API documentation from OpenAPI specs
- Hugo + Pagefind static sites
- RAG-powered investigation mode

### 🧪 Use Case #3: QA Testing Automation
- Multi-language test execution (Go, Python, JavaScript)
- Coverage analysis and aggregation
- Coverage gate enforcement
- Comprehensive QA reports
- LLM-based test generation

## Getting Started

```bash
# Clone repository
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Install dependencies
go mod download

# Build
make build

# Run
go run cmd/server/main.go
```

## Status

| Use Case | Status | Completeness |
|----------|--------|--------------|
| 🔍 Code Review | ✅ Production Ready | 100% |
| 📚 Documentation | ✅ Functional | 80% |
| 🧪 QA Testing | ✅ Functional | 90% |

---

**Built with ❤️ by the AurumCode team**
