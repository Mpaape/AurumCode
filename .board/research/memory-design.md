# Memory research and minimum design

- Researched: 2026-08-01
- Status: design input for ADR-0001 and memory cards; not implementation proof
- Scope: repository context for AurumCode, not the separate operator/session
  memory used by development tools

## Sources examined

| Source | Useful property | What AurumCode does not adopt for the MVP |
|---|---|---|
| [Aider repository map](https://aider.chat/docs/repomap.html) | Extract a compact symbol/signature map, rank the dependency graph, and fit selected context into an explicit token budget. | A chat/editing-specific map format or an always-present whole-repo prompt. |
| [Hermes Agent persistent memory](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/memory.md) | Small curated memory, strict character bounds, explicit add/replace/remove, and a frozen per-session snapshot. | Automatic personal/user memory, prompt injection at every review, or allowing an agent to promote repository claims by itself. |
| [ai-memory](https://github.com/akitaonrails/ai-memory) | Project isolation, bounded sanitized observations, compiled durable pages, FTS5 without requiring an LLM, explicit provenance, handoffs, and optional vector search. Its prior-art section describes the “Karpathy LLM Wiki” as a compile-not-retrieve pattern. | A server, lifecycle hooks, raw session archive, git-versioned wiki, embeddings, cross-machine sync, or provider calls as a prerequisite for code review. |
| [OpenClaw built-in memory](https://github.com/openclaw/openclaw/blob/main/docs/concepts/memory-builtin.md) | One per-agent SQLite database, FTS5 keyword search, deterministic ranking, bounded WAL maintenance, and rebuild when configuration changes. | Default cloud embeddings, broad conversation memory, or a database outside the enabled repository. |
| [sqlite-memory](https://github.com/sqliteai/sqlite-memory) | Content hashing, transactional replacement, cleanup of deleted inputs, and restart-safe indexing in one SQLite store. | Embeddings, CRDT/cloud synchronization, storing duplicate source content, or external extensions in the baseline. |
| [SQLite FTS5](https://www.sqlite.org/fts5.html) | Embedded lexical retrieval with no service or network dependency. | Treating ranking as truth or authority. |

“Karpathy LLM Wiki” is useful prior-art terminology, not a published standard
or a repository-memory contract. AurumCode adopts its smallest defensible idea:
compile validated source facts into inspectable entities instead of retrieving
opaque raw agent history. The authoritative inputs remain Git blobs and current
policy.

## Findings

1. A repo map and agent memory solve different problems. A repo map is derived,
   snapshot-bound context; durable memory contains selected historical claims.
   Combining them in one opaque store makes invalidation and trust unclear.
2. Aider demonstrates that useful repository context does not require durable
   memory: changed symbols plus a graph-ranked, token-bounded live map is enough
   for the baseline review path.
3. Hermes demonstrates the value of hard bounds and explicit mutation, but its
   user-oriented Markdown memory is not an entity index and must not become an
   authorization channel.
4. The heavier systems add embeddings, servers, hooks, sync, and automatic
   consolidation. Those are inappropriate dependencies for an MVP Git review.
5. SQLite plus FTS5 is the simplest optional local store that supports atomic
   updates, lexical lookup, provenance, migrations, integrity checks, and safe
   deletion without a daemon.
6. Storing full source blobs duplicates Git, expands the sensitive-data surface,
   and increases repository impact. Hashes, entities, ranges, signatures,
   relations, and bounded summaries are sufficient; selected code is reread
   from the exact Git blob at review time.

## Selected model

```text
ChangeSet + exact Git blobs
          |
          v
LiveContextResolver ---------------> review (memory=off)
          |
          +--> ephemeral entity map -> review (memory=ephemeral)
          |
          +--> local SQLite/FTS5 ---> bounded optional augmentation
                 .git/aurumcode/memory.sqlite3
```

The review use case depends only on `LiveContextResolver`. `MemoryAugmenter` is
a decorator behind a port; deleting it does not change the domain or provider
contracts. Durable mode is opt-in and located under Git metadata so the
consumer worktree stays clean. No vector database or embedding provider is
part of the baseline.

The schema keeps two trust classes separate:

- **Derived entities:** rebuildable file/entity/relation facts bound to repo,
  commit, blob, parser, source range, and generator version.
- **Historical claims:** bounded inference/feedback records with evidence,
  confidence, validity interval, expiry, and explicit human approval when they
  may affect future behavior.

Retrieved values are data, not instructions. They cannot grant tools, change
rules, approve reviews, create suppressions, enable publication, or select a
more privileged provider.

## MVP acceptance consequences

- `memory=off` MUST execute review end to end without opening SQLite or writing
  any file, and it is the first shippable review path.
- `memory=ephemeral` MAY build the same entity schema in a temporary directory;
  cleanup is mandatory after success, failure, cancellation, and restart.
- `memory=local` MUST be explicit. Its default path, quotas, retention, file
  permissions, WAL bounds, status, rebuild, and guarded removal are testable.
- A small scripted-provider fixture MUST yield contract-equivalent findings and
  gate decisions in all modes. Context provenance may differ and is reported.
- Every mode MUST leave `git status --porcelain=v1` empty in the consumer repo.
- Corrupt, stale, oversized, or unavailable optional memory falls back to the
  live baseline only when trusted policy selects `fallback_off`; otherwise the
  run is explicitly incomplete. It never returns a false empty success.
- Entity-index incremental output MUST equal a clean rebuild for the same Git
  snapshot.
