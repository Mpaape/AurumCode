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

The baseline implementation is one SQLite database with FTS5 plus a small
relation graph (file, entity, and relation tables) built on top of it. There
is no vector database, embedding service, daemon, or network dependency. It
stores:

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

## Alternatives considered

This section resolves every alternative named below to exactly one row of the
criteria matrix in `.board/research/memory-design.md`. Every justification
line below cites a criterion by the exact column name used in that matrix
(`Indexing cost`, `Query scope`, or `Network dependency`); no other criterion
name is used here, and none of the four rejected alternatives is a typed
status, boundary sentinel, `invalid`, or any other implementation control
value — each is a design compared and rejected on its own evidence.

### Rejected: Aider

Aider's repository map (see the matrix) is the strongest evidence that a
live, token-bounded map is enough for review, but it is re-derived every
call and keeps nothing across sessions, so it cannot alone satisfy
`memory=local`'s opt-in incremental entity map.

- Criterion: Query scope — Aider's map is rebuilt fresh for every prompt with
  no cross-session store; AurumCode's `local` mode instead needs a queryable
  store that survives between runs, which the matrix shows only the baseline
  row provides.
- Criterion: Indexing cost — Aider's changed-file, no-embedding scan is the
  right cost shape (scales with the change, not the repository) and is the
  model AurumCode's own incremental rebuild follows; this is why Aider is
  cited as the design precedent for cost even though it is rejected as the
  storage layer.
- Criterion: Network dependency — Aider has none, matching the baseline; this
  criterion does not distinguish Aider from the decision and is not the
  reason it is rejected.

### Rejected: Karpathy

- Criterion: Indexing cost — the "context engineering" framing specifies no
  indexing pipeline at all, so it cannot be evaluated as an indexing design
  and cannot satisfy the bounded, deterministic rebuild that `standards/memory`
  requires.
- Criterion: Query scope — there is no queryable store, only whatever an
  operator manually places in the context window for one call; AurumCode
  needs a store `MemoryAugmenter` can query the same way on every run.
- Criterion: Network dependency — none is described, which matches the
  baseline, but an undefined mechanism cannot be adopted regardless of this
  criterion.

### Rejected: Hermes

- Criterion: Query scope — Hermes injects one frozen, capped Markdown file
  per concern with no partial lookup; AurumCode needs to query individual
  entities and relations, not an entire file at once.
- Criterion: Indexing cost — Hermes has none because it never parses source;
  that is only acceptable for hand-authored user/agent notes, so AurumCode
  restricts Hermes-shaped free text to the separate "historical claims" trust
  class in `memory-design.md`, never to derived entity facts.
- Criterion: Network dependency — Hermes's built-in store has none, matching
  the baseline; its optional external memory providers do add one, and
  AurumCode does not adopt any of them in the baseline.

### Rejected: Tree-sitter

- Criterion: Indexing cost — Tree-sitter's near-linear incremental parse per
  edit is exactly the parsing cost shape AurumCode's future entity extractor
  should use, but Tree-sitter alone persists nothing between calls, so cost
  alone cannot make it a memory design.
- Criterion: Query scope — a parsed syntax tree is scoped to one buffer or
  file with no cross-file index; AurumCode's `local` mode needs cross-file
  relation queries, which Tree-sitter does not provide by itself.
- Criterion: Network dependency — Tree-sitter has none, matching the
  baseline; this criterion does not distinguish it from the decision either.

### Rejected: SCIP

- Criterion: Indexing cost — SCIP's batch, per-language, CI-grade compile of
  the whole repository is disproportionate to an MVP that must also run
  `memory=off` with zero index and a small, incremental `memory=local`
  rebuild inside one consumer repository.
- Criterion: Query scope — SCIP's precise cross-file symbol graph is the
  closest of the five to what AurumCode's relation graph now echoes at much
  smaller scale (symbols and relations, not full source), which is why the
  baseline's graph tables are modeled after SCIP's shape rather than FTS5
  text search alone.
- Criterion: Network dependency — SCIP's indexer itself is local, but its
  typical deployment uploads the index to a network serving component; the
  AurumCode baseline never does this, so a SCIP-shaped serving dependency is
  rejected even though the indexer step is not.

### Decision: SQLite FTS5 plus graph

AurumCode's baseline closes on SQLite FTS5 plus a small relation graph,
justified by the same three criteria against the matrix's baseline row:

- Criterion: Indexing cost — local and incremental, touching only the
  changed snapshot's file/entity/relation rows and their FTS5 shadow tables,
  matching the cost shape Aider and Tree-sitter demonstrate is sufficient,
  without SCIP's batch per-language compile or Hermes's total absence of
  structure.
- Criterion: Query scope — lexical FTS5 search joined against the relation
  graph gives cross-file, entity-scoped queries that persist across runs,
  which none of Aider, Karpathy, or Hermes provide, at a fraction of SCIP's
  indexing and serving cost.
- Criterion: Network dependency — none. The database is an embedded library
  opened in-process against a file under the enabled repository's Git
  metadata; there is no vector database, embedding service, daemon, or
  external serving component in the baseline, which is the one property the
  baseline shares with every alternative except SCIP's typical deployment.

## Consequences

The MVP can ship useful provider-agnostic review before durable memory. Local
memory later improves context selection and incremental cost without creating a
hosted data store. Semantic embeddings remain a future adapter behind the
retrieval port and are not part of the baseline or its public claims.

The demo repository MUST exercise all three modes, confirm `git status` remains
clean, enforce size bounds, interrupt SQLite writes, rebuild from Git, remove
state safely, and prove that disabling memory makes no memory-related syscall or
artifact.
