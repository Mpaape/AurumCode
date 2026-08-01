# AurumCode Reconstruction Board

This directory is the executable source of truth for the reconstruction. It
maps work as an atomic dependency graph; it is deliberately not a calendar or
a human-effort estimate.

The pre-existing `.taskmaster/` backlog is preserved as audit input only. Its
historical `done` values do not prove that a capability exists and never drive
the execution queue; AUR-001 and AUR-002 characterize those claims explicitly.

## Operating model

1. A card moves from `backlog` to `ready` only when every `depends_on` card is
   in `done`. `ready` authorizes an isolated test designer, not a builder.
2. A feature or defect card moves to `doing` only after its acceptance test
   demonstrably fails for the intended behavioral reason. A characterization,
   policy-data, or behavior-preserving refactor instead records an initial
   `GREEN`, proves a targeted mutation `RED`, restores `GREEN`, and locks that
   baseline before any refactor. One builder then owns the card and may only
   change its declared paths; compile, dependency, loader, permission, and
   infrastructure failures are never valid red evidence.
3. The builder produces a patch and evidence, never an approval.
4. Two isolated reviewers each evaluate every hunk and all ten review
   dimensions against the exact same immutable evidence bundle. Reviewer A
   prioritizes correctness/design; Reviewer B prioritizes security/adversarial
   behavior. The priorities never divide coverage.
5. A separate skeptical approver must falsify the result by running the card's
   mutation and observing the acceptance proof fail.
6. Any blocker rejects the patch. A disagreement is decided by an isolated
   third reviewer; it is never resolved by majority vote.
7. Approval uses only the canonical `CandidateIdentityV1` in
   [`requirements/REQUIREMENTS.md`](requirements/REQUIREMENTS.md), including
   repository/base/head/change, spec, config, policy, prompt/rubric, skills,
   provider/model/backend, tools/toolchain, dependency lock, images, tests, and
   role-context manifests. Any bound change invalidates every review.
8. Only the coordinator integrates an approved patch. Commits must never carry
   AI authorship, co-authorship, signatures, or generated-by attribution.

No prose claim is evidence. A completed card has raw command output, exit code,
artifact hashes, reviewer verdicts, and the skeptical mutation result under
`.board/evidence/<card-id>/`.

Acceptance programs emit observations only. They cannot write evidence, issue
a verdict, or assert approval. The coordinator captures bounded output,
recomputes the canonical evidence hash chain, and materializes a candidate-
bound bundle. A repository JSON value such as `authenticated: true` or
`"proved": true` is untrusted data: a claim is never a proof.

There is no human gate. Every card is proved by agent-driven validation —
including a headless-browser observation of the promised effect where the
feature reaches a user (AUR-423) — plus a skeptical mutation that must make the
acceptance proof fail. `done` is judged on the recomputable evidence bundle
alone. The owner is consulted for exactly two things, and they are never a
manifest field: creating an account with a third party in the owner's name, and
spending money. Both live in `cards/blocked-on-owner`.

## State directories

- `cards/backlog`: specified but dependency-blocked work.
- `cards/ready`: dependencies complete; an isolated test designer may work.
- `cards/doing`: red evidence confirmed and exactly one active builder.
- `cards/review`: immutable candidate awaiting independent review.
- `cards/done`: fully accepted work.
- `cards/blocked-on-owner`: work the owner alone can unblock, and nothing else.
  It is reserved for **account creation** (registering with a third party in the
  owner's name) and **budget** (spending money). A card does not land here
  because it is risky, architectural, or far-reaching.
- `evidence`: generated proof bundles; secrets and raw prompts are forbidden.
- `decisions`: architecture and arbitration records.
- `research`: normative sources and dated technical research.

## Parallel offices

| Office/desk | Stable path ownership | Responsibility |
|---|---|---|
| `O00-governance` | `.board/`, gate/evidence policies | Board, gates, evidence, approvals |
| `O00-research` | `.board/research/`, `standards/` | Versioned primary-source research |
| `O00-security` | `internal/security/`, security fixtures | Redaction, canaries, supply chain, threats |
| `O01-core` | `internal/domain/`, `internal/application/`, `internal/ports/` | Hexagonal domain and use-case contracts |
| `O02-index` | `internal/git/`, `internal/index/`, `internal/context/` | Git facts, stateless context, optional derived index |
| `O03-providers` | `internal/model/`, provider fakes | Provider-neutral LLM execution |
| `O04-agents` | `internal/agents/`, `internal/skills/` | Agent protocol, skills, capabilities |
| `O05-review` | `internal/review/`, review use case/renderers | Deterministic and semantic code review |
| `O06-scm` | `internal/scm/`, `internal/publisher/`, Action jobs | Change sources and privileged publication |
| `O07-docs` | `internal/documentation/` | Documentation generation and validation |
| `O08-runtime` | `internal/orchestrator/`, run store | Workflow, checkpoints, loops, observability |
| `O08-testqa` | `internal/testgen/`, `internal/qa/`, sandbox | Test proposals and skeptical execution |
| `O09-demo` | `demo-repo/`, adversarial/eval fixtures | Provider/SCM/language end-to-end proof |
| `O10-memory` | `internal/memory/`, optional context decorator | Opt-in repository-local memory; never review baseline |
| `O11-mcp` | `internal/mcp/` | MCP transport, tools, auth, injection defense |
| `O12-release` | release audit, migration, runbooks | Compatibility and independently reproduced release |
| `O13-delivery` | `deploy/`, delivery entrypoints | OCI and removable cloud adapters |
| `O14-legacy` | one explicitly named legacy path per card | Characterize, migrate, retain, or delete legacy artifacts |

Cross-office files require an explicit integration card. Path ownership keeps
parallel patches disjoint; dependency edges, not global phase barriers, control
when work may begin.

Review is stateless by default. [`ADR-0001`](decisions/ADR-0001-optional-local-memory.md)
defines `off`, `ephemeral`, and `local` memory modes. AUR-094 proves review with
no persistent index; AUR-225 integrates memory only after that workflow exists.

## Card contract

Every card follows [`CARD_TEMPLATE.md`](CARD_TEMPLATE.md). In particular:

- `accept` is one decisive, containerized command that names the expected
  artifact or test and fails before implementation;
- every concrete TDD test uses a closed `path::selector` reference located
  directionally within `paths`; an optional `read_paths` field separately
  allowlists read-only inputs without granting write ownership;
- `mutation` intentionally removes or corrupts the promised behavior and must
  make `accept` fail;
- unit, contract, integration, and E2E requirements are explicit rather than
  inferred;
- outputs are proposals unless a separately authorized publisher card applies
  them;
- credentials arrive only through environment variables or mounted secret
  files and may never enter logs, prompts, fixtures, evidence, or git history.

Every implementation card also lists canonical product requirement IDs and
`CR-*` controls, allowed and forbidden paths, pre/postconditions, Given/When/
Then scenarios, public-contract effects, data class, trust boundaries,
documentation, compatibility, migration, rollback, and one falsifiable
mutation per scenario/boundary. The card is invalid when these are boilerplate
or cannot distinguish the promised behavior from a no-op.

## Bootstrap and OCI engines

No project-built approver validates its own sandbox foundation. Until the Go
harness exists, `.board/bin/oci-run` invokes a pinned external image through a
detected Docker or Podman CLI. It materializes only `paths`, optional
`read_paths`, and the acceptance program into an ephemeral container; it never
bind-mounts the repository. Because both engines reject copying into an already
read-only rootfs, a first container is created but never started, receives the
allowlisted staging tree, and is sealed as an ephemeral derived image. Only a
second container executes untrusted code, and that runtime is read-only. The
derived image and both container records are removed by the runner. The
`bootstrap-readonly-v1` profile is non-root,
read-only, network-denied, socket/mount/device/capability-free,
no-new-privileges, and bounded by time, resources, PIDs, stdout and stderr.
Captured test output is explicitly untrusted and cannot become evidence or
approval by assertion. Static validation rejects unsafe Compose and mutable
dependencies before they can be run. The same conformance suite later applies
to both engines; an unavailable engine is inconclusive rather than a pass.

Supply-chain locks for the Go toolchain, modules, images, actions, parsers,
grammars, site tools, and scanners are verified before first use, not deferred
to a release gate.

The temporary shell bootstrap additionally requires the host utilities it
names explicitly (`git`, hashing/stat/path tools, bounded capture, and the OCI
CLI). A missing utility fails closed with an infrastructure diagnosis. These
locks must be decomposed into dedicated AUR-233 child cards; absence is never
reported as behavioral RED or OCI conformance.

Run `./.board/validate.sh` to validate board structure. The graph and work queue
are summarized in [`KANBAN.md`](KANBAN.md).
