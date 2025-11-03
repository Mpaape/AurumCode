---
layout: default
title: Home
nav_order: 1
description: "AurumCode - Comprehensive documentation system for multi-language codebases"
permalink: /
---

# AurumCode Documentation

Welcome to the AurumCode documentation system - a comprehensive solution for generating, managing, and serving documentation for multi-language codebases.

## What is AurumCode?

AurumCode is an intelligent documentation generation and management system that supports multiple programming languages and automatically creates beautiful, searchable documentation sites. It extracts documentation from your codebase, normalizes it to markdown format, and publishes it to a Jekyll-powered GitHub Pages site.

## Supported Languages

AurumCode provides native documentation extraction for:

- **Go** - Using `go doc` and `godoc`
- **JavaScript/TypeScript** - Using `typedoc` and `jsdoc`
- **Python** - Using `pydoc` and docstring extraction
- **C#** - Using `xmldocmd` and .NET XML documentation
- **Java** - Using `javadoc`
- **C/C++** - Using Doxygen
- **Rust** - Using `cargo doc`
- **Bash** - Comment-based extraction
- **PowerShell** - Using `platyPS` and comment blocks

## Key Features

- **Multi-language support** - Extract documentation from 10+ programming languages
- **Automated pipeline** - Complete CI/CD integration with GitHub Actions
- **LLM-powered enhancement** - AI-generated welcome pages and summaries
- **Incremental updates** - Only regenerate documentation for changed files
- **Beautiful UI** - Just the Docs theme with search and navigation
- **Docker support** - Containerized deployment option

## Quick Start

```bash
# Extract documentation from your codebase
aurumcode extract --language go --source ./src --output ./docs

# Generate the documentation site
cd docs && bundle install && bundle exec jekyll serve

# View at http://localhost:4000
```

## Documentation Sections

Explore the documentation:

- [**Technology Stack**](_stack/) - Languages, tools, and dependencies
- [**Architecture**](_architecture/) - System design and patterns
- [**Tutorials**](_tutorials/) - Step-by-step guides
- [**API Reference**](_api/) - Detailed API documentation
- [**Roadmap**](_roadmap/) - Future plans and features
- [**Custom Documentation**](_custom/) - Additional resources

## Getting Help

- [GitHub Repository](https://github.com/Mpaape/AurumCode)
- [Report an Issue](https://github.com/Mpaape/AurumCode/issues/new)
- [View Source Code](https://github.com/Mpaape/AurumCode)

## License

AurumCode is open source software. See the repository for licensing details.

---

Last updated: {{ site.time | date: "%B %d, %Y" }}
