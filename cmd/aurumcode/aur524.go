// AUR-524: the gate verdict for the same reviewed SHA (or, absent one, the
// same reviewed diff content), the same central policy, the same
// repository context/skills and the same model must not flip between a
// --base run and a --pr run.
//
// AUR-513's per-file review cache (review_cache.go) already solves "do not
// pay twice for the same file content" for --base alone, keyed per file.
// This is a second, coarser-grained reuse of the FINISHED review's own
// issues -- exactly the slice evaluateGate is handed -- keyed by the WHOLE
// reviewed SHA/diff plus the active policy's own digest, reusing the exact
// SAME key builders AUR-513 already publishes (modelCacheKey via
// reviewContextCacheKey) rather than duplicating them, and the SAME
// on-disk store AUR-441 already defines (internal/review/cache.Cache;
// cache.Entry's Issues field is exactly the shape this needs) rather than
// a second store. It is deliberately engaged on --base AND --pr alike
// (AUR-513's own per-file cache is --base only; this one is not), and
// deliberately gated on a gate actually being declared -- there is no
// "verdict" to stabilize otherwise, and every other run stays completely
// untouched by this card.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// gateVerdictCachePath is the fixed cache.Entry.Path label a verdict entry
// is stored under -- never a real diff file path, so it reads unambiguously
// in the on-disk JSON even though the directory is shared with AUR-513's
// own per-file entries (today's cache.ResolveDir()). The verdict KEY (see
// gateVerdictCacheKey) is what actually separates the two kinds of entry;
// this is only a readability label.
const gateVerdictCachePath = "aur524-gate-verdict"

// gateVerdictCacheUnavailableNotice is AC-004's own declaration: without a
// persistent cache directory, this run's gate verdict cannot be shared
// with a later run, and could not have reused an earlier one either. The
// run still proceeds -- a missing cache is never a correctness gate here
// any more than it is for AUR-441's own cache (see persistFreshResults) --
// this only says so plainly, on stderr and in the published limitations.
const gateVerdictCacheUnavailableNotice = "gate verdict reuse unavailable (AURUMCODE_CACHE_DIR not set): this run's verdict cannot be shared with another run, and could not reuse one either"

// gateVerdictCacheAvailable is AC-004's gate: this card's reuse is only
// ever attempted when the caller explicitly configured a cache directory
// meant to outlive this one process (AURUMCODE_CACHE_DIR, cache.EnvDir).
// Without it, cache.ResolveDir()'s own default
// (os.TempDir()/aurumcode-review-cache-<pid>) is scoped to THIS process's
// pid -- cache.ResolveDir's own doc already explains that default is never
// a location two separate `aurumcode` invocations could share by
// construction -- so treating it as a valid verdict store would silently
// promise a reuse guarantee that cannot structurally be kept across a
// --pr run and a later --base run (or vice versa): exactly the flip this
// card exists to prevent.
func gateVerdictCacheAvailable() bool {
	return strings.TrimSpace(os.Getenv(cache.EnvDir)) != ""
}

// gateVerdictCacheKey combines AUR-513's own reviewContextCacheKey (model
// identity including base URL, review language, codebase context, memory
// notes, profile selection, context-block digest and rule-catalog digest --
// see review_cache.go; reused verbatim, never recomputed here) with the two
// further inputs that make a verdict specific to THIS card: policyDigest
// (render.PolicyDigest, AUR-521's own audit digest over the active central
// policy's config.yml and every skill it names -- "" when no policy is
// active) and reviewedIdentity (the exact commit or diff this run
// evaluated -- see reviewedIdentity below). A difference in ANY of these
// inputs changes the key, so a reused verdict can never cross a policy, a
// skill, a model/endpoint or a reviewed commit (AC-002). MUT-001 removes
// exactly the policyDigest term from this combination -- AC-002's own test
// must then fail (a policy change would wrongly keep reusing the old
// verdict).
func gateVerdictCacheKey(provider llm.Provider, baseModelIdentity, language, codebase, notes, profiles, contextBlockDigest, ruleCatalogDigest, policyDigest, reviewedIdentity string) string {
	inner := reviewContextCacheKey(provider, baseModelIdentity, language, codebase, notes, profiles, contextBlockDigest, ruleCatalogDigest)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\n%q\n%q", inner, policyDigest, reviewedIdentity)))
	return fmt.Sprintf("verdict:%x", sum)
}

// reviewedIdentity returns "sha:<commit>" when a reviewed commit SHA is
// known (GITHUB_SHA, read identically by --base's runReview and --pr's
// runPRReview), or else a content digest of the reviewed diff itself
// ("diff:<sha256 hex>") when no SHA is available at all (a local --base
// run outside CI). This card's own Non-goals explicitly exclude caching
// across two DIFFERENT SHAs that happen to produce an identical diff --
// this fallback is not that: it only applies when there is no SHA to key
// on in the first place, so the SAME diff reviewed twice in a row still
// has a stable identity to reuse against.
func reviewedIdentity(sha string, diff *types.Diff) string {
	sha = strings.TrimSpace(sha)
	if sha != "" {
		return "sha:" + sha
	}
	var files []types.DiffFile
	if diff != nil {
		files = diff.Files
	}
	body, err := json.Marshal(files)
	if err != nil {
		return "diff:marshal-error"
	}
	sum := sha256.Sum256(body)
	return "diff:" + hex.EncodeToString(sum[:])
}

// loadGateVerdict looks up a previously concluded review's gate-relevant
// issues for key. A read/parse error or a plain miss are both treated as
// "nothing to reuse" -- the gate checklist's own requirement that a
// corrupted entry degrades to a fresh review, never to a crash or a false
// hit. Callers must only call this when gateVerdictCacheAvailable().
func loadGateVerdict(key string) ([]types.ReviewIssue, bool) {
	c, err := cache.Open(cache.ResolveDir())
	if err != nil {
		return nil, false
	}
	entry, ok, getErr := c.Get(key)
	if getErr != nil || !ok {
		return nil, false
	}
	return entry.Issues, true
}

// storeGateVerdict persists issues -- already redacted and rule-cited,
// exactly result.Issues as evaluateGate itself receives them, the same
// boundary cache.Entry.Issues' own doc already requires of AUR-513's
// per-file entries -- under key. The caller must never call this for an
// inconclusive, degraded or partially-covered review (AC-003): that
// decision is made by the caller with the same gateInconclusiveReason
// signal the gate itself already computed, never re-derived here. A write
// failure is swallowed for the same reason persistFreshResults swallows
// one: caching is a performance optimization, never a correctness gate.
func storeGateVerdict(key string, issues []types.ReviewIssue) {
	c, err := cache.Open(cache.ResolveDir())
	if err != nil {
		return
	}
	_ = c.Put(key, cache.Entry{Path: gateVerdictCachePath, Issues: issues})
}

// reuseOrStoreGateVerdict is the one call site both runReview and
// runPRReview make, right after gateInconclusiveReason is final and right
// before evaluateGate runs. On a hit, it returns the reused issues and
// true; the caller then evaluates the gate against THOSE issues instead of
// the ones this run just produced, so the verdict for an unchanged
// SHA/policy/context/model is byte-for-byte the one a previous concluded
// run already reached. On a miss, and only when this run itself is
// conclusive (gateInconclusiveReason == ""), it stores this run's own
// issues for a later run to reuse. Every path through this function is a
// no-op (false, unchanged issues) unless gate.Declared() -- a run with no
// `gate:` key anywhere carries no trace of this card.
func reuseOrStoreGateVerdict(stderr io.Writer, limitations *[]string, gateDeclared bool, provider llm.Provider, baseModelIdentity, language, codebase, notes, profiles, contextBlockDigest, ruleCatalogDigest, policyDigest, reviewedSHA string, diff *types.Diff, gateInconclusiveReason string, issues []types.ReviewIssue) ([]types.ReviewIssue, bool) {
	if !gateDeclared {
		return issues, false
	}
	if !gateVerdictCacheAvailable() {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", gateVerdictCacheUnavailableNotice)
		*limitations = append(*limitations, gateVerdictCacheUnavailableNotice)
		return issues, false
	}
	// AC-003: an inconclusive run never reuses a prior verdict (that would
	// let a stale pass paper over today's real problem) and never becomes
	// one either (nothing here would be safe to serve to a later run).
	if gateInconclusiveReason != "" {
		return issues, false
	}
	key := gateVerdictCacheKey(provider, baseModelIdentity, language, codebase, notes, profiles, contextBlockDigest, ruleCatalogDigest, policyDigest, reviewedIdentity(reviewedSHA, diff))
	if cached, hit := loadGateVerdict(key); hit {
		fmt.Fprintln(stderr, "aurumcode review: gate verdict reused from cache (same reviewed SHA, policy, context/skills and model)")
		return cached, true
	}
	storeGateVerdict(key, issues)
	return issues, false
}
