# Pointing AurumCode at an OpenAI-compatible endpoint

The generator can have its landing page (`index.md`) written by an LLM. It talks
to one endpoint shape: an OpenAI-compatible `POST {base-url}/chat/completions`.
Any gateway that speaks it works - a LiteLLM proxy is one example, and the
provider package is named after it.

Everything else the generator produces is unaffected: without credentials the
run still writes every extracted page, `_config.yml` and a landing page built
from the repository's `README.md`.

## Configuration

Three variables, read by `cmd/regenerate-docs`:

| Variable | Meaning |
|----------|---------|
| `LLM_API_KEY` | sent as `Authorization: Bearer <key>` |
| `LLM_BASE_URL` | prefix of the request URL; `/chat/completions` is appended |
| `LLM_MODEL` | model id; defaults to `gpt-4o-mini` when unset |

`LLM_API_KEY` and `LLM_BASE_URL` are required together. With only one of them
the provider is not constructed and the run logs a warning.

```bash
export LLM_API_KEY=your-key
export LLM_BASE_URL=http://localhost:4000/v1   # your gateway
export LLM_MODEL=gpt-4o-mini

go run ./cmd/regenerate-docs
```

Through the GitHub Action the same three values are the `llm-api-key`,
`llm-base-url` and `llm-model` inputs. The action registers the key with the log
masker before any other step runs.

## The OPENAI_API_KEY fallback

If `LLM_API_KEY`/`LLM_BASE_URL` are unset and `OPENAI_API_KEY` is set, the
OpenAI provider is used instead. Its base URL is fixed at
`https://api.openai.com/v1` in `internal/llm/provider/openai/openai.go`; there
is no environment variable that redirects it. To reach a self-hosted or proxied
endpoint, use `LLM_BASE_URL`, not `OPENAI_API_KEY`.

## Checking it worked

The run logs one of these lines:

```
✓ LiteLLM configured (<base-url>)
⚠️  LLM_BASE_URL not set - skipping LiteLLM provider
ℹ️  LLM_API_KEY not set - LLM features will be disabled (docs generation will still work)
```

A configured run also prints the remaining per-run and daily budget at the end.

## Troubleshooting

**The landing page is not LLM-written.** Both `LLM_API_KEY` and `LLM_BASE_URL`
must be set; check the log line above.

**Connection refused.** `LLM_BASE_URL` must be reachable from wherever the
generator runs, and must be the prefix that `/chat/completions` is appended to.

**Model not found.** The gateway must expose the id in `LLM_MODEL`; unset means
`gpt-4o-mini`.
