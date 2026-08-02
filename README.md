# AurumCode

Multi-language API documentation generator, usable as a GitHub Action or as a
local command.

It scans a source tree, runs a documentation extractor per detected language,
and writes a Jekyll-ready site next to the code.

## What a run produces

For an output directory of `.aurumcode` (the default), a run writes:

```
.aurumcode/
├── _config.yml            # minimal Jekyll config; created only when absent
├── index.md               # site landing page, links every generated page
└── <language>/<name>.md   # one markdown page per documented unit
```

`_config.yml` declares `theme: jekyll-theme-primer` and `markdown: kramdown`.
It is never overwritten once it exists, so it can be edited freely.
`index.md` is regenerated on every run.

The generator produces markdown and that site scaffold. It does not build the
Jekyll site itself; the Action does that only when publishing is requested.

## Quick start as a GitHub Action

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

      - name: Generate and publish documentation
        id: docs
        uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: '.aurumcode'
          publish: 'pages'
```

Publishing is opt-in. Without `publish`, the Action only generates files and
uploads nothing. Repository Settings > Pages must have "GitHub Actions" as the
source before `publish: pages` can work; see [PAGES_SETUP.md](PAGES_SETUP.md).

See [ACTION_USAGE.md](ACTION_USAGE.md) for every input, output and example.

## Languages

Go is the generator's own stack and is always available. The other extractors
depend on an external tool being on `PATH`; when it is missing, that language is
skipped with a warning and the rest of the run continues.

| Language | Requires on `PATH` | Available through the Action |
|----------|--------------------|------------------------------|
| Go | `gomarkdoc` | yes, installed by the Action |
| JavaScript / TypeScript | `typedoc` | with `extra-toolchains: javascript` |
| Python | `pydoc-markdown` | with `extra-toolchains: python` |
| C / C++ | `doxygen` | with `extra-toolchains: cpp` |
| Bash | `bash` | yes |
| PowerShell | `pwsh` | yes |
| Rust | `cargo` | no, see below |
| C# | `dotnet`, `xmldocmd` | no, see below |

Bash and PowerShell pages are produced by an in-process comment parser; only the
interpreter's presence is checked. No external documentation tool is used for
them.

Rust and C# are held back from the default registration because their toolchains
compile the documented repository, which executes code from it (`build.rs`,
proc-macros, MSBuild tasks). They are enabled only by setting
`AURUMCODE_ALLOW_REPO_CODE_EXECUTION` to a comma-separated list of `rust`,
`csharp` when running the binary directly. The Action never sets that variable,
so Rust and C# cannot be produced through the Action.

## Local usage

```bash
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Required for Go extraction
go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0

# Optional: any OpenAI-compatible endpoint, for the AI-written landing page
export LLM_API_KEY=your_key
export LLM_BASE_URL=https://your-endpoint/v1
export LLM_MODEL=gpt-4o-mini        # optional, defaults to gpt-4o-mini

go run ./cmd/regenerate-docs
```

The package argument must be the directory (`./cmd/regenerate-docs`). The
command's `package main` spans more than one file, so naming a single file does
not compile.

Without any LLM variable the run still generates every page; only the wording of
the landing page changes.

## Environment variables

Read by `cmd/regenerate-docs`:

| Variable | Effect |
|----------|--------|
| `AURUMCODE_SOURCE_DIR` | tree to scan, default `.` |
| `AURUMCODE_OUTPUT_DIR` | where generated markdown is written, default `.aurumcode` |
| `AURUMCODE_DOCS_DIR` | where `index.md` and `_config.yml` are written, defaults to the output directory |
| `AURUMCODE_LANGUAGES` | comma-separated allow-list, empty means every registered language |
| `AURUMCODE_INCREMENTAL` | `true` documents only files changed since the last run |
| `AURUMCODE_VALIDATE_JEKYLL` | `true` runs `bundle exec jekyll build` in the docs directory after generation |
| `AURUMCODE_ALLOW_REPO_CODE_EXECUTION` | opt-in list for `rust`, `csharp` |
| `LLM_API_KEY`, `LLM_BASE_URL`, `LLM_MODEL` | OpenAI-compatible endpoint for the landing page; the key and the base URL are both required |
| `OPENAI_API_KEY` | used only when `LLM_API_KEY`/`LLM_BASE_URL` are unset |

Of these, the Action exposes only the output directory (as `output-dir`) and the
three `LLM_*` variables (as `llm-api-key`, `llm-base-url`, `llm-model`).

## Repository layout

```
AurumCode/
├── cmd/regenerate-docs/          # the generator command
├── internal/
│   ├── documentation/
│   │   ├── extractors/           # one package per language
│   │   ├── incremental/          # git-based change detection
│   │   ├── normalizer/           # Jekyll front matter
│   │   ├── site/                 # site scaffold and the command runner
│   │   └── welcome/              # landing page generation
│   ├── llm/                      # optional LLM providers
│   ├── pipeline/                 # extraction orchestrator
│   └── qa/
├── .docker/docs.Dockerfile       # toolchain image
└── action.yml                    # GitHub Action definition
```

## Development

```bash
go build ./...
go test ./...
```

Both are verified to pass in `golang:1.21-alpine` with the repository mounted at
`/w`.

## Documentation

- [ACTION_USAGE.md](ACTION_USAGE.md) - every Action input, output and example
- [SETUP_GUIDE.md](SETUP_GUIDE.md) - repository setup for publishing
- [PAGES_SETUP.md](PAGES_SETUP.md) - GitHub Pages source configuration
- [LITELLM_QUICKSTART.md](LITELLM_QUICKSTART.md) - pointing the generator at an
  OpenAI-compatible endpoint

## License

No `LICENSE` file is present in this repository, so no license is granted here
yet. Open an issue at https://github.com/Mpaape/AurumCode/issues if you need one
declared.
