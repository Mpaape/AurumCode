---
layout: default
title: Getting Started
---

# Getting Started with AurumCode

AurumCode extracts API documentation from a source tree and writes a
Jekyll-ready set of markdown files next to the code.

## As a GitHub Action

```yaml
name: Documentation

on:
  push:
    branches: [main]

jobs:
  docs:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pages: write
      id-token: write
    environment:
      name: github-pages
      url: ${{ steps.docs.outputs.page-url }}
    steps:
      - uses: actions/checkout@v4

      - id: docs
        uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: '.aurumcode'
          publish: 'pages'
```

`publish` is what makes the action build and upload the site; the default,
`none`, only generates files. The repository's Settings > Pages source must be
"GitHub Actions".

## Locally

```bash
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

go build -o aurumcode ./cmd/regenerate-docs

# Go extraction requires gomarkdoc on PATH
go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0

./aurumcode
```

The build argument is the directory, not a file: the command's `package main`
spans several files.

Each further language needs its own tool on `PATH` before that language
produces anything:

```bash
npm install -g typedoc        # JavaScript / TypeScript
pip install pydoc-markdown    # Python
# doxygen for C/C++, from your package manager
```

## Supported languages

| Language | Needs on `PATH` | Output |
|----------|-----------------|--------|
| Go | `gomarkdoc` | `.aurumcode/go/` |
| JavaScript/TypeScript | `typedoc` | `.aurumcode/javascript/`, `.aurumcode/typescript/` |
| Python | `pydoc-markdown` | `.aurumcode/python/` |
| C/C++ | `doxygen` | `.aurumcode/cpp/` |
| Bash | `bash` | `.aurumcode/bash/` |
| PowerShell | `pwsh` | `.aurumcode/powershell/` |

Rust and C# extractors exist but stay unregistered unless
`AURUMCODE_ALLOW_REPO_CODE_EXECUTION` names them, because `cargo doc` and
`dotnet build` execute code from the repository being documented. The GitHub
Action never sets that variable.

A language whose tool is missing is skipped with a warning; the rest of the run
continues.

## How it works

1. **Detect** - the source tree is scanned for files of each registered language.
2. **Extract** - one extractor per language writes markdown into
   `<output-dir>/<language>/`.
3. **Normalize** - Jekyll front matter is added to the generated markdown.
4. **Scaffold** - `_config.yml` (only if absent) and `index.md` are written;
   `index.md` links every markdown page found under the output directory.

Building and deploying the site is not part of the generator. The Action does it
when `publish` is set.

## Configuration

There is no configuration file. Every knob is an environment variable:

| Variable | Effect |
|----------|--------|
| `AURUMCODE_SOURCE_DIR` | tree to scan, default `.` |
| `AURUMCODE_OUTPUT_DIR` | output directory, default `.aurumcode` |
| `AURUMCODE_DOCS_DIR` | where `index.md` and `_config.yml` go, defaults to the output directory |
| `AURUMCODE_LANGUAGES` | comma-separated allow-list, empty means all registered |
| `AURUMCODE_INCREMENTAL` | `true` only documents files changed since the last run |
| `AURUMCODE_VALIDATE_JEKYLL` | `true` runs `bundle exec jekyll build` after generation |
| `AURUMCODE_ALLOW_REPO_CODE_EXECUTION` | opt-in list for `rust`, `csharp` |
| `LLM_API_KEY` + `LLM_BASE_URL` | OpenAI-compatible endpoint for the landing page |
| `LLM_MODEL` | model id, defaults to `gpt-4o-mini` |

## Next steps

- [ACTION_USAGE.md](../ACTION_USAGE.md) - every Action input and output
- [LITELLM_QUICKSTART.md](../LITELLM_QUICKSTART.md) - the optional LLM landing page
- Issues: https://github.com/Mpaape/AurumCode/issues
