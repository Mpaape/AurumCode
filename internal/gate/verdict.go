// AUR-524: the gate verdict for the same reviewed content (diff and, when
// known, SHA), the same central policy, the same repository context/
// skills and the same model must not flip a concluded FAIL into a PASS --
// not between two runs of the same command, and not between a --base run
// and a --pr run.
//
// Version 2 (2026-10-02, after independent review): AURUMCODE_CACHE_DIR is
// UNTRUSTED input in CI -- in a shared runner/cache scope, another
// workflow (or a step with different privileges) can write into the exact
// same directory this card reads from. A naive "reuse the stored verdict
// wholesale" design would let a forged, stale, or cross-repo entry (e.g.
// one planted with an empty issues list) turn a REAL failure into a
// silent pass -- the opposite of what a compliance gate exists to prevent
// (CR-TRUST-001). So reuse here is MONOTONIC: a stored entry can only ADD
// findings to what this run already found on its own, never remove or
// replace them. The worst a forged/stale/cross-repo entry can do is add a
// spurious failure (a false negative becomes visible, loudly, as an extra
// finding to adjudicate) -- it can never manufacture an approval. Reused
// and current findings are deduped by exact equality (types.ReviewIssue
// is a plain comparable struct, no slice/map fields).
//
// This reuses AUR-513's own key builders (modelCacheKey,
// reviewContextCacheKey) and AUR-441's own on-disk store
// (internal/review/cache.Cache; cache.Entry's Issues field), never
// duplicating or replacing either. What is stored is the RAW, pre-rule-
// config issues (before config.ApplyRuleConfig), because a reused entry
// must be re-evaluated against THIS run's rule config, not the config
// that happened to be in effect when it was written (AC-006): a rule a
// repository has since disabled must stop firing even out of a reused
// entry, and a rule re-enabled after being disabled when the entry was
// written must start firing again.
package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// VerdictCachePath is the fixed cache.Entry.Path label a verdict entry
// is stored under -- never a real diff file path, so it reads
// unambiguously in the on-disk JSON even though the directory is shared
// with AUR-513's own per-file entries (today's cache.ResolveDir()). The
// verdict KEY (VerdictCacheKey) is what actually separates the two
// kinds of entry; this is only a readability label, and the one marker
// the acceptance tests use to confirm an inconclusive run left nothing
// behind.
const VerdictCachePath = "aur524-gate-verdict"

// VerdictCacheUnavailableNotice is AC-004's own declaration, on stderr, in
// English (the catalog's terminal.verdict_cache_unavailable carries it per
// language):
// without a persistent cache directory, this run's gate verdict cannot be
// shared with a later run, and could not have reused an earlier one
// either. The run still proceeds -- a missing cache is never a correctness
// gate here any more than it is for AUR-441's own cache.
const VerdictCacheUnavailableNotice = "gate verdict reuse unavailable (AURUMCODE_CACHE_DIR not set): this run's verdict cannot be shared with another run, and could not reuse one either"

// VerdictCacheAvailable is AC-004's gate: this card's reuse is only
// ever attempted when the caller explicitly configured a cache directory
// meant to outlive this one process (AURUMCODE_CACHE_DIR, cache.EnvDir).
// Without it, cache.ResolveDir()'s own default
// (os.TempDir()/aurumcode-review-cache-<pid>) is scoped to THIS process's
// pid -- never a location two separate `aurumcode` invocations could share
// by construction -- so treating it as a valid verdict store would
// silently promise a reuse guarantee that cannot structurally be kept.
func VerdictCacheAvailable() bool {
	return strings.TrimSpace(os.Getenv(cache.EnvDir)) != ""
}

// DiffContentDigest is a deterministic sha256 hex digest of diff.Files --
// ALWAYS folded into the key (unlike this card's v1, which keyed on a SHA
// OR a diff digest): a reviewed SHA is also folded in when known
// (VerdictKeyInputs.ReviewedSHA), but the SHA is never trusted alone,
// only ever alongside the content it is supposed to name. A nil diff
// (never expected in practice) digests the same as an empty one.
func DiffContentDigest(diff *types.Diff) string {
	var files []types.DiffFile
	if diff != nil {
		files = diff.Files
	}
	body, err := json.Marshal(files)
	if err != nil {
		return "diff:marshal-error"
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// VerdictKeyInputs groups every input VerdictCacheKey combines.
// Named fields, not positional strings, deliberately: runReview and
// runPRReview both build one of these, and a transposed positional
// argument between two ~15-parameter call sites is exactly the kind of
// mistake that would silently weaken this card's own guarantee.
type VerdictKeyInputs struct {
	// ContextKey yields the review-context cache key (the command builds it
	// from the model, language, codebase, notes, profiles and rule catalog).
	// It is lazy: the key is only needed when a verdict can be reused.
	ContextKey func() string

	// PolicyDigest is config.Config.PolicyDigest -- AUR-521's own audit digest
	// over the active central policy's config.yml and every skill it
	// names ("" when no policy is active). MUT-001 removes this term;
	// AC-002's tests must then fail.
	PolicyDigest string
	// PromptVersionDigest is AUR-543's own content digest of the fixed
	// prompt a builder renders (prompt.PromptBuilder.FixedContentDigest,
	// supplied by the command through VerdictReuseContributor.PromptDigest)
	// -- the SAME value runReview's per-file cache
	// already folds into cache.Key's promptVersion argument, reused here
	// rather than recomputed with different inputs.
	PromptVersionDigest string
	// BinaryIdentity is binaryIdentity() above: a stale binary never
	// shares a verdict with a current one.
	BinaryIdentity string
	// RepoIdentity is owner/repoName on --pr, localRepoIdentity(cwd) on
	// --base, "" when unknown (fails closed: an unknown repo never
	// collides with a known one).
	RepoIdentity string
	// DiffDigest is DiffContentDigest(diff) -- ALWAYS present.
	DiffDigest string
	// ReviewedSHA is GITHUB_SHA when set, "" otherwise -- folded in
	// ADDITIONALLY to DiffDigest, never as a substitute for it.
	ReviewedSHA string
}

// VerdictCacheKey combines reviewContextCacheKey's own, unduplicated
// result with every further input VerdictKeyInputs names. A
// difference in ANY of them changes the key, so a reused verdict can
// never cross a policy, a skill, a model/endpoint, a prompt version, a
// binary build, a repository or a reviewed diff/SHA.
func VerdictCacheKey(in VerdictKeyInputs) string {
	inner := in.ContextKey()
	body, err := json.Marshal(struct {
		Inner               string
		PolicyDigest        string
		PromptVersionDigest string
		BinaryIdentity      string
		RepoIdentity        string
		DiffDigest          string
		ReviewedSHA         string
	}{inner, in.PolicyDigest, in.PromptVersionDigest, in.BinaryIdentity, in.RepoIdentity, in.DiffDigest, in.ReviewedSHA})
	if err != nil {
		body = []byte("marshal-error")
	}
	sum := sha256.Sum256(body)
	return fmt.Sprintf("verdict:%x", sum)
}

// LoadGateVerdict looks up a previously stored RAW issue set for key. A
// read/parse error or a plain miss are both treated as "nothing to
// reuse" -- a corrupted or forged-but-unparseable entry degrades to a
// fresh review, never to a crash.
func LoadGateVerdict(key string) ([]types.ReviewIssue, bool) {
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

// StoreGateVerdict persists rawIssues -- result.Issues exactly as it
// stood immediately BEFORE config.ApplyRuleConfig, still redacted and
// rule-cited (the same boundary cache.Entry.Issues' own doc already
// requires of AUR-513's per-file entries), so a later run can re-evaluate
// them against ITS OWN rule config (AC-006) rather than freezing whatever
// config happened to be active when this run wrote the entry. The caller
// must never call this for an inconclusive run (AC-003). A write failure
// is swallowed: caching is a performance/stability optimization, never a
// correctness gate.
func StoreGateVerdict(key string, rawIssues []types.ReviewIssue) {
	c, err := cache.Open(cache.ResolveDir())
	if err != nil {
		return
	}
	_ = c.Put(key, cache.Entry{Path: VerdictCachePath, Issues: rawIssues})
}

// UnionReviewIssues is this card's own trust boundary: current (what THIS
// run actually found, on its own, with no cache involved) always
// survives untouched; reused (a stored entry, already re-evaluated
// against this run's rule config) can only ADD to it. types.ReviewIssue
// is a plain, fully comparable struct, so exact-equality dedup via a map
// key is correct and needs no custom Equal.
func UnionReviewIssues(current, reused []types.ReviewIssue) []types.ReviewIssue {
	seen := make(map[types.ReviewIssue]bool, len(current)+len(reused))
	out := make([]types.ReviewIssue, 0, len(current)+len(reused))
	for _, issue := range current {
		if !seen[issue] {
			seen[issue] = true
			out = append(out, issue)
		}
	}
	for _, issue := range reused {
		if !seen[issue] {
			seen[issue] = true
			out = append(out, issue)
		}
	}
	return out
}

// ReuseOrStoreGateVerdict is called by VerdictReuseContributor
// (contributors.go), the verdict-reuse step both --base and --pr assemble,
// right after the inconclusive reason is final and right before
// EvaluateGate runs. currentIssues is this run's own, freshly computed,
// already-rule-config-applied result.Issues; rawIssues is the SAME run's
// issues captured immediately before that rule config was applied (what
// gets stored on a miss). cfg is the run's effective config (run.Cfg),
// reapplied to a reused entry's raw issues before the union (AC-006).
//
// Every path through this function returns currentIssues UNCHANGED
// unless a hit actually adds something:
//   - !gateDeclared, !VerdictCacheAvailable(), !promptDigestOK, or
//     gateInconclusiveReason != "" (AC-003): no read, no write, no
//     announcement other than AC-004's own unavailability notice.
//   - a miss stores rawIssues (conclusive only) and returns currentIssues.
//   - a hit reapplies cfg to the stored raw issues, unions the result
//     into currentIssues (monotonic: current never shrinks or is
//     replaced), announces how many were newly added when that is
//     nonzero, and returns the union.
func ReuseOrStoreGateVerdict(language string, stderr io.Writer, limitations *[]string, gateDeclared, promptDigestOK bool, in VerdictKeyInputs, cfg *config.Config, gateInconclusiveReason string, currentIssues, rawIssues []types.ReviewIssue) []types.ReviewIssue {
	if !gateDeclared {
		return currentIssues
	}
	if !VerdictCacheAvailable() {
		// An operator note: said on stderr, never in the parecer, whose
		// reader cannot act on a cache directory of the runner.
		fmt.Fprintf(stderr, "aurumcode review: %s\n", i18n.Text(language, "terminal.verdict_cache_unavailable"))
		return currentIssues
	}
	if !promptDigestOK {
		return currentIssues
	}
	// AC-003: an inconclusive run never reuses a prior verdict and never
	// becomes one either -- under this card's own monotonic design a
	// reused entry could never turn a FAIL into a PASS anyway, but an
	// inconclusive run has no trustworthy raw issues of its own to add
	// to the store either.
	if gateInconclusiveReason != "" {
		return currentIssues
	}
	key := VerdictCacheKey(in)
	if stored, hit := LoadGateVerdict(key); hit {
		reapplied := config.ApplyRuleConfig(stored, cfg)
		merged := UnionReviewIssues(currentIssues, reapplied)
		if added := len(merged) - len(currentIssues); added > 0 {
			fmt.Fprintf(stderr, "aurumcode review: %s\n", reappliedText(language, added))
		}
		return merged
	}
	StoreGateVerdict(key, rawIssues)
	return currentIssues
}

// reappliedText counts the findings a reused verdict added, in language.
func reappliedText(language string, added int) string {
	if added == 1 {
		return i18n.Text(language, "terminal.verdict_reapplied_one")
	}
	return i18n.Format(language, "terminal.verdict_reapplied", added)
}
