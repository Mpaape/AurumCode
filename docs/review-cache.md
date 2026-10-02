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

- **The real answering model/endpoint, captured before any context
  wrapping** (`baseModelIdentity`): `modelCacheKey(provider)` called on
  `provider` the instant it is selected, BEFORE `config.WrapProviderWithWarnings`
  ever gets a chance to wrap it. This is not redundant with the
  `modelCacheKey(provider)` call `reviewContextCacheKey` still also makes on
  the (by then, possibly wrapped and possibly `--limite`-wrapped) final
  provider — see "Why the model identity is captured twice" below for the
  bug this closes.
- **Selected reviewer profile(s)** (AUR-502): the joined
  `reviewprofile.Profile.Signature()` of every selected profile, in
  selection order. Order matters because `reviewprofile.MergeFindings`
  attributes a shared finding to the earliest profile in declaration
  order, so reordering the same set of names can change the answer.
  Before this card, `partitionByCache` was called with a key computed
  **before** `runProfilePasses` ever saw which profile(s) were chosen, so
  two reviews of the same diff under different profiles could wrongly
  share one cache entry.
- **The assembled, REDACTED context block's content** (repo prompt +
  skills + docs, and, under a central policy, the policy's own —
  AUR-518): `contextBlockCacheDigest` hashes the same block
  `config.BuildContextBlockWithWarnings` builds for the real outbound
  prompt. See "Why the context digest hashes the redacted block" below.
- **The dynamic rule/skill-section catalog** (AUR-519): `ruleCatalogCacheDigest`
  hashes the sorted rule-catalog ID list and the sorted `dynamicRules` map
  (each entry's full `review.Rule`, not just its ID) — this is what the
  model is taught and what the gate accepts citations against. See
  "ruleCatalogDigest: defense in depth" below for its current test status.

Two reviews identical in every one of these may validly reuse a cache
entry; a difference in any one of them must produce a different key.

### Why the model identity is captured twice

`config.contextInjectingProvider` (`internal/config/wrap.go`, the type
`WrapProviderWithWarnings` returns the instant ANY context is configured —
a repo prompt, a skill, a doc) embeds `llm.Provider` as an **interface**
field. Go only promotes methods declared on the embedded field's own
*static* type, so a method the underlying concrete provider also happens to
implement, but that is not part of the `llm.Provider` interface itself, is
not promoted through the wrapper. `llm.ModelResolver` (`ResolveModel`) is
exactly such a method: `internal/llm/provider/litellm.Provider` implements
it (reporting the real configured model), but once wrapped, a type
assertion `provider.(llm.ModelResolver)` on the wrapped value fails, and
`modelCacheKey` silently falls back to `litellm.Provider.Name()`'s fixed
`"litellm"` literal — no model, no endpoint.

Before this fix: with ANY docs/skills/prompt configured (common), two
reviews against the same endpoint under two **different** `LLM_MODEL`
values collided on one cache entry, because the only thing distinguishing
them (`ResolveModel`'s reported model) had become invisible. `cmd/aurumcode/cost.go`'s
own `fixedModelProvider` documents this exact promotion-loss hazard for a
different wrapper and works around it by implementing `ResolveModel`
directly on itself — but that only fixes `--limite`'s own wrapper, not the
context wrapper underneath it, and only when `--limite` is actually used.

The fix: `runReview` (`cmd/aurumcode/main.go`) calls `modelCacheKey(provider)`
immediately after provider selection and before any wrapping, captures it
as `baseModelIdentity`, and passes that string into `reviewContextCacheKey`
unconditionally. The pre-existing `modelCacheKey(provider)` call inside
`reviewContextCacheKey` itself is kept, unchanged, because `--limite`'s
`fixedModelProvider` (wrapped later, on top of the context wrapper, and
implementing `ResolveModel` directly — not relying on promotion) still
needs that late-captured value for cost-key accounting exactly as before
AUR-513; removing it would regress `tests/acceptance/AUR-433.sh`. The two
values are complementary, not alternatives.

`modelCacheKey` also now folds in `LLM_BASE_URL` directly (two endpoints
serving a model under the same name are not the same answering entity).
`TestAUR513ModelIdentitySurvivesContextWrapping` (`cmd/aurumcode/aur513_test.go`)
proves both halves: a unit check that two `litellm.Provider`s with
different models AND base URLs, wrapped by the identical context, still
produce different review-cache keys (and a sanity assertion that their
**wrapped** `modelCacheKey` alone collides, demonstrating the bug this
closes); and an end-to-end run against a real `httptest.Server` with a
request counter, where switching `LLM_MODEL` between two rounds against a
persisted cache forces a real second request.

### Why the context digest hashes the redacted block

`contextBlockCacheDigest` hashes the block `config.BuildContextBlockWithWarnings`
assembles — the SAME, already-redacted text the real call sends to the
model — rather than each provider's raw, pre-redaction output.

An earlier revision of this function hashed raw provider output
specifically to avoid a collision: `redaction.Filter.Redact` replaces every
secret-shaped span with the same fixed `[REDACTED]` placeholder, so two
files differing only in a secret's value can redact to byte-identical text,
and a test that changed only a secret-shaped marker between rounds proved
exactly that — the digest didn't move, because the text it was computed
over didn't move either, once the Model identity and the raw-text approach
disagreed with what the model actually receives.

That collision is real, but hashing the raw text to dodge it is answering a
different question than "should this count as a cache hit": the model only
ever sees the REDACTED prompt. If two configurations produce the identical
redacted block, the model would answer identically either way, and reusing
the cached answer is correct — not a weakness, just the same thing the
model itself already cannot distinguish. Hashing raw provider text instead
would make the cache MORE conservative than the model it is caching for,
forcing needless re-reviews for a difference the model never sees, while
still not changing the actual secret-exposure story at all (the digest, raw
or redacted, is a one-way sha256 hex string either way — `cache.Key`'s own
established pattern for a file's diff content — so "no secret in legible
form" holds in both versions).

`TestAUR513AC002DocContentChangeForcesFreshReview` therefore changes a
**plainly-visible** line (ordinary prose, not secret-shaped) between
rounds, so the test actually exercises "the configured text reaches the
digest" rather than a redaction collision either version would behave
identically under.

This still calls `config.BuildContextBlockWithWarnings` a second time
rather than reading the already-wrapped provider's internals, because
`contextInjectingProvider` is unexported and exposes no seam for a caller
outside `internal/config` to recover its block text from, and this card's
`paths` do not include `internal/config`. Every provider
`ConfiguredProviders` returns today (`RepoPromptProvider`,
`FileContextProvider`, `TextContextProvider`, `PathInstructionsProvider`)
reads one local file and does nothing else, so a second call is
deterministic and side-effect-free; a future stateful provider (MCP/RAG,
referenced but not yet implemented) would need this revisited — most
likely by memoizing one call's result for both the digest and the real
wrap within a single review, rather than building the block twice.

### ruleCatalogDigest: defense in depth, not independently exercised end to end

Today every skill file `dynamicRulesFromLocalSkills` reads
(`review.ParseSkillSections`) is drawn from the exact same files
`contextBlockCacheDigest`'s block already hashes, so in practice a
skill-content change that alters the derived rule catalog also changes the
context-block digest, and this card's tests invalidate the cache through
that shared path rather than isolating this one directly. `ruleCatalogDigest`
is kept anyway, independently folded into the key, because the two can
diverge without warning in the future: a catalog entry derived with
normalization the block's raw bytes do not reflect, a central-policy-only
rule source, or any rule contribution that is not simply "the bytes of a
context file" would change what the model is taught and what the gate
accepts without necessarily changing the context block itself. This is a
documented gap, not a claim of end-to-end proof for this one component in
isolation.

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
into the outbound prompt (`internal/prompt/builder.go`'s "## Codebase
context" section, via `review.ReviewContext.CodebaseContext`) and (b)
already one of `reviewContextCacheKey`'s inputs. So a cache-hit file's
symbols still reach the model reviewing a changed sibling file, and a
change to that cross-file picture still invalidates the right entries.

Concretely: if `lib.go` defines `HelperZZZ` and is reviewed once alongside
`app.go` (which calls it), then on a later round where only `app.go`
changes again and `lib.go`'s own diff against the fixed base is
byte-identical to the cached round (a genuine cache hit, never resent),
`HelperZZZ` still appears in that round's codebase-context pack, because
the pack was built from both files' paths before the partition ran.
`TestAUR513AC003CrossFileEvidenceSurvivesPartialHit`
(`cmd/aurumcode/aur513_test.go`) proves this end to end, via
`AURUMCODE_PROMPT_CAPTURE`: it specifically parses the captured prompt's
own `## Codebase context` JSON section and asserts `HelperZZZ` is in its
`Symbols` field — NOT merely a substring check against the whole prompt,
because `app.go`'s own diff literally contains the text `HelperZZZ()` at
its call site, which would make a bare substring check pass even with an
empty codebase-context pack. `tests/acceptance/AUR-513.sh`'s
`AC-003-MUT-001` selector mutates the `reviewCtx` construction to resolve
the codebase context from `toSend` (the already-partitioned, miss-only
diff) instead of the full `diff`, reproducing the regression this design
exists to refuse, and confirms the test goes RED for it.

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
  entry over a real cached one and proves the second round, under the
  **same** `AURUMCODE_LLM_FIXTURE` as round 1 (deliberately: changing the
  fixture between rounds would also change `modelCacheKey` and force a
  fresh key on its own, proving nothing about the corrupted-entry path
  specifically), never prints a reuse note and actually (re)writes an
  `AURUMCODE_PROMPT_CAPTURE` file naming `app.go`. `AC-004-MUT-001`
  (`tests/acceptance/AUR-513.sh`) mutates `partitionByCache` to treat any
  `Get` error as a hit with zero issues and confirms this test goes RED for
  it.
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

## Known gap: `cache.PromptVersion` stays a hand-bumped constant

`internal/review/cache.PromptVersion` ("v1") is a manually-bumped lever,
not a digest of the actual built-in prompt template — and that package
(`internal/review/cache`) is outside this card's `paths` (`read_paths`
only), so it cannot be changed from here.

Replacing it with an automatic digest would mean hashing
`internal/prompt`'s built-in template content from `cmd/aurumcode` through
an already-exported accessor. The only exported accessor that touches the
built-in template today, `PromptBuilder.FixedOverheadTokens`, returns a
token **count**, not the rendered text; the function that actually renders
the built-in instructions (`buildBasePrompt`) and the one that assembles the
fixed strings around it (`fixedOverhead`) are both unexported. Building a
safe, deterministic call into `PromptBuilder.BuildPrompt` with minimal
inputs (an empty diff, zero-value metrics) to recover a usable digest was
judged too invasive to attempt reliably within this card's time budget —
particularly confirming `formatMetrics`/`formatLanguages` tolerate a
zero-value `*analyzer.DiffMetrics` without a nil-pointer panic, which was
not verified. This is left as a follow-up: either export a small
`prompt.BuiltinTemplateDigest()`-style accessor, or a future card
specifically scoped to `internal/prompt` reads this document and closes
the gap. Until then, a change to the built-in prompt template still
requires a human to bump `PromptVersion` by hand, exactly as before this
card.
