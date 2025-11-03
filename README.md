# AurumCode 🔮

**Multi-Language Documentation Generator with AI-Powered Enhancements**

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![GitHub Action](https://img.shields.io/badge/GitHub-Action-2088FF?logo=github-actions)](action.yml)
[![Documentation](https://img.shields.io/badge/Docs-GitHub%20Pages-success)](https://mpaape.github.io/AurumCode/)

> Automatically generate beautiful, searchable documentation for your codebase in 8+ programming languages.

## ✨ Features

- 🌐 **8 Language Support**: Go, JavaScript/TypeScript, Python, C#, C/C++, Rust, Bash, PowerShell
- 🤖 **AI-Powered**: Optional LLM-enhanced welcome pages and summaries
- 🔄 **Incremental Mode**: Only regenerate docs for changed files
- 📚 **Jekyll Integration**: Beautiful, searchable static sites
- 🚀 **GitHub Action**: Use as `Mpaape/AurumCode@main` in any repository
- 🐳 **Docker Support**: Complete toolchain in one container
- ⚡ **Fast**: Parallel processing and smart caching

## 🚀 Quick Start

### Use as GitHub Action (Recommended)

Add to `.github/workflows/docs.yml`:

```yaml
name: Documentation

on:
  push:
    branches: [main]

jobs:
  generate-docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate Documentation
        uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: 'docs'

      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./docs/_site
```

**See [ACTION_USAGE.md](ACTION_USAGE.md) for complete documentation.**

### Local Usage

```bash
# Clone the repository
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Set API keys (optional - for AI features)
export TOTVS_DTA_API_KEY=your_key_here
export TOTVS_DTA_BASE_URL=https://your-endpoint

# Generate documentation
go run cmd/regenerate-docs/main.go

# Build Jekyll site
cd docs
bundle install
bundle exec jekyll serve

# Open http://localhost:4000
```

## 📖 Supported Languages & Tools

| Language | Tool | Output Format |
|----------|------|---------------|
| **Go** | gomarkdoc | Markdown API docs |
| **JavaScript/TypeScript** | TypeDoc | Markdown API docs |
| **Python** | pydoc-markdown | Markdown API docs |
| **C#** | xmldocmd | Markdown from XML docs |
| **C/C++** | Doxygen + doxybook2 | Markdown from Doxygen |
| **Rust** | rustdoc | HTML (convertible) |
| **Bash** | shdoc | Markdown from comments |
| **PowerShell** | platyPS | Markdown from help |

## 🏗️ Architecture

```
AurumCode/
├── cmd/
│   ├── regenerate-docs/     # Documentation generator CLI
│   ├── server/              # Webhook server (legacy)
│   └── cli/                 # CLI interface (legacy)
├── internal/
│   ├── documentation/
│   │   ├── extractors/      # Language-specific extractors
│   │   ├── normalizer/      # Jekyll front matter processor
│   │   ├── welcome/         # AI welcome page generator
│   │   ├── incremental/     # Git-based change detection
│   │   └── site/            # Jekyll builder
│   ├── pipeline/            # Documentation pipeline orchestrator
│   └── llm/                 # LLM integration (optional)
├── docs/                    # Jekyll documentation site
├── .docker/                 # Docker toolchain container
└── action.yml              # GitHub Action definition
```

## 🔧 Configuration

### GitHub Action Inputs

```yaml
- uses: Mpaape/AurumCode@main
  with:
    source-dir: '.'              # Source code directory
    output-dir: 'docs'           # Output directory
    languages: ''                # Comma-separated list (empty = all)
    incremental: 'false'         # Enable incremental mode
    generate-welcome: 'false'    # AI-powered welcome page
    llm-api-key: ''              # API key for LLM
    llm-base-url: ''             # Custom LLM endpoint
    build-jekyll: 'true'         # Build Jekyll site
```

### Environment Variables

```bash
# For AI-powered features
TOTVS_DTA_API_KEY=sk-your_key
TOTVS_DTA_BASE_URL=https://your-endpoint

# Or use OpenAI
OPENAI_API_KEY=sk-your_key
```

## 📚 Documentation

- **[SETUP_GUIDE.md](SETUP_GUIDE.md)** - Complete setup instructions
- **[ACTION_USAGE.md](ACTION_USAGE.md)** - Using AurumCode as GitHub Action
- **[PRODUCT_VISION.md](docs/PRODUCT_VISION.md)** - Project vision and roadmap

## 🐳 Docker

Use the pre-configured Docker container with all tools:

```bash
# Build the container
docker build -f .docker/docs.Dockerfile -t aurumcode-docs .

# Run documentation generation
docker run --rm \
  -v $(pwd):/workspace \
  -w /workspace \
  aurumcode-docs \
  go run cmd/regenerate-docs/main.go
```

## 🧪 Development

```bash
# Install dependencies
go mod tidy

# Run tests
go test ./...

# Run linter
golangci-lint run

# Build locally
go build -o bin/aurumcode cmd/regenerate-docs/main.go
```

## 📊 Project Status

- ✅ 8 language extractors implemented
- ✅ Jekyll site builder with just-the-docs theme
- ✅ Incremental documentation support
- ✅ GitHub Action for external repositories
- ✅ Docker container with all tools
- ✅ CI/CD with GitHub Actions
- 🚧 AI-powered welcome pages (optional)
- 🚧 Multi-language README generation

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📝 License

MIT License - See [LICENSE](LICENSE) for details

## 🙏 Acknowledgments

Built with:
- [Go](https://go.dev) - Core language
- [Jekyll](https://jekyllrb.com) - Static site generator
- [Just the Docs](https://just-the-docs.github.io/just-the-docs/) - Jekyll theme
- Various language-specific documentation tools

---

**Made with ❤️ for developers who love good documentation**

🤖 *Automated documentation, simplified*
