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

## Try it now

```bash
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode
bash demo.sh
```

`demo.sh` builds the two binaries below and runs all three features offline,
with no LLM key: it generates documentation, reviews a diff with the
deterministic security pass, reviews the same diff through a fixture model
response, and prints the Jekyll publish command. It exits `0` when the
repository does what this README says.

## Features

AurumCode is two binaries, `aurumcode` (code review) and `regenerate-docs`
(documentation). Each feature below has a real command you can run.

- **Review a local diff** - the best first command in the product, because it
  needs no configuration at all:

  ```bash
  aurumcode review --base HEAD~1 --seguranca
  ```

  `--seguranca` runs the project's own deterministic security rules (a regex
  match over the diff, no model call). It is a *named subset* of the full
  catalog, not everything in it - the command itself reports the count, e.g.
  `security pass applied 4 of 8 security rules`. Drop `--seguranca` and set
  `AURUMCODE_LLM_FIXTURE=<path>` (offline, deterministic) or
  `LLM_API_KEY`+`LLM_BASE_URL` for a model-driven review of the same diff.

- **Review a pull request and publish a formal review**:

  ```bash
  aurumcode review --pr 42 --repo owner/project --publicar --modo-publicacao review
  ```

  This submits one GitHub review with `APPROVE`, `COMMENT` or
  `REQUEST_CHANGES`. It needs `GITHUB_TOKEN` (write permission on the pull
  request) and an LLM provider. For GitHub Actions, copy
  [.github/workflows/examples/code-review.yml](.github/workflows/examples/code-review.yml)
  as-is. It is only a caller for the official reusable workflow: no PR number,
  SHA, Go setup, token plumbing, or comment formatting is needed in the
  consumer repository. Configure `LLM_API_KEY` and `LLM_BASE_URL` once under
  repository Settings > Secrets and variables > Actions. The workflow uses
  `gpt-5.6-terra` by default; override it only when needed with the reusable
  workflow's optional `model` input.

  To choose the language once for this repository, add
  `.aurumcode/config.yml`:

  ```yaml
  review:
    language: pt-BR
    publication: review
    inline_comments: true
    context:
      # O prompt embutido continua ativo; estes arquivos são adicionais.
      # prompt: .aurumcode/prompt.md
      # skills:
      #   - .aurumcode/skills/code-review.md
      # docs:
      #   - docs/architecture.md
  ```

  `inline_comments` is optional and adds eligible findings to exact added
  lines inside the formal review. Code suggestions with `proposed_code` become
  native GitHub suggestions that the author can apply; without it, the review
  stays as one clean summary in the PR. The AurumCode process never applies or
  commits a suggestion on its own.

  `review.context` is the only place needed to expand the review with team
  knowledge. `prompt` accepts one repository-wide Markdown file; `skills` and
  `docs` accept short, curated Markdown/text files. They are additive context,
  not replacements for the built-in review prompt. A skill here is simply a
  readable Markdown guidance file; it does not need a runtime or another
  installation. Keep the lists focused so the model receives the guidance
  relevant to this repository. The official workflow now respects
  `publication` from this file; the configured context is read from the base
  branch of the pull request.

  To keep the legacy publication format with separate timeline/line comments,
  use `--modo-publicacao comments --na-linha`. The command-line mode overrides
  the repository setting; accepted modes are `review` and `comments`.

  The caller workflow does not change. Without this file, reviews use
  `en-US`; supported values include `pt-BR`, `en-US`, `es-ES`, `fr-FR`,
  `de-DE`, `it-IT` and `ja-JP`.

- **Generate documentation**:

  ```bash
  go run ./cmd/regenerate-docs
  ```

  See "Local usage" below for details, or
  [.github/workflows/examples/documentation.yml](.github/workflows/examples/documentation.yml)
  to publish it to GitHub Pages on every push to `main`.

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
        uses: Mpaape/AurumCode@v1
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

Go is the generator's own stack: its extractor parses source with `go/parser`
and `go/doc` in-process (since AUR-424) and needs no external tool on `PATH`.
The other extractors each depend on an external tool being on `PATH`; when it
is missing, that language is skipped with a warning and the rest of the run
continues.

| Language | Requires on `PATH` | Available through the Action |
|----------|--------------------|------------------------------|
| Go | none, built in (`go/parser` + `go/doc`) | yes |
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

# Go extraction needs no external tool: it uses go/parser and go/doc
# in-process (since AUR-424).

# Optional: any OpenAI-compatible endpoint, for the AI landing page and review
export LLM_API_KEY=your_key
export LLM_BASE_URL=https://your-endpoint/v1
export LLM_MODEL=claude-3-5-haiku-20241022 # optional; use the model configured by your provider
export AURUMCODE_DOCS_REVIEW=auto   # auto, required or off

go run ./cmd/regenerate-docs
```

The package argument must be the directory (`./cmd/regenerate-docs`). The
command's `package main` spans more than one file, so naming a single file does
not compile.

Without any LLM variable the run still generates every deterministic page. With
the provider configured, the LLM may write the landing page and a bounded
`reviews/docs-review.md` editorial report; it never rewrites API pages.

## Environment variables

Read by `cmd/regenerate-docs`:

| Variable | Effect |
|----------|--------|
| `AURUMCODE_SOURCE_DIR` | tree to scan, default `.` |
| `AURUMCODE_OUTPUT_DIR` | where generated markdown is written, default `.aurumcode` |
| `AURUMCODE_DOCS_DIR` | where `index.md` and `_config.yml` are written, defaults to the output directory |
| `AURUMCODE_BASE_URL` | path the site is published under, e.g. `/my-repo`. **Read with `os.LookupEnv`, so setting it to the empty string is meaningful**: it declares the domain root and suppresses the derivation below, which is what a custom domain needs. Unset means "derive it" |
| `AURUMCODE_LANGUAGES` | comma-separated allow-list, empty means every registered language |
| `AURUMCODE_INCREMENTAL` | `true` documents only files changed since the last run |
| `AURUMCODE_VALIDATE_JEKYLL` | `true` runs `bundle exec jekyll build` in the docs directory after generation |
| `AURUMCODE_DOCS_REVIEW` | `auto` (review when an LLM exists), `required` (fail without a review) or `off` |
| `AURUMCODE_DOCS_LANGUAGE` | language used by the editorial review, default `pt-BR` |
| `AURUMCODE_DEPLOY_GH_PAGES` | `true` makes the run fail: `resolveConfig` (main.go:397-400) returns an error because gh-pages deploy is not implemented in this build; publish the output directory with a dedicated step instead |
| `AURUMCODE_ALLOW_REPO_CODE_EXECUTION` | opt-in list for `rust`, `csharp` |
| `LLM_API_KEY`, `LLM_BASE_URL`, `LLM_MODEL` | OpenAI-compatible endpoint for the landing page and editorial review; the key and base URL are both required |
| `OPENAI_API_KEY` | used only when `LLM_API_KEY`/`LLM_BASE_URL` are unset |

Of these, the Action exposes the output directory (as `output-dir`), the base
path (as `base-path`) and the three `LLM_*` variables (as `llm-api-key`,
`llm-base-url`, `llm-model`).

### Where the site is published

A GitHub Pages project site is served from `owner.github.io/<repo>/`, not from
the domain root. Markdown links are copied into the generated HTML verbatim, so
a link written as `/go/mypkg/` resolves to `owner.github.io/go/mypkg/` and
answers 404. Setting Jekyll's `baseurl` does not fix it: `baseurl` only affects
what the theme resolves through `relative_url`, and the links in the index do
not go through it.

The base path is therefore resolved before the index is written, from the first
source that answers:

1. `AURUMCODE_BASE_URL` / the Action's `base-path` input - the operator's
   intent, and the only source that can declare a domain root;
2. a `baseurl` already present in the output directory's `_config.yml` - that
   file is what Jekyll reads, and AurumCode never overwrites it;
3. `GITHUB_REPOSITORY` - `owner/my-repo` yields `/my-repo`; `owner/owner.github.io`
   yields the root, because a user or organisation site has no prefix;
4. the root, which is what a local run with no CI environment gets.

Sources 1 and 2 disagreeing fails the run rather than publishing a site whose
links and whose theme assets resolve through different prefixes. Source 3 losing
to source 2 is silent on purpose: a file on disk is a fact, a derivation is a
guess.

If your site is on a **custom domain**, the derivation would wrongly add a
`/repo` prefix. Declare the root explicitly with `base-path: ''` (or
`AURUMCODE_BASE_URL=`), or commit a `_config.yml` containing `baseurl: ""`.

Values are normalized: surrounding whitespace, a missing or doubled leading
slash, a trailing slash, and a full URL such as the `base_url` output of
`actions/configure-pages` are all reduced to `/segment` or to the empty root. A
protocol-relative `//host` value is reduced to a path on this site rather than
being allowed to send visitors to another origin.

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

`go build ./...` and `go test ./...` currently FAIL with a package collision:
`tests/unit` and `tests/integration` each hold more than one `package main`
fixture file alongside the real `unit`/`integration` package the acceptance
harness expects (a known, tracked issue, not a broken checkout). Build the
product code by naming its three real roots instead:

```bash
go build ./cmd/... ./internal/... ./pkg/...
```

`demo.sh` (see "Try it now" above) builds and runs both binaries this way and
exits `0`, so it doubles as a smoke test for this command.

## Documentation

- [ACTION_USAGE.md](ACTION_USAGE.md) - every Action input, output and example
- [SETUP_GUIDE.md](SETUP_GUIDE.md) - repository setup for publishing
- [PAGES_SETUP.md](PAGES_SETUP.md) - GitHub Pages source configuration
- [LITELLM_QUICKSTART.md](LITELLM_QUICKSTART.md) - pointing the generator at an
  OpenAI-compatible endpoint

## License

[MIT](LICENSE), Copyright (c) 2026 Mateus Magnus Pimentel Paape.
