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
| `internal/analysis` | Deterministic static-analysis pass over a diff, plus the Semgrep runner. |
| `internal/analyzer` | Diff parsing, language detection from the language catalog, text diffs. |
| `internal/apply` | Turns validated suggestions into safe, applyable patches. |
| `internal/artifacts` | The analysis-data artifact: resolved, age- and digest-checked, cached copy of published scanner data. |
| `internal/changelog` | Builds changelog sections and semantic-version bumps from reviewed commits. |
| `internal/config` | Effective configuration: sections, central policy precedence, gate, exceptions, quality gates. |
| `internal/context` | Bounded, deterministic codebase context and skill reader. |
| `internal/dtrack` | Client of OWASP Dependency-Track for the SBOM gate. |
| `internal/evidence` | Content-addressed evidence-bundle manifest. |
| `internal/gate` | The gate pipeline: `Run`, `Result`, `Contributor`, `Pipeline` and the failure rule. |
| `internal/git` | GitHub client and git access used by the `--pr` path. |
| `internal/governance` | Task specification and dependency-graph model of the board. |
| `internal/grammar` | The only source of per-language structure, from grammar catalogs. |
| `internal/llm` | Providers, orchestration, budget and cost estimation. |
| `internal/memory` | Optional review memory. |
| `internal/prompt` | Prompt building, budgeting, response parsing, comment filter, coverage notes. |
| `internal/render` | Deterministic reports, audit records, SARIF and finding identity. |
| `internal/review` | The reviewer, scope, rules (including dynamic skill rules) and the review cache. |
| `internal/reviewprofile` | Built-in, versioned reviewer profiles. |
| `internal/sandbox` | Sealed execution profiles. |
| `internal/sbom` | CycloneDX SBOM generation and validation. |
| `internal/security` | Redaction of secrets from every sink. |
| `internal/supplychain` | Sigstore/Cosign signing of SBOMs and images. |
| `internal/testgen` | Deterministic test proposals from a diff. |
| `internal/xbom` | BOMs beyond the SBOM (build, data, and so on) from catalogs. |

## Review flow

`--base` reviews a local diff and prints a report; `--pr` reviews a pull
request and publishes comments, a formal review and commit statuses. Both run
the same phases and differ only in where the diff comes from and how the
result is published.

1. **Resolve inputs.** Configuration, central policy, diff, context, profiles.
2. **Analyses.** Model review, security and quality passes, SAST, structural
   coverage, embedded analysis.
3. **Gate.** The shared pipeline below decides pass, fail or inconclusive.
4. **Publish and exit.** Report or comments, status, SARIF and audit, exit
   code. A phase returns `(exit, done)`, so each early exit keeps its code.

## Gate pipeline

`assembleGatePipeline` is the only place the pipeline is declared. Contributors
apply in this order:

1. `exceptions`: declares that no exception can match when the repository identity is unverified.
2. `verdict-reuse`: reuses or stores a concluded verdict.
3. `policy-skills`: the policy's skill sections.
4. `sast`: `quality_gates.sast`.
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
- **A scanner.** Add a runner in `internal/analysis`, expose its findings as
  deterministic `ReviewIssue`s with a rule id, enable it from a config section,
  and feed the result to a contributor. The model never decides whether a scanner
  finding exists.
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
the test.
