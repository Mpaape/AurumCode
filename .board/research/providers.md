# Provider contracts research

- Researched: 2026-08-04
- Status: design input for `standards/providers` and provider-adapter cards;
  not implementation proof
- Scope: the seven LLM provider surfaces AurumCode's provider layer must
  speak, fixed here as a versioned, dated reference so adapter cards
  implement against a citation instead of memory

## Non-goals

This card fixes the contract; it does not implement an HTTP client, does not
make a network call to any provider, and does not add a Go dependency.
Fixtures under `tests/specs/AUR-019/fixtures/` are recorded from the sources
cited below, not captured from a live request.

## Provider contracts matrix

This is the criteria matrix `AC-001` resolves every provider cited in
`standards/providers/*.yaml` against. A criterion is any column to the right
of `Date`. `tests/acceptance/AUR-019.sh` and `tests/integration/AUR-019_test.go`
read the exact seven provider names from
`tests/specs/AUR-019/provider-names.txt` and never duplicate that list. Every
`Source` cell carries the primary reference as `[label](url)`; the accept
program checks the `url` host against the allowlist in
`tests/specs/AUR-019/allowed-source-domains.txt` and never dereferences it.
`Version` is either a semantic-version-like tag (`v2.53.0`) or a dated API
version string (`2024-10-21`) paired with its own `Date` column; the two are
never the same field. Wire format, Auth, Error taxonomy and Capabilities are
the compact form of the full per-provider record in
`standards/providers/<slug>.yaml`, which this table must not contradict.

| Provider | Source | Version | Date | Wire format | Auth | Error taxonomy | Capabilities |
|---|---|---|---|---|---|---|---|
| OpenAI | [openai-python v2.53.0](https://github.com/openai/openai-python/releases/tag/v2.53.0) | v2.53.0 | 2026-08-03 | transport=http; endpoint=/v1/chat/completions; method=POST; content_type=application/json | scheme=bearer; header=Authorization; binding=header; format=Bearer <OPENAI_API_KEY> | envelope=error.message,error.type,error.param,error.code; taxonomy=invalid_request_error,authentication_error,permission_error,not_found_error,rate_limit_error,api_error,insufficient_quota | streaming=supported; tool_use=supported; structured_output=supported |
| Anthropic | [anthropic-sdk-python v0.120.2](https://github.com/anthropics/anthropic-sdk-python/releases/tag/v0.120.2) | v0.120.2 | 2026-07-28 | transport=http; endpoint=/v1/messages; method=POST; content_type=application/json | scheme=api-key-header; header=x-api-key; binding=header; format=x-api-key: <ANTHROPIC_API_KEY>; anthropic-version: 2023-06-01 | envelope=error.type,error.message,request_id; taxonomy=invalid_request_error,authentication_error,billing_error,permission_error,not_found_error,conflict_error,request_too_large,rate_limit_error,api_error,timeout_error,overloaded_error | streaming=supported; tool_use=supported; structured_output=supported |
| LiteLLM | [litellm v1.95.0](https://github.com/BerriAI/litellm/releases/tag/v1.95.0) | v1.95.0 | 2026-08-02 | transport=sdk-call; endpoint=/chat/completions; method=POST; content_type=application/json | scheme=bearer; header=Authorization; binding=sdk-call; format=Bearer <LITELLM_VIRTUAL_KEY> against the proxy; per-provider api_key/env var (e.g. OPENAI_API_KEY) when calling completion() directly; call=litellm.completion | envelope=maps every backend failure onto the openai Python exception hierarchy (Exception Mapping); no proxy-native envelope field; taxonomy=AuthenticationError,BadRequestError,RateLimitError,APIConnectionError,Timeout,ServiceUnavailableError,ContextWindowExceededError,ContentPolicyViolationError | streaming=supported; tool_use=supported; structured_output=supported |
| Ollama | [ollama v0.32.5](https://github.com/ollama/ollama/releases/tag/v0.32.5) | v0.32.5 | 2026-07-27 | transport=http; endpoint=/api/chat; method=POST; content_type=application/json | scheme=none; header=not-applicable; binding=none; format=none for the local daemon (default http://localhost:11434); Authorization: Bearer <OLLAMA_API_KEY> for Ollama Cloud/turbo mode | envelope=error (single string field), carried on a non-200 HTTP status; taxonomy=400-invalid-request,404-model-not-pulled,500-internal | streaming=supported; tool_use=supported; structured_output=supported |
| Azure | [Azure OpenAI REST API reference (data-plane inference, 2024-10-21 GA)](https://learn.microsoft.com/en-us/azure/ai-services/openai/reference?view=rest-openai-2024-10-21) | 2024-10-21 | 2026-05-19 | transport=http; endpoint=/openai/deployments/{deployment-id}/chat/completions?api-version=2024-10-21; method=POST; content_type=application/json | scheme=api-key-header-or-entra-bearer; header=api-key; binding=header; format=api-key: <AZURE_OPENAI_KEY>, or Authorization: Bearer <Entra ID token> for Microsoft Entra ID / managed identity | envelope=error.message,error.type,error.param,error.code,error.innerError.code,error.innerError.content_filter_results; taxonomy=invalid_request_error,authentication_error,permission_error,not_found_error,rate_limit_error,api_error,ResponsibleAIPolicyViolation | streaming=supported; tool_use=supported; structured_output=supported |
| Gemini | [google-genai v2.16.0](https://github.com/googleapis/python-genai/releases/tag/v2.16.0) | v2.16.0 | 2026-07-30 | transport=http; endpoint=/v1beta/models/{model}:generateContent; method=POST; content_type=application/json | scheme=api-key-query-or-header; header=x-goog-api-key; binding=query-param; format=?key=$GEMINI_API_KEY query parameter, or x-goog-api-key header; Vertex AI variant uses OAuth2/service-account credentials instead | envelope=error.code,error.message,error.status,promptFeedback.blockReason; taxonomy=INVALID_ARGUMENT-400,PERMISSION_DENIED-403,NOT_FOUND-404,RESOURCE_EXHAUSTED-429,INTERNAL-500,blockReason-SAFETY,blockReason-BLOCKLIST,blockReason-PROHIBITED_CONTENT,blockReason-IMAGE_SAFETY | streaming=supported; tool_use=supported; structured_output=supported |
| Bedrock | [Amazon Bedrock Runtime API Reference - Converse (2023-09-30)](https://docs.aws.amazon.com/bedrock/2023-09-30/APIReference/API_runtime_Converse.html) | 2023-09-30 | 2025-12-02 | transport=http; endpoint=/model/{modelId}/converse; method=POST; content_type=application/json | scheme=aws-sigv4; header=Authorization; binding=signed-request; format=AWS SigV4 request signing with IAM credentials; no bearer token; requires the bedrock:InvokeModel action | envelope=no single JSON error field; a named exception type per failure, each with its own HTTP status; taxonomy=AccessDeniedException-403,InternalServerException-500,ModelErrorException-424,ModelNotReadyException-429,ModelTimeoutException-408,ResourceNotFoundException-404,ServiceUnavailableException-503,ThrottlingException-429,ValidationException-400 | streaming=supported; tool_use=supported; structured_output=supported |

## Findings

1. Every provider in this matrix is reachable behind one of two request
   shapes: an OpenAI-compatible chat-completions body (OpenAI, Azure,
   Ollama, and LiteLLM's own unified surface) or a provider-native
   role/content message array (Anthropic's `messages`, Gemini's `contents`,
   Bedrock's `Converse` `messages`). A provider adapter cannot assume either
   shape is universal; `standards/providers/*.yaml` records the concrete
   shape per provider instead of a shared struct.
2. Authentication has three distinct families across the seven: a static
   bearer/API-key header (OpenAI, Anthropic, Azure API-key mode, Gemini),
   request signing with no bearer token at all (Bedrock SigV4), and no
   authentication by default (Ollama's local daemon). A provider contract
   that assumes "one auth header shape fits all" cannot represent Bedrock or
   local Ollama correctly.
3. No provider in this matrix shares Anthropic's or Bedrock's error
   taxonomy verbatim with OpenAI's. Azure is the closest OpenAI mirror (same
   envelope shape, added content-filter fields) precisely because it serves
   the same models through a different deployment and auth model. LiteLLM's
   choice to normalize every backend's failure into the `openai` Python
   exception hierarchy is itself a documented design decision
   ("Exception Mapping"), not evidence that the wire-level envelopes match;
   AurumCode's own error taxonomy work is a separate, dependent card and
   must not assume LiteLLM's mapping is lossless.
4. Structured output is supported natively by all seven surfaces as of the
   versions cited above, but through different parameter names and
   constraints: `response_format.json_schema` (OpenAI, Azure), passthrough
   of the same field gated per model (LiteLLM), `format` accepting either
   the literal string `"json"` or a full schema object (Ollama),
   `output_config.format` (Anthropic), `responseSchema`/`responseMimeType`
   (Gemini), and `outputConfig.textFormat` (Bedrock). This is why the card's
   MUT-001 mutation targets exactly this capability: marking
   `structured_output: supported` without a recorded fixture is the
   single easiest way for a later adapter card to silently regress from a
   researched, versioned claim to an unverified one.
5. LiteLLM and Azure are not independent alternatives to the other five in
   the way AUR-020's alternatives were to each other: LiteLLM is a routing
   and translation layer over the other six, and Azure is a deployment of
   OpenAI's own models under different auth and versioning. Both still need
   their own row here because AurumCode's provider layer talks to each of
   them as a distinct wire contract, never through a shared assumption.
6. Bedrock's `Converse` API is the one surface in this matrix with no single
   JSON `error` object; every failure is a named exception type with its own
   HTTP status. A provider-agnostic error mapper that assumes an
   `error.type`/`error.message` shape exists for every provider is
   contradicted by this row and must branch on Bedrock's exception name
   instead.

## Recorded fixtures

`tests/specs/AUR-019/fixtures/<slug>/<capability>.json` holds one recorded,
non-secret fixture for every `capability: supported` entry in
`standards/providers/<slug>.yaml`, for exactly the three tracked
capabilities in `tests/specs/AUR-019/capabilities.txt` (`streaming`,
`tool_use`, `structured_output`). Each fixture is transcribed from the cited
source's own documented request/response shape; none of them is captured
from a live provider call, and none contains a credential or endpoint
secret of any kind. A capability marked `supported` with no
matching fixture, or a fixture whose `capability`/`provider` fields do not
match the file it lives under, fails `AC-001` with the typed code
`AUR-019/AC-001/unfixtured-capability`.

## Consequences for dependent cards

- Provider adapter cards implement `pkg/types.Provider` against the wire
  format, auth and error taxonomy recorded here and in
  `standards/providers/<slug>.yaml`; a wire detail this card does not cite
  is not yet decided and must not be assumed.
- Structured-output support in an adapter is only claimed for a provider
  whose `standards/providers/<slug>.yaml` marks it `supported` with its
  fixture present; there is no default assumption either way.
- This card ships no client, no retry policy, and no cost table; those are
  separate dependent `O0x` cards.
