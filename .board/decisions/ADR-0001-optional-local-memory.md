# ADR-0001: Optional repository-local memory

- Status: accepted product constraint; implementation pending board cards
- Decision owner: project owner
- Scope: review, documentation, test generation, QA, agents, and MCP

## Context

Repository memory is useful for incremental entity mapping, but it cannot be a
precondition for useful code review or create operational weight in consumer
repositories. The target repository may be small, read-only, ephemeral, private,
or unwilling to retain derived information.

## Decision

AurumCode exposes three explicit modes:

| Mode | Persistence | Intended use |
|---|---|---|
| `off` | None | Minimum MVP review from the change set and bounded live context |
| `ephemeral` | Temporary database removed after the run | Rich context without retained repository data |
| `local` | Small database under the consumer repository's Git metadata | Opt-in incremental entity map across runs |

`off` is the default until the local-memory security, size, cleanup, and
equivalence gates are approved. Enabling memory MUST NOT enable publication,
model access, tools, or any other capability.

The default durable path is `.git/aurumcode/memory.sqlite3`, not the worktree.
This keeps memory local to the enabled repository without dirtying `git status`
or requiring a committed `.gitignore`. Worktrees resolve the owning common Git
directory deliberately; bare repositories use their Git directory. A caller MAY
provide an explicit state path through trusted base configuration or a CLI flag.
Repository-head configuration MUST NOT redirect it.

The baseline implementation is one SQLite database with FTS5. There is no
vector database, embedding service, daemon, or network dependency. It stores:

- snapshot, file, entity, relation, content-hash, and tombstone records;
- bounded identifiers, paths, ranges, signatures, doc summaries, provenance,
  confidence, and schema/generator versions;
- approved bounded project knowledge only when separately authorized.

It does not duplicate full source blobs, persist raw prompts/responses or
chain-of-thought, copy credentials, or treat remembered prose as authority.
Source context is reread from the exact validated Git blob when needed.

## Required invariants

1. `memory=off` performs a complete review workflow and never opens, creates,
   migrates, probes, or locks a memory database.
2. `ephemeral` leaves no state after success, failure, cancellation, or crash
   recovery/cleanup.
3. `local` creates only the configured database and its transient SQLite files
   under the resolved state directory, with restrictive permissions.
4. Every run leaves the consumer worktree and Git index byte-for-byte clean.
5. The memory augmenter always returns the unchanged stateless baseline,
   optional bounded augmentation, and a typed status (`disabled`, `ready`,
   `unavailable`, `invalid`, `over_budget`, or `partial`). It never chooses the
   run outcome. Only the trusted composition root applies the versioned policy:
   explicit `fallback_off` discards augmentation and records degraded status;
   `fail` terminates the run as incomplete/failed. There is no silent fallback,
   no policy inside the store/augmenter, and no false successful review.
6. Entity facts bind to repository identity, commit/blob hash, parser version,
   and source range. Inferences remain labeled and cannot change policy.
7. Incremental output is equivalent to a clean rebuild for the same snapshot.
8. Size, entity count, retained snapshots, snippet length, and execution time
   have hard configurable bounds. Reaching a bound triggers deterministic GC or
   an explicit status, never uncontrolled growth.
9. `aurumcode memory status`, `rebuild`, and `remove` are local, deterministic,
   scriptable, and proposal-free. `remove` resolves and displays the exact safe
   target and cannot operate on the repository, worktree root, home, or `/`.
10. The same small demo change produces contract-equivalent review artifacts in
    `off`, `ephemeral`, and `local` modes with scripted providers; differences
    in optional context provenance are normalized and explained.

## Consequences

The MVP can ship useful provider-agnostic review before durable memory. Local
memory later improves context selection and incremental cost without creating a
hosted data store. Semantic embeddings remain a future adapter behind the
retrieval port and are not part of the baseline or its public claims.

The demo repository MUST exercise all three modes, confirm `git status` remains
clean, enforce size bounds, interrupt SQLite writes, rebuild from Git, remove
state safely, and prove that disabling memory makes no memory-related syscall or
artifact.
