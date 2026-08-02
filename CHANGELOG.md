# Changelog

All notable changes to AurumCode are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## Unreleased

No version has been released. The repository carries a single tag, `beta`, and
no `1.0.0`; entries below describe the current state of the tree rather than a
shipped version.

### Present

- `cmd/regenerate-docs`: documentation generator. Writes `<output-dir>/_config.yml`,
  `<output-dir>/index.md` and `<output-dir>/<language>/<name>.md`.
- Extractors registered by default: Go (`gomarkdoc`), JavaScript/TypeScript
  (`typedoc`), Python (`pydoc-markdown`), C/C++ (`doxygen`), Bash and PowerShell
  (in-process comment parsers).
- Rust (`cargo doc`) and C# (`dotnet` + `xmldocmd`) are registered only when
  `AURUMCODE_ALLOW_REPO_CODE_EXECUTION` names them, because both compile the
  documented repository and so execute code from it.
- `action.yml`: composite GitHub Action with the inputs `source-dir`,
  `output-dir`, `llm-api-key`, `llm-base-url`, `llm-model`, `extra-toolchains`
  and `publish`, and the outputs `docs-generated`, `languages-detected`,
  `docs-path` and `page-url`.
- Optional LLM landing page through an OpenAI-compatible endpoint
  (`LLM_API_KEY` + `LLM_BASE_URL`), with `OPENAI_API_KEY` as a fixed-endpoint
  fallback.
- Incremental extraction via git change detection, reachable through
  `AURUMCODE_INCREMENTAL` when running the binary directly.

### Not present

- No webhook server, HTTP endpoint or long-running service.
- No code-review or test-generation output; the binary generates documentation.
- No gh-pages branch deployment inside the generator: publishing is done by the
  Action's `publish` input, and `AURUMCODE_DEPLOY_GH_PAGES` is rejected with an
  error if set.
- No configuration file is read. Every knob is an environment variable or an
  Action input.

## Project information

- Repository: https://github.com/Mpaape/AurumCode
- Documentation: [README.md](README.md), [ACTION_USAGE.md](ACTION_USAGE.md)
- License: no `LICENSE` file is present in this repository
