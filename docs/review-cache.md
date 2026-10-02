# Review cache: identity, cross-file evidence, and degrade behavior (AUR-513)

This extends AUR-441's per-file review cache (`cmd/aurumcode/review_cache.go`,
`internal/review/cache`). See `docs/specs/AUR-441.md` for the original
design (one cache entry per changed file, keyed by that file's own raw diff
content plus the model identity and a prompt-version constant). This
document covers what AUR-513 adds: every other input that can change the
model's answer, how a per-file hit keeps cross-file evidence, and how a
broken cache degrades.

## What is now part of the cache key

`reviewContextCacheKey` (`cmd/aurumcode/review_cache.go`) folds in, on top of
AUR-441's original three (review language, codebase-context pack, memory
notes):

- **Selected reviewer profile(s)** (AUR-502): the joined
  `reviewprofile.Profile.Signature()` of every selected profile, in
  selection order. Order matters because `reviewprofile.MergeFindings`
  attributes a shared finding to the earliest profile in declaration
  order, so reordering the same set of names can change the answer.
  Before this card, `partitionByCache` was called with a key computed
  **before** `runProfilePasses` ever saw which profile(s) were chosen, so
  two reviews of the same diff under different profiles could wrongly
  share one cache entry.
- **The assembled context block's content** (repo prompt + skills + docs,
  and, under a central policy, the policy's own — AUR-518):
  `contextBlockCacheDigest` calls every configured `config.ContextProvider`
  directly and hashes their concatenated RAW output.
- **The dynamic rule/skill-section catalog** (AUR-519): `ruleCatalogCacheDigest`
  hashes the sorted rule-catalog ID list and the sorted `dynamicRules` map
  (each entry's full `review.Rule`, not just its ID) — this is what the
  model is taught and what the gate accepts citations against, and it can
  change independently of the raw context block (e.g. a skill file's
  parsed title/description changes even when unrelated prose around it
  does not).

Two reviews identical in every one of these may validly reuse a cache
entry; a difference in any one of them must produce a different key.

### Why the context digest is computed over RAW provider output, not the redacted block

`contextBlockCacheDigest` deliberately calls each provider's `Provide`
directly and hashes the concatenation of their raw text, instead of hashing
the already-redacted block `config.BuildContextBlockWithWarnings` returns
for the actual outbound prompt.

This was found by testing, not by inspection: an early version hashed the
redacted block, and a test that changed a secret-shaped marker string
between two rounds (`token:<value>`) kept reusing the stale cache entry,
because `redaction.Filter.Redact` replaces every secret-shaped span with
the **same fixed** `[REDACTED]` placeholder regardless of the span's real
value — two genuinely different files collapsed to the same digest. This is
exactly the collision `internal/review/cache/cache.Key`'s own doc already
identifies and avoids for a file's diff content (hashing it before
redaction, for the same reason). `contextBlockCacheDigest` now follows the
same rule. The result is still a one-way sha256 hex digest — never the
provider's text, raw or redacted — so AC-002's "no secret in legible form"
requirement holds exactly the way `cache.Key`'s own output already does.

### Why the context digest re-reads providers instead of reading the wrapped provider

`contextInjectingProvider` (`internal/config/wrap.go`), the type
`config.WrapProviderWithWarnings` returns, is unexported and exposes no way
for a caller outside `internal/config` to recover the block text it
carries. This card's `paths` are `cmd/aurumcode`,
`tests/acceptance/AUR-513.sh` and `docs/review-cache.md` — `internal/config`
is a `read_paths` dependency only, so it cannot be changed to add a seam.
`contextBlockCacheDigest` therefore calls
`config.ConfiguredProviders`' own `ContextProvider.Provide` a second time,
bounded by `config.ProviderTimeout` exactly like the real call
(`callProviderBounded`). Every provider this codebase has today
(`RepoPromptProvider`, `FileContextProvider`, `TextContextProvider`,
`PathInstructionsProvider`) reads one local file and does nothing else, so
a second call is deterministic and side-effect-free. A future stateful
provider (MCP/RAG, referenced but not yet implemented) would need this
revisited — most likely by memoizing one call's result for both the digest
and the real wrap within a single review, rather than calling twice.

## AC-003: a per-file hit must not drop cross-file evidence

**Chosen approach:** rely on the codebase-context pack already being
computed from the full, unpartitioned diff, before any per-file cache
lookup happens, and already being part of the cache key.

In `runReview` (`cmd/aurumcode/main.go`), `resolveCodebaseContext(diff)`
runs on the complete reviewed diff — every changed file, hit or miss — and
only afterward does `partitionByCache` reduce the diff sent to the model
down to the misses (`toSend`). `codebasectx.Resolver.Resolve`
(`internal/context/resolver.go`) reads every changed path's own content
from the checkout and extracts its defined symbols and cross-file
reference edges into `Pack.Symbols`/`Pack.References`/`Pack.Dependents`,
regardless of which files will turn out to be cache hits. That pack is
serialized into `codebaseContextText`, which is both (a) embedded verbatim
into the outbound prompt (`internal/prompt.builder.go`'s "## Codebase
context" section, via `review.ReviewContext.CodebaseContext`) and (b)
already one of the three original inputs to `reviewContextCacheKey`. So a
cache-hit file's symbols still reach the model reviewing a changed sibling
file, and a change to that cross-file picture still invalidates the right
entries.

Concretely: if `lib.go` defines `HelperZZZ` and is reviewed once alongside
`app.go` (which calls it), then on a later round where only `app.go`
changes again and `lib.go`'s own diff against the fixed base is
byte-identical to the cached round (a genuine cache hit, never resent),
`HelperZZZ` still appears in that round's prompt, because the codebase-
context pack was built from both files' paths before the partition ran.
`TestAUR513AC003CrossFileEvidenceSurvivesPartialHit`
(`cmd/aurumcode/aur513_test.go`) proves this end to end, via
`AURUMCODE_PROMPT_CAPTURE`.

**Alternative considered and rejected:** refuse to serve a cache hit for
any file the newly-changed file set might depend on. This needs a
dependency graph the cache package does not have, and building and
maintaining a reliable one (which files statically depend on which,
transitively, across languages) is a much larger undertaking than this
card's Outcome asks for, and risks the opposite failure — treating an
unrelated file as "depended on" and forcing it to be resent forever,
quietly defeating the cache's whole purpose. The codebase-context pack
already gives the model the cross-file picture it needs without that
graph, and it was already wired in before this card for exactly this
`AUR-515`/`AUR-536` reason; this card's job was to confirm, by test, that
nothing in the cache partitioning narrows what that pack sees, and to fold
it (plus the review's other new identity inputs) correctly into the key.

## AC-004: a broken cache degrades to a fresh review, never an approval

Two independent failure points, both pre-existing in AUR-441's design and
unchanged by this card except for being proven by test here:

- **A corrupted entry** (`cache.Cache.Get`, `internal/review/cache/cache.go`):
  a read or JSON-parse error returns `(nil, false, err)`. `partitionByCache`
  (`cmd/aurumcode/review_cache.go`) only ever treats a lookup as a hit when
  `getErr == nil && ok`; any other outcome — including a parse error on a
  truncated/garbled on-disk entry — puts that file back into the miss list,
  so it is reviewed fresh and its real finding(s) surface normally.
  `TestAUR513AC004CorruptCacheEntryDegradesToFreshReview` writes a garbled
  entry over a real cached one and proves the second round still reports
  the (now different) finding and never prints a reuse note for that file.
- **An unusable cache directory** (`cache.Open`,
  `internal/review/cache/cache.go`): a directory that cannot be created or
  used returns an error. `runReview` treats that exactly like "no cache
  configured for this run" — `toSend` stays the full, unpartitioned diff,
  and `persistFreshResults`/`mergeCacheHits` are skipped entirely (gated on
  `cacheErr == nil`). Caching is a best-effort optimization, never a
  correctness gate.
  `TestAUR513AC004UnreadableCacheDirDegradesToFreshReview` points
  `AURUMCODE_CACHE_DIR` at a path that already exists as a plain file (so
  `os.MkdirAll` fails) and proves the review still runs in full and still
  reports the real finding.

In both cases the gate-relevant guarantee already established for
AUR-441/AUR-519 holds: a cache failure can only ever cost a repeated model
call, never silently turn an inconclusive/blocked result into a pass, and
never omit a file from coverage.
