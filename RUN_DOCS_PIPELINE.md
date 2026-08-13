# Run the Documentation Pipeline

How to run the AurumCode documentation generator yourself, outside of GitHub
Actions.

The generator is `cmd/regenerate-docs`. It is one of two `package main`
commands in this repository — the other is `cmd/aurumcode` (`aurumcode
review`, local code review; see [docs/specs/AUR-430.md](docs/specs/AUR-430.md)
and unrelated to this document). `cmd/regenerate-docs` is the binary the
Docker image builds (`Dockerfile` line 15: `-o regenerate-docs
./cmd/regenerate-docs`), and it is the binary the composite action runs.
Everything below is stated in terms of that program; nothing here describes a
feature it does not have.

## What it does

`cmd/regenerate-docs` builds the extractor pipeline in
`internal/pipeline/extractor_pipeline.go` and:

1. Registers one extractor per supported language.
2. Runs each language's external documentation tool over the source tree.
3. Writes the resulting markdown into the output directory.
4. Scaffolds a Jekyll site next to it (`index.md` and `_config.yml`).
5. Generates an AI welcome page **only** when an LLM provider is configured.

It does not generate a CHANGELOG, does not rewrite README sections, does not
read webhook events, and does not deploy anything.

## Provider configuration

The generator talks to any OpenAI-compatible HTTP endpoint. There is no
built-in provider, no default endpoint, and no vendor-specific variable: you
supply the base URL.

| Variable | Read at | Effect |
|---|---|---|
| `LLM_API_KEY` | `cmd/regenerate-docs/main.go:66` | API key for the OpenAI-compatible endpoint. |
| `LLM_BASE_URL` | `cmd/regenerate-docs/main.go:67` | Base URL of that endpoint. Required together with `LLM_API_KEY`. |
| `LLM_MODEL` | `cmd/regenerate-docs/main.go:74` | Model id. Defaults to `gpt-4o-mini`. |
| `OPENAI_API_KEY` | `cmd/regenerate-docs/main.go:68` | Fallback provider, used only when the pair above is not set. |

Behaviour that follows from those lines:

- `LLM_API_KEY` **and** `LLM_BASE_URL` set → the OpenAI-compatible provider is
  used (`main.go:73-91`).
- `LLM_API_KEY` set without `LLM_BASE_URL` → the provider is skipped with a
  warning (`main.go:92-93`).
- Neither set → `OPENAI_API_KEY` is used if present (`main.go:98-112`).
- Nothing set → documentation is still generated; only the welcome page is
  disabled (`main.go:94-95`, `main.go:117`).

```bash
export LLM_API_KEY=your_key_here
export LLM_BASE_URL=https://llm.example.com/v1   # your own OpenAI-compatible endpoint
export LLM_MODEL=gpt-4o-mini                     # optional
```

The GitHub Action exposes the same three values as the `llm-api-key`,
`llm-base-url` and `llm-model` inputs (`action.yml:283-285`).

## Pipeline configuration

| Variable | Declared at | Default | Effect |
|---|---|---|---|
| `AURUMCODE_SOURCE_DIR` | `main.go:32` | `.` | Tree to document. Resolved to an absolute path. |
| `AURUMCODE_OUTPUT_DIR` | `main.go:33` | `.aurumcode` | Where generated markdown is written. |
| `AURUMCODE_DOCS_DIR` | `main.go:34` | output dir | Where your hand-written pages live. |
| `AURUMCODE_BASE_URL` | `main.go:39` | derived | Path the site is published under, e.g. `/my-repo`. Read with `os.LookupEnv`, so **set-to-empty means "publish at the domain root"** and is different from unset. Unset falls through to the `baseurl` in an existing `_config.yml`, then to `GITHUB_REPOSITORY`, then to the root. |
| `AURUMCODE_LANGUAGES` | `main.go:35` | all | Comma-separated allow-list. |
| `AURUMCODE_INCREMENTAL` | `main.go:36` | `false` | `true`/`false` only; anything else is a startup error. |
| `AURUMCODE_VALIDATE_JEKYLL` | `main.go:37` | `false` | Validate the generated site. |
| `AURUMCODE_DEPLOY_GH_PAGES` | `main.go:38` | `false` | Setting it to `true` **aborts**: deployment is not implemented in this build (`main.go:397-400`). |
| `AURUMCODE_ALLOW_REPO_CODE_EXECUTION` | `repo_code_execution.go:43` | unset | Opt-in list (`csharp`, `rust`) whose toolchains execute code from the documented repository. |

The prefix is not decorative: the composite action already applies its own
`source-dir` input by changing directory, so a bare `SOURCE_DIR` would be
applied twice (`main.go:27-30`).

### Publishing under a base path

Only `AURUMCODE_BASE_URL` is read with `os.LookupEnv` rather than `os.Getenv`,
because for this one variable the empty string is an answer and not an absence:

```bash
# Derive it (project site owner.github.io/my-repo/ -> /my-repo)
GITHUB_REPOSITORY=owner/my-repo go run ./cmd/regenerate-docs

# Declare it
AURUMCODE_BASE_URL=/my-repo go run ./cmd/regenerate-docs

# Declare the domain root - needed on a custom domain, where nothing in the
# environment reveals that no prefix applies
AURUMCODE_BASE_URL= go run ./cmd/regenerate-docs
```

The resolved value does two independent things, and both are needed: it prefixes
every generated link in `index.md` (which is what stops the 404s, since kramdown
copies a markdown destination into the `href` verbatim) and it writes `baseurl:`
into a freshly created `_config.yml` (which is what makes the theme's own assets
resolve). If the output directory already contains a `_config.yml` without a
`baseurl`, AurumCode does not overwrite it - it logs a warning naming the exact
line to add.

## Run it in a container

Go does not need to be installed on the host. Go's own extraction is built in
(`go/parser` + `go/doc`, no external tool, since AUR-424); every other
language's extractor still shells out to that language's own documentation
tool, so only those other languages whose tool is present get documented.

```bash
docker run --rm \
  -v "$PWD":/w:ro -w /w -v "$PWD/out":/out \
  golang:1.21-alpine sh -c '
    AURUMCODE_LANGUAGES=go AURUMCODE_OUTPUT_DIR=/out go run ./cmd/regenerate-docs'
```

The full multi-language toolchain image is `.docker/docs.Dockerfile`.

## Run it with Compose

`docker-compose.test.yml` builds the image and runs the same binary against the
mounted working tree:

```bash
docker compose -f docker-compose.test.yml build
docker compose -f docker-compose.test.yml run --rm test-docs-pipeline
```

`run-docs-pipeline.sh` (or `run-docs-pipeline.bat` on Windows) wraps those two
commands. Compose substitutes `.env` from this directory on its own.

The service sets `entrypoint` because the image's own `ENTRYPOINT` is the
GitHub Action wrapper script; a bare `command:` would be passed to that wrapper
as arguments instead of running the generator.

**Known limitation.** `Dockerfile`'s runtime stage installs only
`ca-certificates`, `git`, `bash`, `curl`, `jq` and `wget` (lines 21-27). It
ships no external documentation toolchain. Go's own extraction needs none (see
above), so Go is never skipped here for a missing tool; every other language
whose tool is still missing (`typedoc`, `pydoc-markdown`, `doxygen`) is
skipped instead, e.g.:

```
[Pipeline] SKIP javascript: required tool not in PATH (typedoc not found: ...)
[Pipeline] SKIP python: required tool not in PATH (pydoc-markdown not found: ...)
[Pipeline] SKIP cpp: required tool not in PATH (doxygen not found: ...)
aurumcode: result=partial docs=<n> skipped=0 failed=0 languages_skipped=cpp,javascript,python output=/tmp/out index=true config=true
```

exit code `0` (`partial` — see the `result` table above), as long as the
documented tree has at least one Go package. Until the image carries the
other toolchains, use the `docker run` recipe above or
`.docker/docs.Dockerfile` to get every language; the Compose service is usable
for wiring and configuration checks.

## Reading the result

The run ends with a machine-readable summary line (`main.go:295`):

```
aurumcode: result=partial docs=25 skipped=0 failed=1 languages_skipped=none output=/out index=true config=true
```

`result` is one of:

| `result` | Meaning | Exit code |
|---|---|---|
| `ok` | Every registered extractor succeeded and markdown exists. | 0 |
| `partial` | Some extractors failed or were skipped, but markdown exists. | 0 |
| `empty` | Nothing was documented; no supported source file was found. | 0 |
| `failed` | The pipeline failed, or claimed partial success with no file on disk. | 1 |

`index=` and `config=` report whether `index.md` and `_config.yml` exist. Both
must be `true` for the output to be publishable as a site: without `index.md` a
published root answers 404, and without `_config.yml` the pages are served as
raw markdown (`main.go:195-209`, `main.go:266-277`).

## Build the site

```bash
cd <output-dir> && bundle install && bundle exec jekyll build
```

## Troubleshooting

**`LLM_API_KEY not set`** — informational, not an error. Documentation is still
generated; only the welcome page is skipped.

**`llm-api-key was given without llm-base-url`** — this product ships no
default endpoint. Set `LLM_BASE_URL` to your own OpenAI-compatible URL.

**`no documentable Go package found in ...`** — Go's own extractor
(`go/parser` + `go/doc`, no external tool) found no buildable, non-test source
for the pinned target (`linux/amd64`) in that directory. The run continues and
reports `result=partial`.

**`AURUMCODE_INCREMENTAL must be one of true/false`** — the boolean variables
accept only `true`/`false`/`1`/`0`/`yes`/`no` (`main.go:431-440`).

**`csharp/rust extractors are disabled`** — deliberate. Their toolchains
execute code from the tree being documented, so they are opt-in through
`AURUMCODE_ALLOW_REPO_CODE_EXECUTION`.
