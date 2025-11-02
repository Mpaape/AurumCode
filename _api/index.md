---
layout: default
title: API Reference
nav_order: 3
has_children: true
---

# API Reference

Complete Go package documentation for AurumCode.

## Core Packages

### Pipeline Orchestration
- **internal/pipeline** - Main orchestrator coordinating all 3 use cases
- **internal/pipeline/review_pipeline.go** - Automated code review workflow
- **internal/pipeline/docs_pipeline.go** - Documentation generation workflow  
- **internal/pipeline/qa_pipeline.go** - QA testing automation workflow

### LLM Integration
- **internal/llm** - LLM provider abstraction with fallback chains
- **internal/llm/provider** - OpenAI, Anthropic, Ollama, LiteLLM adapters
- **internal/llm/cost** - Token usage and budget tracking

### Code Analysis
- **internal/analyzer** - Diff parsing, language detection, metrics
- **internal/review** - Code review engine
- **internal/review/iso25010** - ISO/IEC 25010 quality scoring

### Git Integration
- **internal/git/githubclient** - GitHub API client (diffs, comments, status)
- **internal/git/webhook** - Webhook signature validation and parsing

### Documentation
- **internal/documentation** - Changelog, README, API docs generation
- **internal/docgen** - LLM-powered documentation generator

### Testing
- **internal/testing** - Multi-language test execution
- **internal/testgen** - AI-powered test generation

### Configuration
- **internal/config** - YAML config loading with validation
- **internal/prompt** - Prompt building and response parsing

### Types
- **pkg/types** - Shared types: Event, Diff, ReviewResult, Config

---

_This documentation is auto-generated from source code._
