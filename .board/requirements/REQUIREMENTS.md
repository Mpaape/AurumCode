# AurumCode requirements registry

This registry is the canonical bridge between product claims, atomic cards,
acceptance scenarios, the consumer demo, and release evidence. A capability is
not complete merely because a card or implementation exists. Its requirement
row must resolve to passing, content-addressed evidence.

Normative words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are interpreted
as BCP 14 terms only when capitalized. Historical PRDs and `.taskmaster/` files
are inputs, not sources of truth.

## Product outcomes

| ID | Verifiable requirement | Release threshold | Required proof families |
|---|---|---|---|
| PR-REV-001 | Review a local Git change with `memory.mode=off`, no network, no provider credentials, and a deterministic fake OpenAI-compatible provider. | One clean-clone E2E succeeds and leaves no persistent memory. | contract, E2E, clean-tree, secret canary |
| PR-REV-002 | Detect actionable planted review defects before human review. | Recall **>=90%** on a versioned, independently labelled corpus; confidence interval and false-positive rate are reported. | benchmark, hidden mutants, reviewer A/B, skeptic |
| PR-REV-003 | Require two complete, context-isolated reviews of every human-authored hunk. Each reviewer has full veto power; lenses are priorities, not divided coverage. | 100% hunk coverage by both reports; zero accepted candidate with either blocking verdict. | contract, E2E, isolation faults |
| PR-REV-004 | Run a pre-sealed skeptical challenge after deterministic gates and before candidate approval. | 100% detection of critical planted mutants; `inconclusive` blocks. | mutation, fault injection, E2E |
| PR-REV-005 | Publish only a revalidated, exact candidate through a separately authorized adapter. | Zero publication with stale identity, analyzer credentials, malformed findings, or forged human approval. | security E2E, replay/tamper tests |
| PR-DOC-001 | Generate owned source documentation deterministically without an LLM. | Clean-clone baseline build succeeds for a Go demo and changes only owned output. | golden, integration, E2E |
| PR-DOC-002 | Keep documentation synchronized after a merge. | Incremental generation completes in **<5 minutes** on the fixed reference repository and resource profile. | benchmark, equivalence, clean-clone E2E |
| PR-DOC-003 | Preserve manual documentation and remove only stale generated pages owned by AurumCode. | 100% preservation of seeded manual regions; owned stale page removed. | golden, mutation, rollback |
| PR-DOC-004 | Offer optional LLM prose enrichment without making it a documentation dependency. | Baseline output remains valid and byte-stable with enrichment disabled or unavailable. | contract, fault injection, E2E |
| PR-TST-001 | Propose tests as reviewable edits and execute them only in a constrained worker. | No source mutation outside an isolated worktree; hostile manifests cannot gain network or host access. | sandbox, security E2E |
| PR-TST-002 | Improve coverage of new or changed code. | Median line coverage delta **>=+20 percentage points** on the versioned eligible corpus; no regression in existing passing tests. | benchmark, revert proof, flake check |
| PR-CFG-001 | Work with zero repository configuration on standard repositories. | **>=80%** of the versioned standard-repository corpus completes the baseline review with no config file. | corpus E2E, compatibility matrix |
| PR-CFG-002 | Parse strict repository config and resolve credentials only at adapter boundaries. | Invalid/unknown config fails closed; secret values never enter config domain objects or evidence. | schema, fuzz, secret canary |
| PR-LLM-001 | Keep the application core provider-neutral and support OpenAI-format endpoints, including LiteLLM. | Every enabled adapter passes one provider contract suite; fake server covers malformed output, timeout, rate limit, streaming, and cancellation. | contract, E2E, fault injection |
| PR-LLM-002 | Support OpenAI Chat Completions and Responses request shapes through explicit capabilities. | Unsupported capabilities fail before network; no silent semantic downgrade. | golden protocol tests |
| PR-LLM-003 | Offer profiles for OpenAI, LiteLLM, Anthropic, Ollama, Azure OpenAI, Gemini OpenAI-compatible, and Bedrock through LiteLLM without leaking vendor types into the core. | Import-boundary test plus provider parity report. | architecture, contract |
| PR-CST-001 | Enforce per-run token/request/cost budgets. | Over-budget calls are rejected before dispatch and reported without secret-bearing payloads. | unit, contract, E2E |
| PR-SCM-001 | Read changes from local Git, GitHub, Gitea, GitLab, and generic Git through one normalized `ChangeSource` port. | Each adapter passes the same normalization and least-privilege suite. | contract, fixtures, E2E |
| PR-SCM-002 | Keep the analyzer read-only and every remote-SCM publisher separately authorized. A publisher accepts only a validated `SignedPublicationBundleV1` that binds the approved `CandidateIdentityV1`, normalized findings, destination, and authorization context. | No read adapter can publish; every publisher independently revalidates the signed bundle, approval, destination, and current SCM head before its first write. | architecture, credentials, stale-head, replay/tamper E2E |
| PR-MEM-001 | Review without memory. `off` is the default and performs no persistent memory read/write. | Review E2E has no dependency on SQLite/index cards and leaves checkout plus `.git/aurumcode` absent. | DAG reachability, syscall/file audit, E2E |
| PR-MEM-002 | Optionally create lightweight repository-local entity memory only when enabled. | `local` stores SQLite/FTS5 under `.git/aurumcode`; bounded size/retention; no vector DB, embeddings, daemon, or tracked files. | integration, size budget, clean-tree |
| PR-MEM-003 | Treat memory as untrusted augmentation and preserve an explicit failure policy. | Augmenter returns baseline plus typed status; composition root alone chooses `fallback_off` or `fail`. | contract, corruption/over-budget faults |
| PR-MEM-004 | Rebuild or remove memory without changing baseline review semantics. | Same snapshot and fake-provider responses yield structurally equivalent baseline findings before and after remove/rebuild. | metamorphic E2E |
| PR-SKL-001 | Load immutable skills as untrusted data under explicit capability and path allowlists. | Malicious skill cannot change approval, access a secret, escape allowed paths, or acquire undeclared tools. | schema, injection, sandbox E2E |
| PR-MCP-001 | Expose read-only MCP tools separately from mutating tools. | Read-only server cannot cause repository/SCM writes; mutating tools require independent authorization and exact candidate identity. | MCP conformance, auth E2E |
| PR-OPS-001 | Resume, cancel, and retry bounded workflows without duplicate external effects. | Crash at each state and replay produce at-most-once publication and deterministic terminal state. | state-model, chaos, E2E |
| PR-EVD-001 | Bind every decision to one canonical candidate identity. | Any change to a bound digest invalidates reviews, skeptic result, publication, and approval. | table-driven stale matrix, replay E2E |
| PR-EVD-002 | Persist sanitized, content-addressed evidence, never raw prompts, responses, credentials, or chain-of-thought. | Canary absent from every sink; hash-chain tamper is detected. | secret scan, tamper E2E |
| PR-SEC-001 | Execute untrusted repository commands in rootless, read-only, no-new-privileges OCI workers with network denied and no engine socket. | Docker and Podman conformance reject privileged, root, host mount, device, socket, writable checkout, capability, namespace, and egress probes. | bootstrap verifier, OCI E2E |
| PR-SUP-001 | Verify toolchains, dependency locks, images, grammars, and third-party tools before first use. | Unpinned or changed input fails the bootstrap supply-chain gate. | lock audit, SBOM, provenance |
| PR-DEMO-001 | Maintain a separate consumer repository fixture that validates every public use case. | Every requirement row maps to at least one deterministic demo scenario; destructive scenarios run only in disposable clones. | trace audit, clean-clone E2E |
| PR-REL-001 | Ship the Go implementation as a reproducible OCI image and removable delivery adapters. | Two clean builds match; Docker and Podman run it; cloud packaging contains no domain logic. | reproducibility, multi-arch, deployment smoke |
| PR-ARCH-001 | Preserve Go and hexagonal/SOLID boundaries. | Core packages import no provider, SCM, container-engine, database-driver, CLI, or cloud adapter package. | import graph, architecture tests |
| PR-COM-001 | Never add AI authorship, co-authorship, signatures, or generated-by attribution to commits. | Proposed commit range contains none of the forbidden trailers/phrases; existing history is not rewritten. | git policy test |

## Canonical candidate identity

Every evidence, review, challenge, publication, appeal, and human-approval
record MUST bind the exact same `CandidateIdentityV1` tuple:

```text
repository_identity
base_tree_digest
head_tree_digest
change_digest
task_spec_digest
configuration_digest
policy_digest
prompt_and_rubric_digest
skill_set_digest
provider_model_backend_identity_digest
toolchain_and_tool_set_digest
dependency_lock_digest
container_image_set_digest
test_manifest_digest
role_context_manifest_digest
```

The tuple contains digests and non-secret aliases only. Provider credentials,
prompt bodies, responses, environment values, and private remote payloads are
not identity fields and MUST NOT be persisted. A byte change in any tuple
input creates a different candidate. No component may define a shorter local
variant.

## Traceability rules

1. Every implementation card lists one or more IDs from this registry and the
   applicable `CR-*` control IDs from the standards crosswalk.
2. Each requirement has an atomic acceptance script in the demo or gate suite.
3. An aggregator card may collect child evidence, but may not implement another
   independently deployable behavior.
4. Numeric targets are release criteria, not per-card estimates. A threshold
   change requires an explicit ADR and migration of the corpus baseline.
5. The stateless review and deterministic documentation baselines are thin
   vertical slices. Optional providers, languages, memory, sites, SCMs, and
   cloud adapters extend them; they never block the baseline.
6. `PRD -> requirement -> card -> acceptance scenario -> demo scenario ->
   evidence digest` is audited before release.
7. `AUR-203` is the release-closure aggregator: every mandatory capability gate
   and every row in the legacy disposition ledger MUST be in its ancestor
   closure. `AUR-204` audits that closure before the release gates continue.
8. `AUR-209` is the sole terminal GA gate. Every mandatory product requirement
   MUST occur in its ancestor closure, and `PR-COM-001` MUST additionally be an
   explicit dependency of that final gate. Optional baseline extensions may run
   later, but a capability claimed by the GA profile cannot remain an unrelated
   terminal node.

## Source disposition

- `.taskmaster/docs/prd.txt`: decomposed and retained as historical input.
- `.taskmaster/docs/prd-documentation-extractors.txt`: claims revalidated; its
  Jekyll-first decision is not inherited automatically.
- `.taskmaster/docs/doc-system-gap-analysis.md`: untrusted gap hypothesis;
  several “exists” claims conflict with the checkout and require
  characterization.
- User direction in the current reconstruction: authoritative for Go-only
  implementation, atomic DAG, optional minimal memory, skeptical container
  evidence, double independent review, skills/MCP, and no AI commit attribution.
