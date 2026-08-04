# Changelog

All notable changes to AurumCode are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## Unreleased

No version has been released. The repository carries a single tag, `beta`, and
no `1.0.0`; entries below describe the current state of the tree rather than a
shipped version.

### Fixed

- **Every documentation link 404'd on a site published under a base path.** This
  is the default for a GitHub Pages project site (`owner.github.io/<repo>/`).
  The generated index linked each page at its raw permalink, e.g. `/go/mypkg/`,
  which on that host resolves to `owner.github.io/go/mypkg/` and does not exist.
  A browser check against a published site recorded 404 on 100% of the
  documentation links while the site root itself answered 200.

  Declaring Jekyll's `baseurl` did not work around it: `baseurl` only affects
  URLs the theme resolves through `relative_url`, and kramdown copies a markdown
  link destination into the `href` verbatim. The prefix has to be in the
  markdown the generator writes.

  `site.ScaffoldConfig.BaseURL` and its `applyBaseURL` were already implemented
  and correct; the pipeline's only production call to `site.NewScaffold`
  (`internal/pipeline/extractor_pipeline.go`) never populated the field, so the
  code was unreachable. It is now fed from a resolved base path, which also
  emits `baseurl:` into a newly created `_config.yml` so the theme's own assets
  resolve. The two effects are asserted separately, because a site can have one
  without the other.

### Added

- `AURUMCODE_BASE_URL` environment variable and the matching `base-path` Action
  input, resolving the publication path from, in order: the caller's declared
  value; a `baseurl` already present in the output directory's `_config.yml`;
  a derivation from `GITHUB_REPOSITORY` (`owner/my-repo` → `/my-repo`, and the
  domain root for an `owner.github.io` user or organisation site); the root.

  The variable is read with `os.LookupEnv`, not `os.Getenv`, so setting it to
  the empty string declares the domain root and is distinct from leaving it
  unset. That distinction is what makes a site on a custom domain servable:
  nothing in the CI environment reveals that no prefix applies. For the same
  reason the Action input defaults to the sentinel `auto` rather than to an
  empty string, and is exported to the generator only when the caller moves off
  the sentinel.

  Supplied values are normalized: whitespace, a missing, doubled or trailing
  slash, and a full URL such as the `base_url` output of
  `actions/configure-pages` all reduce to `/segment` or to the empty root, and a
  protocol-relative `//host` is reduced to a path on this site rather than being
  allowed to redirect visitors to another origin.

  A caller-declared value that contradicts a `baseurl` already on disk aborts
  the run. A derivation that loses to a value on disk is silent by design: the
  file is what Jekyll reads, the derivation is a guess. When the output
  directory already holds a `_config.yml` with no `baseurl`, the generator does
  not overwrite it — it logs a warning naming the exact line to add.

  A local run with no CI environment and no declared value resolves to the root,
  so behaviour off CI is unchanged.

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
  `output-dir`, `base-path`, `llm-api-key`, `llm-base-url`, `llm-model`,
  `extra-toolchains` and `publish`, and the outputs `docs-generated`,
  `languages-detected`, `docs-path` and `page-url`.
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
