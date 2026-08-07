# AurumCode Reconstruction Board

This directory is the executable source of truth for the reconstruction. It
maps work as an atomic dependency graph; it is deliberately not a calendar or
a human-effort estimate.

The pre-existing `.taskmaster/` backlog is preserved as audit input only. Its
historical `done` values do not prove that a capability exists and never drive
the execution queue; AUR-001 and AUR-002 characterize those claims explicitly.

## Operating model (lightweight delivery, 2026-08)

The board runs on a simple, parallel delivery cycle:

1. A card moves from `backlog` to `ready` only when every `depends_on` card is
   in `done`. `ready` authorizes the driver, not a builder.
2. The developer executes the card, commits to a human-authenticated commit
   (never AI attribution), and adds a `## Delivery record` section to the card
   body with `- commit: <40-hex sha>`.
3. The card moves to `review`. An independent reviewer agent code-reviews the
   named commit against the card's own outcome criteria and, on approval,
   appends `- review: approved` to the Delivery record.
4. If the card frontmatter declares `validation: tested` or `skeptical`, a
   validator agent executes the card's own acceptance and appends
   `- validation: passed` on success. Cards without a `validation:` field
   default to `none` and skip this step.
5. The card moves to `done` only when its Delivery record is complete for its
   declared validation kind. The lightweight gate is `bash .board/pipeline.sh`
   — run it and read its exit code.
6. Only the coordinator moves cards and integrates. Commits never carry AI
   authorship, co-authorship, signatures, or generated-by attribution.

The earlier ceremony (two blind reviewers, skeptical mutation, OCI evidence
bundles) is preserved as history below and still governs the 12 legacy `done`
cards; new cards run the lightweight cycle above.

## Legacy ceremony (pre-2026-08; governs legacy `done` cards only)

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
- `cards/cancelled`: a card is never deleted. Work that is no longer wanted
  moves here instead, and only from evidence, never a rename. A card may sit
  in `cards/cancelled` only when `.board/evidence/<id>/cancellation.json`
  exists and validates: `schema: aurum.cancellation`, `version: 1`, the
  card's own id, `approved_by_role: "manager"` (cancellation is a management
  decision, never an agent's), a `reason` of at least 40 characters that is
  neither generic boilerplate nor a bare filler phrase such as "not needed"
  or "obsolete" standing alone, a `superseded_by` field naming the
  replacement card or JSON `null`, and a `card_digest` that is the same
  self-referential SHA-256 recomputation `spec_digest` already uses (the
  card's own frontmatter/body with `status` and `spec_digest` normalized),
  binding the cancellation decision to the exact cancelled text. `base_sha`
  and `spec_digest` are locked the same way as `doing`/`review`/`done`: a
  cancelled card is a frozen snapshot, not an unblocked placeholder. A
  cancelled card can never carry `.board/evidence/<id>/manifest.json` --
  cancellation is not completion, and the `done` evidence bundle asserts the
  opposite claim. Any other card in an active state (`backlog`, `ready`,
  `doing`, `review`, `done`) that lists a cancelled card in `depends_on` fails
  validation unless the cancelled card declares a non-null `superseded_by`
  **and** the dependent card also depends on that successor -- cancelling a
  card can never silently orphan the dependency graph.
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
  directionally within `paths`, naming an artifact one of the two second-reader
  engines can actually execute (`*_test.go` or an acceptance `*.sh`); an
  optional `read_paths` field separately allowlists read-only inputs without
  granting write ownership;
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

## The second reader

`oci-run` executes exactly one program — the card's own acceptance script —
inside a pinned image that carries no language toolchain, with only that card's
`paths`/`read_paths` materialized. Two consequences were reproduced live on
three different cards: a `## TDD proof` line citing
`tests/integration/AUR-0NN_test.go::TestX` was never executed by any gate, and a
defect whose evidence lives outside the card's own `paths` is invisible to the
only engine the gate ran. Every card that claimed "two independent engines
agree" was decided by one.

[`bin/second-reader`](bin/second-reader) is the other engine. It executes the
concrete `Unit`/`Contract`/`Integration`/`E2E` citations of a card **outside**
the acceptance sandbox — same reason the repository build already runs outside
it — against the **whole** repository mounted read-only, through a
digest-pinned image chosen by the citation: `golang` for `*_test.go`, `bash` for
an acceptance program. Both runs are network-denied, non-root, read-only
rootfs, all capabilities dropped, no-new-privileges, and bounded; the Go engine
additionally needs an exec-capable scratch tmpfs, because a Go test is a
compiled binary, and that single deviation is stated rather than hidden. Neither
engine can write one byte into the repository.

The runner emits observations only. `validate.sh` re-derives every fact from the
raw captured bytes and ignores the summary when the two disagree:

- the exact command, rebuilt from the card and required to appear verbatim in
  the log — an observation can never have run a selector the card did not
  declare;
- the sha256 of the test file that was read, recomputed from the tree — editing
  the test expires the record, so a pasted or stale run cannot cover new bytes;
- the framed exit code, the match count, and the refusal of a selector that
  matched nothing. `go test -run` that matches no test exits **zero**; that is
  success without work and it is refused explicitly. A shell citation is proved
  to select something by handing the program a selector it cannot know and
  requiring exit `64`;
- capture regions are line-prefixed and their boundary frames must be unique, so
  a program that prints something frame-shaped cannot forge one;
- the image the run used, compared against
  `locks/oci/second-reader-<engine>-v1.lock.json`. `=== image:` is written by the
  runner, so on its own it is a claim; recomputing it from the lock is what makes
  it a binding, and a record produced in some other image is refused;
- that the cited file **defines** the cited selector — `^func <Test…>(` for Go,
  the literal for a shell dispatcher. The Go command is package-scoped, so
  without this an owned `_test.go` containing no test at all could cite a
  neighbouring card's selector and produce a green log in which every other rule
  held.

A `done` card must carry, inside its sealed bundle and inside
`evidence_hashes`, one `aurum.second-reader-observation` per concrete citation,
bound to the same `CandidateIdentityV1` as the reviews and the acceptance run
and sealed under its own role and nonce.

`review` is the first state in which a cited layer test must exist, be owned,
define its selector, and be executable by one of the two engines. `doing` is
deliberately exempt: the acceptance `Test:` is sealed by an isolated test
designer *before* a builder starts, while the layer tests are the builder's own
output.

### Every card, or a named exemption

The gate is reachable only through a concrete citation, so a card whose four
layers all read `not-applicable: …` used to skip it in silence. It cannot any
more: a card entering `review` or `done` must either hand the second reader
something it can execute, or be named — with its reason — in
[`second-reader-exempt.tsv`](second-reader-exempt.tsv), which carries the same
ratchet as the legacy registry below and is printed on every run as
`board note:`. A `review` card can never be exempted, so the list can only
shrink.

- `.board/second-reader-exempt.tsv` records 2 entry(ies) across 2 card(s).

### Re-execution, and the third verdict

`validate.sh` re-runs the second reader through `bin/second-reader --verify` by
default (`AURUM_SECOND_READER=exec`); `recompute` asks for the structural and
raw-log recomputation alone. Those are the only two values; anything else falls
back to `exec`, the stricter one.

Neither mode can turn "I could not check" into "checked". Every layer that is
not re-executed — no OCI engine on the host, or `recompute` — is named on stderr
as it is skipped, counted, and summarized, and the run ends **`board
inconclusive:` with exit `3`**: not `board valid`, not `board invalid`. Law 4 and
law 11, made observable. A transition to `done` is authorized only by a run that
exited `0`, which is exactly the run in which every recorded layer was executed
again.

### Legacy registry

Cards that reached `done` before this runner existed cite concrete tests that no
gate had executed. All of them have now been executed and their raw logs live in
`tests/second-reader-legacy/`; `validate.sh` recomputes exactly the same facts
from them. The one binding a legacy record cannot carry is the card's sealed
`CandidateIdentityV1`, because reopening a `done` bundle is a coordinator act
performed by three independent roles, and a builder forging those digests would
be manufacturing the self-declared evidence this board exists to refuse.

- `.board/second-reader-legacy.tsv` records 14 entry(ies) across 6 card(s), 6 distinct captured program(s).

Those two numbers are derived from the file and the recorded logs by
`validate.sh`, which requires this line and the registry header to say the same
thing; three texts once stated three different sizes for this one list. The
second number is the honest one: `tests/acceptance/AUR-359.sh` and its three
siblings dispatch four selector names onto one body, so the
Contract/Integration/E2E records of those cards differ only in the selector they
echo. **A citation is not an execution.** 14 citations are 6 distinct captured
programs, and closing that gap is work on `tests/`, not on the registry.

`validate.sh` prints every entry on every run as `board note:` — the migration is
never silent — and refuses an entry for a card that is not in `done`, for a layer
the card no longer cites, for a card/layer that already carries a sealed
observation, for a log under `tests/second-reader-legacy/` that no entry names,
and for any key outside the frozen cutover set compiled into `validate.sh`. That
last rule is what stops a card from joining the list in the same commit that
moves it to `done`: growing either registry is a reviewed change to the
validator, never a data edit.

The temporary shell bootstrap additionally requires the host utilities it
names explicitly (`git`, hashing/stat/path tools, bounded capture, and the OCI
CLI). A missing utility fails closed with an infrastructure diagnosis. These
locks must be decomposed into dedicated AUR-233 child cards; absence is never
reported as behavioral RED or OCI conformance.

Run `./.board/validate.sh` to validate board structure, and
`bash .board/tests/validator-mutants.sh` to validate the validator. Each
second-reader rule in that suite comes in a pair: the mutant must die with the
rule in place and the identical tree must survive with that one rule surgically
removed, because a mutant that dies either way was proving some other check.
The graph and work queue are summarized in [`KANBAN.md`](KANBAN.md).
