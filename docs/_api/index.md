---
layout: default
title: API Reference
nav_order: 5
has_children: true
---

# API Reference

Complete API documentation for all AurumCode packages and modules.

## Core Packages

### `internal/documentation/extractors`
Documentation extraction interfaces and implementations.

- **Interface**: `Extractor` - Base interface for all language extractors
- **Types**: `ExtractRequest`, `ExtractResult`, `ExtractionStats`
- **Languages**: Go, JavaScript/TypeScript, Python, C#, Java, C/C++, Rust, Bash, PowerShell

### `internal/documentation/site`
Jekyll site building and management.

- **SiteBuilder**: Main site building orchestration
- **CommandRunner**: Interface for executing shell commands
- **Configuration**: Jekyll and theme configuration

### `cmd/aurumcode`
Command-line interface implementation.

- **Commands**: extract, build, serve, deploy
- **Flags**: language, source, output, incremental

## Language Extractors

Each language extractor implements the `Extractor` interface with these methods:

- `Extract(ctx context.Context, req *ExtractRequest) (*ExtractResult, error)`
- `Validate(ctx context.Context) error`
- `Language() Language`

---

*Note: Detailed API documentation will be automatically extracted from the codebase.*
