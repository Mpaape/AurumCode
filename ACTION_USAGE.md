# Using AurumCode as a GitHub Action

AurumCode can be used as a reusable GitHub Action in any repository to automatically generate multi-language documentation.

## Quick Start

Add this to your repository's `.github/workflows/docs.yml`:

```yaml
name: Generate Documentation

on:
  push:
    branches: [main]
  pull_request:
  workflow_dispatch:

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate Documentation
        uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: 'docs'

      - name: Deploy to GitHub Pages
        if: github.ref == 'refs/heads/main'
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./docs/_site
```

## Supported Languages

AurumCode automatically detects and generates documentation for:

- **Go** - using gomarkdoc
- **JavaScript/TypeScript** - using TypeDoc
- **Python** - using pydoc-markdown
- **C#** - using xmldocmd
- **C/C++** - using Doxygen + doxybook2
- **Rust** - using rustdoc
- **Bash** - using shdoc
- **PowerShell** - using platyPS

## Inputs

| Input | Description | Default | Required |
|-------|-------------|---------|----------|
| `source-dir` | Source code directory to scan | `.` | No |
| `output-dir` | Output directory for documentation | `docs` | No |
| `languages` | Comma-separated list of languages | `` (all) | No |
| `incremental` | Only process changed files | `false` | No |
| `generate-welcome` | Generate AI-powered welcome page | `false` | No |
| `llm-api-key` | API key for LLM (if generate-welcome=true) | `` | No |
| `llm-base-url` | Custom LLM API endpoint | `` | No |
| `build-jekyll` | Build Jekyll site | `true` | No |

## Outputs

| Output | Description |
|--------|-------------|
| `docs-generated` | Number of documentation files generated |
| `languages-detected` | Languages detected in the repository |

## Examples

### Basic Usage

```yaml
- uses: Mpaape/AurumCode@main
```

### With AI-Powered Welcome Page

```yaml
- uses: Mpaape/AurumCode@main
  with:
    generate-welcome: 'true'
    llm-api-key: ${{ secrets.OPENAI_API_KEY }}
```

### Specific Languages Only

```yaml
- uses: Mpaape/AurumCode@main
  with:
    languages: 'go,python,javascript'
```

### Incremental Mode

```yaml
- uses: Mpaape/AurumCode@main
  with:
    incremental: 'true'
```

## Complete Example with Deployment

```yaml
name: Documentation

on:
  push:
    branches: [main]
    paths:
      - '**.go'
      - '**.js'
      - '**.ts'
      - '**.py'
  workflow_dispatch:

permissions:
  contents: write
  pages: write
  id-token: write

jobs:
  generate-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Generate Documentation
        id: docs
        uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: 'docs'
          generate-welcome: 'true'
          llm-api-key: ${{ secrets.OPENAI_API_KEY }}
          build-jekyll: 'true'

      - name: Show Statistics
        run: |
          echo "Generated ${{ steps.docs.outputs.docs-generated }} files"
          echo "Languages: ${{ steps.docs.outputs.languages-detected }}"

      - name: Deploy to GitHub Pages
        if: github.ref == 'refs/heads/main'
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./docs/_site
          enable_jekyll: false
```

## Local Testing

To test documentation generation locally:

```bash
# Clone AurumCode
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Set API key (optional, for AI features)
export TOTVS_DTA_API_KEY=your_key_here
export TOTVS_DTA_BASE_URL=https://your-endpoint

# Run documentation generation
go run cmd/regenerate-docs/main.go
```

## Requirements

- Repository must have a valid `README.md` for welcome page generation
- For Jekyll build: repository needs `docs/_config.yml` and `docs/Gemfile`
- For AI features: valid API key for LLM provider

## Troubleshooting

### Documentation not generated

- Check that source files exist in `source-dir`
- Verify languages are correctly detected
- Check workflow logs for errors

### Jekyll build fails

- Ensure `docs/Gemfile` exists
- Check Jekyll configuration in `docs/_config.yml`
- Verify all markdown files have valid front matter

### AI features not working

- Verify `llm-api-key` is set correctly
- Check API endpoint is accessible
- Review logs for LLM provider errors

## License

MIT License - See LICENSE file for details
