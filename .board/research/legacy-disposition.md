# Legacy disposition ledger

Status was established from the checkout, not from historical completion
claims. No legacy file is silently carried into the reconstructed release.
Each row must gain a characterization test before code movement or deletion.

| Legacy surface | Observed state | Disposition | Required atomic work |
|---|---|---|---|
| `go.mod`, `go.sum` | Go module and locks exist. | keep then migrate deliberately | Record current toolchain/dependencies; verify checksums and licenses; introduce the new package graph without unpinned updates. |
| `Dockerfile` | Existing build path, not yet accepted as hostile-repo sandbox. | replace | Characterize build; add pinned, non-root multi-stage runtime; prove no engine socket/host mount requirement. |
| `docker-compose.yml`, `docker-compose.test.yml` | Existing orchestration can influence bootstrap. | quarantine then replace | Static denylist audit before any use; build an OCI-engine-neutral test harness; delete obsolete services after equivalence. |
| `action.yml` | Existing public GitHub Action contract. | migrate compatibly | Snapshot inputs/outputs/defaults; introduce v3 analyzer/publisher split; test migration and rollback. |
| `.github/actions/**` | May be absent in this checkout. | characterize absence; create only from spec | Inventory in base SHA; never claim migration for absent paths. |
| `.github/workflows/**` | May be absent in this checkout. | characterize absence; create least-privilege workflows | Test permissions, pin every action by commit digest, separate analyzer and publisher credentials. |
| `cmd/regenerate-docs/**` | Only concrete legacy command entrypoint observed. | characterize then replace behind compatibility shim | Capture flags/output/exit behavior; implement `aurumcode docs`; remove shim only after migration proof. |
| `internal/documentation/extractors/**` | Multiple in-process language extractors and tests exist. | reuse only after contract characterization | Verify output ownership, parser safety, determinism and per-language behavior; wrap behind a port; replace unsafe execution. |
| `internal/documentation/incremental/**` | Cache/detector/manager exist. | reuse concepts, replace persistence | Characterize invalidation and manual-file behavior; integrate with optional derived index without making docs depend on durable memory. |
| `internal/documentation/normalizer/**` | Markdown/frontmatter normalizer exists. | reuse after golden tests | Prove idempotence, safe frontmatter and preservation. |
| `internal/documentation/site/**` | Site runner, scaffold and tests exist; builder/Jekyll/Pagefind implementation files are absent from this checkout. | characterize current files; adapters remain optional | Define a site-builder port; baseline docs emit Markdown without absent adapters; characterize any future site adapter before keeping; do not restore Hugo by default. |
| `internal/pipeline/**` | Extractor pipeline exists; historical PRD calls other pipeline code functional without checkout evidence. | characterize then replace with application use case | Freeze current observable behavior; port one vertical docs flow; remove unreachable/stub code explicitly. |
| `internal/llm/**` | Cost, HTTP, orchestrator and some providers exist. | characterize then migrate behind provider-neutral ports | Run current tests; record wire shapes; remove vendor leakage and secret-bearing objects; retain compatible config where safe. |
| `pkg/types/**` | Shared legacy config/provider/types. | characterize then retire or narrow | Map public consumers; move domain types inward; keep compatibility aliases only with deprecation tests. |
| `configs/default-config.yml`, `configs/.aurumcode/config.example.yml` | Two YAML examples predate config v3. | characterize without applying | AUR-335 records key/type/default-class digests and incompatibilities with zero secret values; migration remains a separate dry-run. |
| `.env.example`, `.env copy.example` | Two environment examples exist; the second path contains a space and must be addressed literally. | characterize; quarantine spaced alias read-only | AUR-358 owns both paths for read-only name/reference-class comparison without loading values. Rename/delete remains forbidden and requires a separate decision. |
| `.aurumcode/prompts/**` | Prompt Markdown exists for review, docs, summary, test and changelog flows. | untrusted inventory | AUR-334 hashes and assigns consumers/disposition with `trusted=false`; bodies never enter agent context during inventory. |
| `.aurumcode/rules/**` | Quality, security and performance YAML rules exist. | untrusted inventory | AUR-351 validates structure and records `authority=none`; no rule may approve, suppress or alter review before a separate policy card. |
| `.aurumcode/bash/*.md` | Generated-looking documentation mirrors shell workflows. | freeze and relate to source | AUR-352 records source relations/orphans/staleness with `executed=false`; snippets are never run. |
| `.aurumcode/iso25010-weights.yml` | Legacy numeric weights exist without proof that current review consumes them safely. | characterize only | AUR-353 records schema/ID/numeric/sum metadata without applying a score or claiming ISO conformity. |
| `.aurumcode/.gitignore` | Local ignore policy exists under the legacy generated-output root. | characterize semantics | AUR-354 uses only synthetic paths in a disposable clone and preserves source/index bytes. |
| Root shell scripts | Multiple docs/action/setup scripts exist. | quarantine | Shellcheck and characterize without credentials/network; replace with Go entrypoints or explicitly delete one script per card. |
| `RUN_DOCS_PIPELINE.md` | A tracked documentation example contained one credential-shaped literal. | redact value; rotate externally; retain history | AUR-333 proves RED/GREEN without printing the match, replaces only the slot with `${OPENAI_API_KEY}`, forbids credential testing and does not rewrite Git history. |
| Root docs (`README.md`, setup/usage guides excluding `RUN_DOCS_PIPELINE.md`) | Public claims conflict with incomplete implementation. | audit and correct | AUR-313 maps each claim to requirement evidence while preserving user-written content. |
| `docs/README.md`, `docs/getting-started.md`, `pages-fix.md` | Legacy public guides have mixed manual ownership and unsupported claims. | claim/ownership inventory | AUR-337 creates a path/range matrix without executing links/snippets or rewriting text. |
| `CHANGELOG.md` | Release history is tracked but manual/generated ownership is not explicit. | preserve and characterize | AUR-356 fixes section ranges, ownership and claim disposition without generating or reordering entries. |
| `GEMINI.md` | Local agent instructions exist outside the canonical CLAUDE/AGENTS pair. | quarantine as untrusted data | AUR-357 records capability claims with `authority=none`, `trusted=false`; no body is loaded during inventory. |
| `Makefile` | Root recipes exist and may invoke legacy scripts/tools. | static characterization only | AUR-336 records target/dependency metadata with `executed=false`; Make and shell are never invoked. |
| `.gitignore`, `.dockerignore` | Git and Docker build-context ignore policies may diverge. | characterize together, do not edit | AUR-355 runs synthetic semantic probes without build, cleanup or host-file enumeration. |
| `.docker/README.md`, `.docker/docs.Dockerfile` | A documentation image path exists outside the root Dockerfile migration. | quarantine-no-build | AUR-338 performs static lock/instruction audit with zero engine, pull, network or build context. |
| `.cursor/mcp.json`, `.gemini/settings.json`, `.mcp.json`, `.claude/settings.local.json` | Client-local MCP/agent settings exist in multiple dialects. | sanitized non-executing inventory | AUR-339 records only dialect/key/identifier digests and counts; values, commands, env and server processes are forbidden. |
| Generated-looking `_api/**`, `index.md`, `.aurumcode/**/*.md` | Mixed generated/manual ownership is unclear. | freeze until ownership is proven | Establish markers/manifest; do not overwrite or delete without an owned-page test. |
| `Gemfile`, `.aurumcode/Gemfile`, `_config.yml`, docs-site files | Jekyll-era dependencies. | optional compatibility only | Lock/audit dependencies; keep only if Jekyll adapter is selected and passes isolated build; otherwise remove through an explicit migration. |
| `demo/**` | Two Go examples, not a consumer repo. | keep as historical fixture; create `demo-repo/` separately | Characterize examples; build an isolated Git-backed consumer with one scenario per public requirement. |
| `tests/fixtures/repo1/**` | Small config fixture. | reuse only after secret/injection audit | Add hostile variants and deterministic history; do not treat as full demo. |
| `.taskmaster/**` | Historical plans and state; many claims do not match checkout. | retain read-only audit input | Never drive execution state from it; supersede via this board and record claim disposition. |
| `.claude/skills/**`, `.agents/skills/**` | Five local routing skills are mirrored byte-for-byte across the two client roots. | keep as untrusted development-tool mirrors | AUR-340 records five pairs, provenance/modes and equivalence with `untrusted=true`; install/refresh remains separately authorized. |
| `CLAUDE.md`, `AGENTS.md` | `CLAUDE.md` is canonical; `AGENTS.md` must resolve to it. | keep | Validate symlink and no-AI-attribution rule; preserve ai-memory managed markers. |

## Machine-readable disposition rules

The inventory is the authoritative file-level snapshot for this card. The
rules below use the intentionally small glob syntax implemented by the card's
acceptance test: `*` matches any byte sequence and `?` matches one byte. Rules
are disjoint; a path must match exactly one rule. The verbs are declarative
only. They do not authorize a move, deletion, execution, migration, or
publication.

### Top-level directories

- disposition: .agents/* -> keep
- disposition: .aurumcode/* -> characterize
- disposition: .board/* -> keep
- disposition: .claude/* -> keep
- disposition: .cursor/* -> characterize
- disposition: .docker/* -> quarantine
- disposition: .github/* -> characterize
- disposition: .gemini/* -> quarantine
- disposition: .taskmaster/* -> characterize
- disposition: _api/* -> characterize
- disposition: cmd/* -> characterize
- disposition: configs/* -> characterize
- disposition: demo/* -> keep
- disposition: docs/* -> characterize
- disposition: internal/* -> characterize
- disposition: pkg/* -> characterize
- disposition: scripts/* -> quarantine
- disposition: standards/* -> keep
- disposition: tests/* -> keep

### Top-level files

- disposition: .ai-memory.toml -> keep
- disposition: .dockerignore -> characterize
- disposition: .env* -> quarantine
- disposition: .gitignore -> characterize
- disposition: .mcp.json -> quarantine
- disposition: ACTION_USAGE.md -> characterize
- disposition: AGENTS.md -> keep
- disposition: CHANGELOG.md -> characterize
- disposition: CLAUDE.md -> keep
- disposition: Dockerfile -> replace
- disposition: GEMINI.md -> quarantine
- disposition: Gemfile -> characterize
- disposition: LITELLM_QUICKSTART.md -> characterize
- disposition: Makefile -> characterize
- disposition: PAGES_SETUP.md -> characterize
- disposition: README.md -> characterize
- disposition: RUN_DOCS_PIPELINE.md -> quarantine
- disposition: SETUP_GUIDE.md -> characterize
- disposition: _config.yml -> characterize
- disposition: action.yml -> migrate
- disposition: docker-compose.test.yml -> quarantine
- disposition: docker-compose.yml -> quarantine
- disposition: generate-docs-simple.sh -> quarantine
- disposition: go.mod -> keep
- disposition: go.sum -> keep
- disposition: index.md -> characterize
- disposition: pages-fix.md -> characterize
- disposition: run-docs-pipeline.bat -> quarantine
- disposition: run-docs-pipeline.sh -> quarantine
- disposition: test-jekyll.sh -> quarantine

## Deletion rule

A deletion card owns exactly the deleted path, its characterization test, and
its migration note. The red proof demonstrates that the legacy behavior or
artifact is still present; green demonstrates either contract-equivalent
replacement or an explicitly approved removal. Bulk “cleanup” cards are
forbidden.

## Sensitive-information rule

Inventory commands report names, types, hashes, and bounded metadata only.
They never print credential values, `.env` bodies, Git credential helpers,
remote URLs containing userinfo, provider requests/responses, or private
repository content into evidence.
