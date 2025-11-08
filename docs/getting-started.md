---
layout: default
title: Getting Started
nav_order: 2
---

# Getting Started with AurumCode

AurumCode automatically generates comprehensive documentation for your codebase by extracting API documentation from source code in multiple languages.

## Quick Start

### As a GitHub Action

Add AurumCode to your repository's `.github/workflows/docs.yml`:

```yaml
name: Documentation

on:
  push:
    branches: [main]

permissions:
  contents: write
  pages: write

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: '.aurumcode'

      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./.aurumcode/_site
```

### Local Installation

1. **Clone and build:**
   ```bash
   git clone https://github.com/Mpaape/AurumCode.git
   cd AurumCode
   go build -o aurumcode cmd/regenerate-docs/main.go
   ```

2. **Install language-specific tools:**
   ```bash
   # Go documentation
   go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest

   # JavaScript/TypeScript documentation
   npm install -g typedoc

   # Python documentation
   pip install pydoc-markdown
   ```

3. **Generate documentation:**
   ```bash
   ./aurumcode
   ```

## Supported Languages

AurumCode automatically detects and documents code in:

| Language | Tool | Output |
|----------|------|--------|
| **Go** | gomarkdoc | `.aurumcode/go/` |
| **JavaScript/TypeScript** | TypeDoc | `.aurumcode/javascript/` |
| **Python** | pydoc-markdown | `.aurumcode/python/` |
| **Bash** | shdoc | `.aurumcode/bash/` |
| **C#** | xmldocmd | `.aurumcode/csharp/` |
| **C/C++** | Doxygen | `.aurumcode/cpp/` |
| **Rust** | rustdoc | `.aurumcode/rust/` |
| **PowerShell** | platyPS | `.aurumcode/powershell/` |

## How It Works

1. **Extract:** AurumCode scans your source code and runs language-specific extractors
2. **Normalize:** Converts extracted docs to Jekyll-compatible Markdown with front matter
3. **Build:** Jekyll combines API docs with your custom guides into a searchable site
4. **Deploy:** Publishes to GitHub Pages automatically

## Custom Documentation

Add your own guides, tutorials, and examples in the `docs/` directory:

```markdown
---
layout: default
title: My Guide
nav_order: 3
---

# My Custom Guide

Your content here...
```

Custom pages are preserved during regeneration and merged with auto-generated API docs.

## Configuration

Create `.aurumcode.yml` in your project root to customize behavior:

```yaml
# Source directories to scan
source_dirs:
  - src/
  - internal/

# Output directory for generated docs
output_dir: .aurumcode

# Enable incremental generation (only changed files)
incremental: true

# Languages to extract (empty = all detected)
languages:
  - go
  - javascript
  - python

# Generate LLM-powered welcome page (requires API key)
generate_welcome: false
```

## Next Steps

- **Browse API Reference:** Auto-generated from your source code
- **View Examples:** See how other projects use AurumCode
- **Report Issues:** [GitHub Issues](https://github.com/Mpaape/AurumCode/issues)
Updated: Sat, Nov  8, 2025  1:52:06 PM
