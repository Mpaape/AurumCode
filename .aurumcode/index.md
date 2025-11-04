---
layout: default
title: Home
nav_order: 1
---

# AurumCode API Documentation

Welcome to AurumCode's automatically generated API documentation.

## 🚀 Quick Start

**Using AurumCode as a GitHub Action:**

```yaml
- uses: Mpaape/AurumCode@main
  with:
    source-dir: '.'
    output-dir: '.aurumcode'
```

**See:** [ACTION_USAGE.md](https://github.com/Mpaape/AurumCode/blob/main/ACTION_USAGE.md)

## 📚 Documentation Sections

This site contains auto-generated API documentation extracted from the AurumCode source code:

- **Go Packages** - Core Go packages and their APIs
- **Internal Packages** - Internal implementation details
- **Pipeline** - Documentation pipeline components
- **LLM Integration** - Language model providers

## 🔧 Supported Languages

AurumCode automatically generates documentation for:

- **Go** (gomarkdoc)
- **JavaScript/TypeScript** (TypeDoc)
- **Python** (pydoc-markdown)
- **C#** (xmldocmd)
- **C/C++** (Doxygen + doxybook2)
- **Rust** (rustdoc)
- **Bash** (shdoc)
- **PowerShell** (platyPS)

## 📖 Navigation

Use the navigation menu on the left to browse the auto-generated API documentation.

---

**Note:** This documentation is automatically generated. To regenerate:

```bash
go run cmd/regenerate-docs/main.go
```

Or trigger the GitHub Actions workflow.
