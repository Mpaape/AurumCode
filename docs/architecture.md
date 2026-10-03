# Architecture

How AurumCode is organised, how a review flows, and where each kind of
extension plugs in. The principles the owner fixed come first because every
other section follows from them.

## Principles

- **No hardcode.** Languages, rules, scanners and BOM types come from YAML
  catalogs and grammars (`internal/analyzer/language_catalog.yml`,
  `internal/grammar/catalog`, `internal/xbom/catalog`), not from `switch`
  statements. A new one is data.
- **LLM first, with deterministic evidence.** The model reviews; AST, linters,
  Semgrep, SBOM and public sources are evidence it can explain but never
  remove or downgrade. The model's own severity is untrusted.
- **Configuration is YAML and Markdown.** Behaviour is set in `.aurumcode.yml`
  and in skill sections written in Markdown. A central policy and a repository
  resolve per section, with an explicit precedence for each.
- **Reproducible container.** Go builds and tests run only in the shared
  container image from a versioned Dockerfile (`.board/bin/go-shared`, sealed
  `oci-run` for acceptance), never on a developer host.
- **Fail closed.** A gate source that errors, a repository identity that
  cannot be verified, an unusable artifact: the review is inconclusive and is
  never approved by silence.

## Module map

`cmd/aurumcode` parses flags, assembles dependencies and publishes. Business
rules live in `internal/`.

| Package | Responsibility |
| --- | --- |
| `internal/analysis` | Deterministic static-analysis pass over a diff (embedded catalog, go vet). |
| `internal/analyzer` | Diff parsing, language detection from the language catalog, text diffs. |
| `internal/apply` | Turns validated suggestions into safe, applyable patches. |
| `internal/artifacts` | The analysis-data artifact: resolved, age- and digest-checked, cached copy of published scanner data. |
| `internal/changelog` | Builds changelog sections and semantic-version bumps from reviewed commits. |
| `internal/config` | Effective configuration: sections, central policy precedence, gate, exceptions, quality gates. |
| `internal/context` | Bounded, deterministic codebase context and skill reader. |
| `internal/deliberation` | Bounded tool conversation with a model, with no review semantics: `Tool` (`Spec`, `Run`), `Limits` (rounds, tokens, per-tool timeout), argument validation before any run, the `Transcript`, and the typed `LimitError` a caller treats as inconclusive. |
| `internal/dtrack` | Client of OWASP Dependency-Track for the SBOM gate. |
| `internal/evidence` | Content-addressed evidence-bundle manifest. |
| `internal/gate` | The gate pipeline: `Run`, `Result`, `Contributor`, `Pipeline`, the failure rule, the inconclusive ranking (`RankReason`) and the exit decision (`ExitPolicy`). |
| `internal/git` | GitHub client and git access used by the `--pr` path. |
| `internal/governance` | Task specification and dependency-graph model of the board. |
| `internal/grammar` | The only source of per-language structure, from grammar catalogs. |
| `internal/llm` | Providers, orchestration, budget and cost estimation. |
| `internal/memory` | Optional review memory. |
| `internal/prompt` | Prompt building, budgeting, response parsing, comment filter, coverage notes. |
| `internal/render` | Deterministic reports, audit records, SARIF and finding identity. |
| `internal/review` | The reviewer, scope, rules (including dynamic skill rules), the review cache, the review session (`internal/review/session`: phase order and per-source data) and the tools a review offers the model (`internal/review/tools`: optional scanners, codebase context, with the manifest's declared cost). |
| `internal/reviewprofile` | Built-in, versioned reviewer profiles. |
| `internal/sandbox` | Sealed execution profiles. |
| `internal/scanner` | The scanner contract (`Scanner`, `Report`, `Finding.ToIssue`), the closed registry of compiled engines and the executor; engines live in subpackages (`internal/scanner/semgrep`) listed in `internal/scanner/engines`. |
| `internal/sbom` | CycloneDX SBOM generation and validation. |
| `internal/security` | Redaction of secrets from every sink. |
| `internal/supplychain` | Sigstore/Cosign signing of SBOMs and images. |
| `internal/testgen` | Deterministic test proposals from a diff. |
| `internal/xbom` | BOMs beyond the SBOM (build, data, and so on) from catalogs. |

## Review flow

`--base` reviews a local diff and prints a report; `--pr` reviews a pull
request and publishes comments, a formal review and commit statuses. Both are
one review session (`internal/review/session`) and differ only in their
source (where the diff comes from) and their publisher (terminal or GitHub).

`session.Order` is the one phase order; `session.Run` executes it and a step
returns `(exit, done)`, so each early exit keeps its code:

1. **resolve.** Validate the invocation; configuration, central policy, diff,
   context, profiles, memory.
2. **model.** Provider selection and the model's quality pass. With
   `deliberation.enabled`, the model may first ask for tools (below). Its result is
   a typed `gate.ModelOutcome`: `reviewed`, `skipped` (no provider
   configured), `provider failed` (no answer, or a required quality review
   that did not happen) or `parse failed` (an answer that could not be
   validated).
3. **evidence.** The security pass, static analysis, the repository's rule
   configuration, every enabled scanner engine and coverage. One step decides where the security
   findings live (in the review's issues on `--pr`, in their own section on
   `--base`); the verdict-reuse snapshot holds the same findings either way.
4. **gate.** The shared pipeline below, from the session's `gate.Run`. The
   inconclusive motive is `gate.RankReason`: provider failure, skipped
   review, unparseable answer, degraded parse, the first scanner's reason, partial
   coverage, in that order.
5. **publish.** The compliance artifacts (from the gate run's findings), the
   report or the GitHub publication, then `gate.ExitPolicy`.

A source is `cmd/aurumcode` code (`baseReview`, `prReview`) that embeds the
shared `reviewState`; what differs between the two and is not the source or
the publisher is data in `session.Source`:

| Model outcome | `--base` (`session.LocalDiff`) | `--pr` (`session.PullRequest`) |
| --- | --- | --- |
| provider failed | not reviewed (exit 1) | not reviewed only with `--exigir-qualidade`; otherwise the gate decides |
| parse failed | not reviewed (exit 1) | not reviewed only with `--exigir-qualidade`; otherwise the gate decides |
| skipped | the gate decides (`--exigir-qualidade` escalates it to provider failed) | not reachable: no provider is an error |

`gate.ExitPolicy` is one ladder, highest first: a review comment that could
not be posted (1), not reviewed (1), a commit status that could not be
published (1), the gate (a breach 3, a block 1), a requested audit or SARIF
not written (1), `--fail-on` (3), the `--check` status's own code.

The session's collaborators are injected (`reviewDeps`), never package
variables: the clock exceptions are judged against, the scanner executor, the
gate-pipeline observer, the codebase resolver, the prompt builder that
versions the caches, and the environment, read once at the command's edge.

## Review prompt

The review prompt is one template, `internal/prompt/templates/review.md`. Its
body is the system message (instructions, rule catalog, response format); its
named `{{define}}` blocks are the slots the user message is built from. Go code
decides which slots render and with what data; every section title lives in
the template, and a test fails on a `## ` title in a Go string of the
prompt-assembly packages.

User-message slots, in order: change summary, CI context and the budgeted code
changes (`user_header`); PR history; codebase context; review memory;
deterministic evidence (one item per finding, with origin, rule, `file:line`,
severity and a redacted snippet); available tools (with declared cost); review
coverage; repository context. A slot whose input is empty renders nothing, so a
review without that input keeps its previous bytes.

Budgets come from `internal/prompt/templates/limits.yml`: the prompt ceiling
used when the caller sets none, the rule catalog ceiling, and the evidence and
tools ceilings. A list slot over its ceiling admits whole items and states how
many it omitted; code hunks that do not fit are declared in the coverage slot.
Every slot, the repository context included, is counted inside the budget.

The `Reviewer` depends on a `Completer` (`CompleteMessages`), implemented by
`llm.Orchestrator`. The prompt travels as a system and a user message. A
provider with the `llm.MessageCompleter` capability receives them separately;
any other provider receives `System + "\n\n" + User` through `Complete`, and
the cost estimate is taken on that same text. Provider decorators that only
forward requests implement `llm.Unwrapper`, so `llm.As` finds a capability
behind them; a decorator that alters the request must not.

`Reviewer.PromptDigest` is the digest of the exact messages sent.
`Reviewer.RequestCacheKey` combines it with digests of the evidence and of the
tool results (`review/cache.RequestKey`), so evidence that the ceiling left out
of the text still changes the key. The model may attach an `assessment`
(`confirmed`, `disputed`, `needs_context`, with justification) to an issue;
`origin` is written only by the engine, and the parser discards a model's.

## Deliberation

With `deliberation.enabled` and a provider that implements
`llm.ToolCaller` (found through `llm.As`), the model phase is a bounded tool
conversation (`internal/deliberation.Session`) instead of a single call:

- The evidence phase runs every `required` scanner as before; an enabled
  scanner that is not `required` is deferred and offered as the tool
  `scanner_<engine>`, beside `codebase_context` (the bounded context of one
  changed file). The manifest goes to the prompt's tools slot with the
  declared cost and result size of each tool. The model decides; the code
  never asks for a tool by itself.
- Every round is one `Orchestrator.CompleteWithTools` call: its cost is
  reserved before the call and committed after, and fallback only moves
  between providers that are `ToolCaller`s. The answer is requested with a
  JSON Schema derived from `types.ReviewResult` (`llm.SchemaOf`) where the
  provider supports it (`response_format: json_schema` on LiteLLM), JSON
  mode otherwise.
- Each call's arguments are checked against the tool's schema before it
  runs; a refused call is recorded and the model is told why. A requested
  scanner runs through the same path as the evidence phase and joins the
  session's scans: its findings count in the gate with their origin, and a
  missing binary or failed scan is the scan's inconclusive reason.
- Exceeding `max_rounds`, `max_cost_tokens` or `per_tool_timeout_seconds`
  is `deliberation_limit:<limit>`: the review is inconclusive, exits 1 and
  publishes nothing (the gate's own reason is never reached). The transcript
  (offered, requested and not requested tools, each call with redacted
  arguments, duration and summarized result) is printed on stderr and
  written to the audit's `deliberation` field.
- Without a tool-capable provider (or with review profiles), the deferred
  scanners run as before. A review that offered tools skips the per-file
  model cache, and the verdict-reuse key folds in the digests of the tool
  results.

## Gate pipeline

`assembleGatePipeline` is the only place the pipeline is declared. Contributors
apply in this order:

1. `exceptions`: declares that no exception can match when the repository identity is unverified.
2. `verdict-reuse`: reuses or stores a concluded verdict.
3. `policy-skills`: the policy's skill sections.
4. `scanners`: every enabled entry of `quality_gates.scanners` (`quality_gates.sast` is the semgrep alias), one `gate.Scan` each; the contributor names no engine.
5. `embedded-analysis`: the embedded analysis catalog.
6. `security-pass`: the `--seguranca` pass's findings; a finding at or above `fail_on` counts in every `gate.inconclusive` mode (under the `analysis` source).
7. `analysis-data`: the analysis-data artifact.
8. `dependency-track`: the SBOM submission; may replace the redaction filter.

A contributor that returns an ordinary error does not abort and is never read
as "no findings": the result becomes inconclusive (and fails under
`gate.inconclusive: block`). An error wrapped with `gate.Fatal` aborts with
the configuration exit code.

## Extension points

- **A gate contributor.** Implement `gate.Contributor` (`Name`, `Origin`,
  `Apply`) and add one line to `assembleGatePipeline`. Its decision is merged
  into the one `gate.Result`; do not publish from inside it.
- **A scanner.** Add a package under `internal/scanner/<engine>` that
  implements `scanner.Scanner` (`Name`, `Run(ctx, Request) (Report, error)`)
  and registers a `scanner.Engine` (category, typed origin, options
  validator) from its `init`, then add its import to
  `internal/scanner/engines`. Nothing in `internal/gate`, `internal/config`
  or `cmd` changes: `quality_gates.scanners: [{engine: <name>}]` enables it,
  `gate.sources`/`gate.triage` accept its name and category, its findings
  reach the gate line, the audit and the SARIF with its origin, and an
  error, a missing binary or `Complete: false` is inconclusive. The model
  never decides whether a scanner finding exists.
- **A configuration section.** Add the type in `internal/config`, its
  validation, and its precedence between central policy and repository in
  `config.ApplyCentralPolicy`, section by section. Document it in
  `docs/configuration.md`.
- **A BOM type.** Add a catalog entry under `internal/xbom/catalog`; extraction
  and generation read the catalog. SBOM stays in `internal/sbom`.
- **A grammar.** Add a catalog entry under `internal/grammar/catalog`; no Go
  code changes for a language the runtime already supports.

## Guards

Structural tests in `cmd/aurumcode` keep this document true: no production
function of `cmd/aurumcode` exceeds 150 lines, and the package list above is
compared with the packages on disk, so a package without a citation here fails
the test. `cmd/aurumcode` also declares no phase list and no exit ladder of its
own: a slice of phase steps, a review source returning an exit code, or a
second caller of `gate.ExitPolicy` fails the test.
