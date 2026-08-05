# Memory research and minimum design

- Researched: 2026-08-04
- Status: design input for ADR-0001 and memory cards; not implementation proof
- Scope: repository context for AurumCode (code map plus optional local
  memory), not the separate operator/session memory used by development
  tools such as `ai-memory`

## Alternatives matrix

This is the criteria matrix AC-001 resolves every alternative cited in
`ADR-0001-optional-local-memory.md` against. A criterion is any column to the
right of `Date`. The ADR MUST NOT justify a decision against a criterion that
is not a column here, and it MUST cite every alternative by exactly the
`Alternative` value used in this table (case-sensitive). Every `Source` cell
carries the primary reference as `[label](url)`; the accept program checks the
`url` host against the allowlist in `tests/specs/AUR-020/allowed-source-domains.txt`
and never dereferences it.

| Alternative | Source | Version | Date | Indexing cost | Query scope | Network dependency |
|---|---|---|---|---|---|---|
| Aider | [Aider repository map](https://aider.chat/docs/repomap.html) | v0.86.0 | 2025-08-09 | Incremental, local: a changed-file scan through `ctags`/tree-sitter tags feeds a graph-ranked (PageRank-style) symbol table; cost scales with changed files, not repository size, and needs no embedding or index-build step. | A ranked, token-budgeted map of the most relevant symbol signatures across the whole repository, recomputed fresh for every prompt; there is no cross-session store to query later. | None. Parsing and ranking run in the local CLI process against files on disk. |
| Karpathy | [Andrej Karpathy, "context engineering" post](https://x.com/karpathy/status/1937902205765607626) | post 1937902205765607626 | 2025-06-25 | Not defined. The post frames the context window as an operating system's RAM and names the discipline of curating it, but specifies no indexing pipeline, storage format, or build cost. | Whatever an operator or harness manually assembles into the context window for one call; there is no queryable store, so "scope" is bounded only by what was hand-placed before that call. | None described; the framing concerns prompt/context assembly, not a service or index. |
| Hermes | [Hermes Agent memory guide](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/user-guide/features/memory.md) | v0.20.0 | 2026-08-03 | None. Memory is two flat Markdown files (`MEMORY.md`, capped near 2200 characters, and `USER.md`, capped near 1375 characters) hand-curated through explicit add/replace/remove actions; there is no parser, embedder, or background build. | The entire capped file is injected as one frozen snapshot into the system prompt at session start; retrieval is all-or-nothing per file, with no partial lookup or query language. | None for the built-in store; optional external providers (Mem0 and similar) are separate opt-in plugins outside the built-in path. |
| Tree-sitter | [tree-sitter/tree-sitter release v0.26.11](https://github.com/tree-sitter/tree-sitter/releases/tag/v0.26.11) | v0.26.11 | 2026-07-12 | Incremental parse per edit is near-linear in the size of the edit; it produces a concrete syntax tree held in memory, not a persisted index, so there is no durable indexing cost at all. | Structural queries (captures/predicates) are scoped to one parsed buffer or file; there is no built-in cross-file index and no cross-session persistence. | None. It is a local parsing library with no service dependency. |
| SCIP | [sourcegraph/scip release v0.9.0](https://github.com/sourcegraph/scip/releases/tag/v0.9.0) | v0.9.0 | 2024-06-29 | A dedicated per-language indexer performs a batch compile of the whole repository into one Protobuf index; this is heavier than a per-edit scan and is normally triggered by CI, not by every keystroke. | Precise, cross-file symbol definitions and references resolved from the compiled index; strong for "go to definition", but it requires a language-specific indexer and, in typical deployments, a serving component to query the index. | The indexer itself runs offline, but the common deployment uploads the index to a network service (e.g. Sourcegraph) for serving, which AurumCode's baseline does not adopt. |
| AurumCode baseline (SQLite FTS5 plus graph) | [SQLite FTS5 documentation](https://www.sqlite.org/fts5.html) | 3.53.3 | 2026-06-26 | Local, incremental: a changed snapshot updates only the touched file/entity/relation rows and their FTS5 shadow tables in one embedded database file; no daemon, batch recompile, or embedding call. | Lexical FTS5 search over entity/doc-summary text joined against a small relation graph (file, entity, relation tables) scoped to the enabled repository's `.git/aurumcode/memory.sqlite3`; queries stay local to that one database. | None. SQLite is an embedded library; the database is opened in-process with no server or outbound call. |

## Findings

1. A repo map and agent memory solve different problems. A repo map is
   derived, snapshot-bound context (Aider, Tree-sitter, SCIP); durable memory
   contains selected historical claims (Hermes). Karpathy's framing names the
   distinction ("what stays resident, what gets retrieved, what gets
   compressed") without specifying a mechanism for either. Combining derived
   and historical data in one opaque store makes invalidation and trust
   unclear, so AurumCode keeps them as two labeled trust classes (below).
2. Aider demonstrates that useful repository context does not require durable
   memory: a changed-symbol, graph-ranked, token-bounded live map is enough
   for the baseline review path, at a cost that scales with the change, not
   the repository.
3. Karpathy's "context engineering" framing is a useful vocabulary for what
   AurumCode's `LiveContextResolver` and `MemoryAugmenter` are for (filling a
   bounded context window with the right facts), but it is not itself an
   indexable, versionable design: it names no schema, no storage, and no query
   scope, so it cannot be the baseline and is rejected as an implementation.
4. Hermes demonstrates the value of hard character bounds and explicit,
   auditable mutation (add/replace/remove), but its memory is one hand-curated
   Markdown blob per concern, not an entity index; adopting it verbatim would
   remove the ability to query, join, or bound memory by entity, and it must
   never become an authorization channel.
5. Tree-sitter is the correct incremental *parser* underneath a repository
   map (Aider already uses it), but it is not itself a memory or query
   system: it holds one buffer's syntax tree in memory and persists nothing.
   AurumCode's baseline may use a tree-sitter-class parser as a future entity
   extractor, but Tree-sitter alone answers no cross-file question.
6. SCIP produces the most precise cross-file symbol graph of the five, but
   its indexing cost (a batch, per-language, CI-grade compile) and its usual
   network-serving deployment are disproportionate to an MVP that must run
   `memory=off` with zero index and `memory=local` inside one consumer
   repository with no external service. AurumCode's own relation graph is a
   deliberately smaller, embedded echo of SCIP's idea (symbols and
   relations, not free text) without the indexer-per-language or serving
   dependency.
7. SQLite plus FTS5 is the simplest optional local store that supports
   atomic updates, lexical lookup, provenance, migrations, integrity checks,
   and safe deletion without a daemon; adding a small relation table set
   turns it from pure lexical search into the symbol/relation graph that
   Aider and SCIP show is necessary for ranked, structural context.
8. Storing full source blobs duplicates Git, expands the sensitive-data
   surface, and increases repository impact. Hashes, entities, ranges,
   signatures, relations, and bounded summaries are sufficient; selected code
   is reread from the exact Git blob at review time.

## Supporting baseline sources (not alternatives under comparison)

These informed the shape of the SQLite FTS5 plus graph baseline but are not
independent alternatives being rejected; they describe the same class of
design AurumCode adopts and are not scored against the matrix above.

| Source | Relevant property |
|---|---|
| [ai-memory](https://github.com/akitaonrails/ai-memory) | Project isolation, bounded sanitized observations, compiled durable pages, FTS5 without requiring an LLM, explicit provenance, optional vector search kept behind a port. Its prior-art section names the "Karpathy LLM Wiki" compile-not-retrieve pattern that finding 3 above builds on. |
| [OpenClaw built-in memory](https://github.com/openclaw/openclaw/blob/main/docs/concepts/memory-builtin.md) | One per-agent SQLite database, FTS5 keyword search, deterministic ranking, bounded WAL maintenance, and rebuild-on-config-change, all without a network dependency. |
| [sqlite-memory](https://github.com/sqliteai/sqlite-memory) | Content hashing, transactional replacement, cleanup of deleted inputs, and restart-safe indexing in one SQLite store, without embeddings or cloud sync in its baseline mode. |

"Karpathy LLM Wiki" is useful prior-art terminology, not a published standard
or a repository-memory contract. AurumCode adopts its smallest defensible
idea: compile validated source facts into inspectable entities instead of
retrieving opaque raw agent history. The authoritative inputs remain Git
blobs and current policy, never the Karpathy post itself, which is cited only
as the origin of the vocabulary.

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
               plus relation graph
                 .git/aurumcode/memory.sqlite3
```

The review use case depends only on `LiveContextResolver`. `MemoryAugmenter` is
a decorator behind a port; deleting it does not change the domain or provider
contracts. Durable mode is opt-in and located under Git metadata so the
consumer worktree stays clean. There is no vector database, embedding
service, daemon, or network dependency in the baseline; the "AurumCode
baseline" row above is the only row in the matrix with a `Network dependency`
value of exactly `None.` combined with a local, incremental `Indexing cost` —
which is the pairing the Decision section of the ADR names as decisive.

The schema keeps two trust classes separate:

- **Derived entities:** rebuildable file/entity/relation facts bound to repo,
  commit, blob, parser, source range, and generator version. This is
  AurumCode's small, embedded echo of what SCIP indexes at much larger scale.
- **Historical claims:** bounded inference/feedback records with evidence,
  confidence, validity interval, expiry, and explicit human approval when they
  may affect future behavior. This is the only place a Hermes-style curated
  note is allowed to live, and it is schema-bound rather than free text.

Retrieved values are data, not instructions. They cannot grant tools, change
rules, approve reviews, create suppressions, enable publication, or select a
more privileged provider.

## MVP acceptance consequences

- `memory=off` MUST execute review end to end without opening SQLite or writing
  any file, and it is the first shippable review path.
- `memory=ephemeral` MAY build the same entity schema in a temporary directory;
  cleanup is mandatory after success, failure, cancellation, and restart.
- `memory=local` MUST be explicit. Its default path, quotas, retention, file
  permissions, WAL bounds, status, rebuild, and guarded removal are testable
  against the limits declared in `standards/memory`.
- A small scripted-provider fixture MUST yield contract-equivalent findings and
  gate decisions in all modes. Context provenance may differ and is reported.
- Every mode MUST leave `git status --porcelain=v1` empty in the consumer repo.
- Corrupt, stale, oversized, or unavailable optional memory falls back to the
  live baseline only when trusted policy selects `fallback_off`; otherwise the
  run is explicitly incomplete. It never returns a false empty success.
- Entity-index incremental output MUST equal a clean rebuild for the same Git
  snapshot.
