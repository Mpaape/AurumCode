# Using AurumCode as a GitHub Action

AurumCode can be used as a reusable GitHub Action in any repository to generate
multi-language documentation and publish it to GitHub Pages.

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
        uses: Mpaape/AurumCode@v1
        with:
          source-dir: '.'
          output-dir: '.aurumcode'
```

The run above only generates: it writes markdown plus the site files
(`index.md`, `_config.yml`) under `output-dir` and uploads nothing. Publishing
is opt-in through the `publish` input, described below.

## What the action writes

Under `output-dir` (`.aurumcode` by default), relative to `source-dir`:

```
.aurumcode/
├── _config.yml            # minimal Jekyll config; created only when absent
├── index.md               # landing page, links every generated page
└── <language>/<name>.md   # one markdown page per documented unit
```

## Supported Languages

Go is the action's own stack: its extractor uses the standard library
(`go/parser` + `go/doc`) in-process and needs no external tool on `PATH`
(since AUR-424). Every other language needs its toolchain requested through
`extra-toolchains`, except Bash and PowerShell, which are parsed in process. A
missing optional toolchain is logged as a warning and that language is skipped.

| Language | Needs on `PATH` | How it is provided |
|----------|-----------------|--------------------|
| Go | none, built in (`go/parser` + `go/doc`) | always available |
| JavaScript/TypeScript | `typedoc` | `extra-toolchains: javascript` |
| Python | `pydoc-markdown` | `extra-toolchains: python` |
| C/C++ | `doxygen` | `extra-toolchains: cpp` |
| Bash | `bash` | runner image |
| PowerShell | `pwsh` | runner image |

Bash and PowerShell pages come from an in-process comment parser; only the
interpreter's presence is checked, and no external documentation tool is run for
them.

### Rust and C# are not reachable through this action

The generator refuses to register the Rust and C# extractors unless
`AURUMCODE_ALLOW_REPO_CODE_EXECUTION` names them: both toolchains compile the
documented repository, which executes code from it (`build.rs`, proc-macros,
MSBuild tasks and the assembly's module initializers). This action never sets
that variable, so neither language can be documented through it.

`extra-toolchains: csharp` still installs `xmldocmd`, but nothing consumes it;
the C# pages are not produced. Use the binary directly, on a repository whose
every branch you trust, if you need those two languages.

## Inputs

Every input below exists in `action.yml`; the action declares no other input.

| Input | Description | Default | Required |
|-------|-------------|---------|----------|
| `source-dir` | Directory inside the calling repository to scan, relative to the workspace root | `.` | No |
| `output-dir` | Directory, relative to `source-dir`, where the generator writes markdown and site files. Must be relative and must not escape `source-dir` | `.aurumcode` | No |
| `base-path` | Path the site is published under, e.g. `/my-repo`. `auto` derives it from `GITHUB_REPOSITORY`. Set it to an **empty string** to publish at a domain root — required for a custom domain | `auto` | No |
| `llm-api-key` | API key for the LLM provider; enables the AI landing page and the documentation review | `` | No |
| `llm-base-url` | Base URL of the OpenAI-compatible LLM endpoint, required together with `llm-api-key` | `` | No |
| `llm-model` | Model id for the LiteLLM provider; read only when `llm-api-key` and `llm-base-url` are both set | `` (provider default) | No |
| `docs-review` | `auto` reviews when an LLM is configured; `required` fails without a review; `off` disables only the editorial page | `auto` | No |
| `extra-toolchains` | Comma-separated extra toolchains to install: `javascript`, `python`, `csharp`, `cpp`. Unknown values fail the run | `` | No |
| `publish` | `none` generates only; `artifact` uploads the built site as a Pages artifact for a later deploy job; `pages` uploads and deploys from this job | `none` | No |

The deterministic documentation is generated without any LLM key. When the
three LLM inputs are present, `auto` also writes
`reviews/docs-review.md`. That page evaluates the published content only; it
does not rewrite API pages. The repository can customize the prompt at
`.aurumcode/prompts/documentation/docs-review.md` and add short editorial
skills under `.aurumcode/skills/documentation/`.

### `base-path`: leave it alone unless you use a custom domain

A GitHub Pages **project** site lives at `owner.github.io/<repo>/`. Links in the
generated index are written into the HTML verbatim, so without the `/<repo>`
prefix every documentation link answers 404. The default `auto` derives the
prefix from `GITHUB_REPOSITORY` and is correct for both a project site and an
`owner.github.io` user/organisation site, so most callers never set this.

Set it only when the derivation would be wrong — that is, when you publish at a
domain root that GitHub cannot infer:

```yaml
- uses: Mpaape/AurumCode@v1
  with:
    # Custom domain (CNAME): the site IS the root, so no prefix.
    base-path: ''
```

`auto` and `''` are deliberately different values. `''` means "publish at the
root"; `auto` means "decide for me". Do not use `''` expecting the default
behaviour — on a project site it reinstates the 404s.

If the output directory already contains a `_config.yml` with a `baseurl`, that
value wins over the derivation, and contradicting it with `base-path` fails the
run instead of publishing a site whose links and whose theme assets disagree.

## Outputs

| Output | Description |
|--------|-------------|
| `docs-generated` | Number of markdown files present under the output directory after the run |
| `languages-detected` | Comma-separated languages the extraction pipeline detected |
| `docs-path` | Workspace-relative path of the generated documentation tree |
| `page-url` | GitHub Pages URL, empty unless `publish` was set to `pages` |

## Publishing

`publish` is refused unless the generated tree is a site: the action stops with
an error when `index.md` or `_config.yml` is missing under the output
directory, and again when the Jekyll build produces no `index.html`. It never
uploads raw markdown, which GitHub Pages would serve as a 404 root.

A composite action cannot declare `permissions`, so the calling job grants them:

```yaml
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
          publish: 'pages'
```

The repository's Settings > Pages source must be set to "GitHub Actions".
With `publish: 'artifact'` the job only needs `contents: read`, and a separate
job with the permissions above runs `actions/deploy-pages`.

## Examples

### Basic Usage

```yaml
- uses: Mpaape/AurumCode@v1
```

### Documentation into a different directory

```yaml
- uses: Mpaape/AurumCode@v1
  with:
    output-dir: 'docs'
```

### Extra language toolchains

```yaml
- uses: Mpaape/AurumCode@v1
  with:
    extra-toolchains: 'python,javascript'
```

### With an AI-written welcome page

```yaml
- uses: Mpaape/AurumCode@v1
  with:
    llm-api-key: ${{ secrets.LLM_API_KEY }}
    llm-base-url: ${{ secrets.LLM_BASE_URL }}
    llm-model: 'claude-3-5-haiku-20241022'
```

## Complete Example with Deployment

```yaml
name: Documentation

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  generate-and-deploy:
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

      - name: Generate Documentation
        id: docs
        uses: Mpaape/AurumCode@v1
        with:
          source-dir: '.'
          output-dir: '.aurumcode'
          extra-toolchains: 'python'
          publish: 'pages'

      - name: Show Statistics
        run: |
          echo "Generated ${{ steps.docs.outputs.docs-generated }} files"
          echo "Languages: ${{ steps.docs.outputs.languages-detected }}"
          echo "Site: ${{ steps.docs.outputs.page-url }}"
```

## Local Testing

To test documentation generation locally:

```bash
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Optional, only for the AI-written welcome page
export LLM_API_KEY=your_key_here
export LLM_BASE_URL=https://your-endpoint
export LLM_MODEL=claude-3-5-haiku-20241022

# Same knobs the action exports
export AURUMCODE_OUTPUT_DIR=.aurumcode

# Reproduce what the site will look like when published under a base path.
# Unset (the line below removed) behaves like base-path: 'auto'; exporting it
# empty behaves like base-path: ''.
export AURUMCODE_BASE_URL=/my-repo

go run ./cmd/regenerate-docs
```

## Requirements

- Go needs no external tool, on the action or for a local run: the extractor
  uses `go/parser` and `go/doc` in-process (since AUR-424)
- A `README.md` in `source-dir` for the welcome page
- For `publish`, a Linux runner: the Jekyll build is a container action
- For the AI-written welcome page, a valid key and base URL for an
  OpenAI-compatible endpoint

## Troubleshooting

### Documentation not generated

- Check that source files exist under `source-dir`
- Check the workflow log for the `[Pipeline] Extracting ... documentation` lines
  (`internal/pipeline/extractor_pipeline.go`); `languages-detected` is derived
  from exactly those lines, so an empty value means no language was extracted
- Languages other than Go need their toolchain in `extra-toolchains`
- Rust and C# are never extracted through this action; see the section above

### Publish stops with "missing index.md/_config.yml"

- The generator writes both under `output-dir`; an empty or partial tree means
  generation failed earlier in the log
- Confirm `docs-generated` is greater than zero

### AI features not working

- `llm-api-key` and `llm-base-url` must both be set, otherwise the provider is
  skipped and the run logs a warning
- `llm-model` is ignored unless both of the above are set

## License

[MIT](LICENSE), Copyright (c) 2026 Mateus Magnus Pimentel Paape.
