// The review cache: a reviewed diff whose prompt, model, profiles and
// evidence did not change reuses the stored verdict instead of paying for the
// same model call twice.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// fileCacheStatus tracks, for one file of the reviewed diff, whether a
// previous run already reviewed byte-identical content for it under the
// same model and prompt version.
type fileCacheStatus struct {
	// path is filepath.Clean(file.Path), matching the cleaning
	// internal/review.enforceRuleCitations already applies to every
	// surviving issue's File field before this package ever sees it.
	path string
	key  string
	hit  bool
	// issues is populated only when hit is true: the cached findings for
	// this exact file, already redacted and rule-cited (see
	// internal/review/cache.Entry's doc).
	issues []types.ReviewIssue
}

// baseURLReporter is a provider that knows the endpoint it calls.
type baseURLReporter interface{ BaseURL() string }

// modelCacheKey identifies, for the cache, which "model" is answering this
// run -- not simply provider.Name(). Two things can make Name() alone
// under-identify the answering entity:
//
//   - The offline fixture provider (AURUMCODE_LLM_FIXTURE; see
//     selectProvider and selectProviderForModel above) always reports the
//     same Name() ("fixture", or the chosen --modelo value) no matter which
//     canned response file is configured, because Name() distinguishes
//     vendors/models, not responses -- but two different fixture files ARE,
//     for caching purposes, two different "models": each answers
//     deterministically, but a different way, so an entry cached under one
//     must never be served under the other (this is exactly what
//     tests/acceptance/AUR-432.sh's nominal_case exercises, reviewing the
//     same repository under three different fixtures in a row). Its content
//     digest is folded into the key whenever AURUMCODE_LLM_FIXTURE is set.
//   - --limite (AUR-433) wraps the selected provider in a
//     fixedModelProvider (cmd/aurumcode/cost.go) that pins the cost
//     tracker's accounting key via llm.ModelResolver -- but Name() is
//     promoted through that wrapper's embedded llm.Provider unchanged, so a
//     metered (--limite) run and an unmetered run against the identical
//     fixture would otherwise share a cache key even though a metered call
//     is guarded by a budget check an unmetered one never has (this is
//     exactly what tests/acceptance/AUR-433.sh's nominal_case exercises: a
//     baseline run with no --limite, immediately followed by --limite runs
//     against the same repository, each expected to still reach the
//     model). When the provider implements llm.ModelResolver, the resolved
//     key is folded in too -- harmless when it merely repeats what Name()
//     already said, decisive when it does not. Two different --limite
//     ceilings on the same resolved model still correctly share one entry:
//     the ceiling changes whether a call is ALLOWED, never what the model
//     would answer.
func modelCacheKey(provider llm.Provider) string {
	name := provider.Name()

	// AUR-513: two endpoints serving a model under the same name are not
	// the same answering entity -- a repository pointed at a self-hosted
	// gateway today and a different one tomorrow, same LLM_MODEL value,
	// must not share a cache entry. selectProvider/selectProviderForModel
	// read LLM_BASE_URL directly to build the real provider; read the same
	// variable here rather than adding an exported accessor to
	// internal/llm/provider/litellm (outside this card's paths).
	if baseURL := os.Getenv("LLM_BASE_URL"); baseURL != "" {
		name += ":baseurl:" + baseURL
	} else if endpoint, ok := provider.(baseURLReporter); ok && endpoint.BaseURL() != "" {
		// A catalog profile resolves its URL without LLM_BASE_URL (for
		// example an Azure resource and deployment): the same rule holds.
		name += ":baseurl:" + endpoint.BaseURL()
	}

	if fixturePath := os.Getenv("AURUMCODE_LLM_FIXTURE"); fixturePath != "" {
		data, err := os.ReadFile(fixturePath)
		if err != nil {
			// Provider selection already read this exact file successfully
			// (selectProvider / selectProviderForModel, moments earlier); a
			// failure here would be a race against the filesystem in
			// between. Fail toward "this run never serves, and never
			// becomes, a cache hit" rather than a stale or colliding key.
			name += ":fixture:unreadable"
		} else {
			sum := sha256.Sum256(data)
			name = fmt.Sprintf("%s:fixture:%x", name, sum)
		}
	}

	if resolver, ok := provider.(llm.ModelResolver); ok {
		if key := resolver.ResolveModel(llm.Options{}); key != "" {
			name += ":resolved:" + key
		}
	}

	return name
}

// AUR-513: the cache key must change whenever ANYTHING that can change the
// model's answer changes -- not only the file's own diff. reviewContextCacheKey
// folds in every such input this engine currently has: baseModelIdentity (see
// its own doc below -- the real answering model/endpoint, captured BEFORE any
// context wrapping can hide it), the review language and memory notes
// (AUR-441's original scope), the codebase-context pack (AUR-515/536, already
// derived from the FULL diff before per-file partitioning -- see AC-003's
// note on partitionByCache below), the selected reviewer profiles (AUR-502's
// profileIdentity, reviewprofile.Profile.Signature() joined -- built for
// exactly this comparison), the content of the assembled context block (repo
// and, under a central policy, the policy's own prompt/skills/docs --
// contextBlockDigest), and the dynamic rule/skill-section catalog the model
// was taught and the gate accepts citations against (ruleCatalogDigest). Two
// reviews that differ in ANY of these produce a different key and therefore
// never share a cache entry; two reviews identical in all of them may still
// validly reuse one.
func reviewContextCacheKey(provider llm.Provider, baseModelIdentity, language, codebase, notes, profiles, contextBlockDigest, ruleCatalogDigest string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%q\n%q\n%q\n%q\n%q\n%q\n%q", baseModelIdentity, language, codebase, notes, profiles, contextBlockDigest, ruleCatalogDigest)))
	return fmt.Sprintf("%s:context:%x", modelCacheKey(provider), sum)
}

// contextBlockCacheDigest returns a sha256 hex digest of the context block
// config.BuildContextBlockWithWarnings assembles from providers for
// changedPaths -- the repository's prompt/skills/docs AND, when a central
// policy is active, the policy's own (contextProviders in runReview already
// lists the policy's providers first, per AUR-518).
//
// This hashes the SAME, already-redacted block the real call sends to the
// model (filter applied), not each provider's raw text. That is the correct
// identity for a cache: if two configurations produce the identical outbound
// prompt, reusing the cached answer is correct regardless of what untracked
// difference existed before redaction. An earlier version of this function
// hashed raw provider output instead, specifically to dodge a collision this
// card's own test caught: redaction.Filter.Redact replaces every
// secret-shaped span with the same fixed "[REDACTED]" placeholder, so two
// files differing only in a secret's value can redact to byte-identical
// text. That collision is real, but it is not a NEW weakness introduced by
// hashing the redacted form -- it is the exact same thing the model itself
// already cannot distinguish in the live prompt it answers from. A cache
// that reuses an answer the model would have produced identically either way
// is correct, not unsafe; see docs/review-cache.md for the full account and
// why AC-002's own test now changes a plainly-visible (non-secret-shaped)
// line instead of a redacted one, so it actually exercises the configured
// text reaching the digest.
//
// The digest itself is still a one-way sha256 hex string: AC-002's "no
// secret in legible form" holds regardless of which text is hashed, because
// nothing in the KEY is ever the original text, redacted or not.
//
// This calls config.BuildContextBlockWithWarnings, the same builder the
// evidence step (review_weighing.go) uses to put the block in the prompt's
// evidence; no provider decorator carries the block any more. Every provider ConfiguredProviders returns
// today (RepoPromptProvider, FileContextProvider, TextContextProvider,
// PathInstructionsProvider; see internal/config/provider_files.go) reads a
// local file and does nothing else, so invoking the build a second time
// here, purely to digest it, is deterministic and side-effect-free;
// AUR-469/470 (MCP/RAG) are documented future extensions of the same seam,
// not present in this codebase today -- see docs/review-cache.md for what a
// stateful future provider would require this function to revisit.
// Warnings are intentionally discarded here: the real call in runReview
// already surfaces every provider warning to stderr once; duplicating that
// announcement from a digest-only call would print it twice for one review.
// A failure to build the block degrades to a fixed sentinel, never a crash
// and never a silent empty digest that could collide with "no context
// configured at all".
func contextBlockCacheDigest(providers []config.ContextProvider, changedPaths []string, filter *redaction.Filter) string {
	block, _, err := config.BuildContextBlockWithWarnings(context.Background(), providers, changedPaths, filter)
	if err != nil {
		return "digest-error"
	}
	sum := sha256.Sum256([]byte(block))
	return hex.EncodeToString(sum[:])
}

// ruleCatalogCacheDigest returns a deterministic sha256 hex digest of the
// expanded rule catalog (ruleCatalogIDs, AUR-519's embedded-plus-skill-section
// ID list the model is taught) and the dynamic rules backing it (dynamicRules,
// ID -> full Rule, including each rule's Description/Severity/Pattern/etc. --
// cmd/aurumcode/policygate.go). Both inputs are sorted first so that the
// identical configuration always serializes to the identical bytes regardless
// of map iteration order or the order review.context.skills/policySkillRules
// happened to merge in.
//
// DEFENSE IN DEPTH, not independently exercised end to end: today every
// skill file dynamicRulesFromLocalSkills reads (review.ParseSkillSections)
// is drawn from the exact same files contextBlockCacheDigest's block already
// hashes, so in practice a skill-content change that alters the derived
// rule catalog also changes the context-block digest, and this card's tests
// invalidate the cache through that shared path rather than isolating this
// one. This digest is kept anyway, independently folded into the key,
// because the two can diverge in the future without warning: a catalog
// entry derived with normalization the raw-text digest does not apply (e.g.
// whitespace-insensitive parsing), a central-policy-only rule source, or any
// other rule contribution that is not simply "the bytes of a context file"
// would change what the model is taught and what the gate accepts without
// necessarily changing the context block's bytes. See docs/review-cache.md.
func ruleCatalogCacheDigest(ruleCatalogIDs []string, dynamicRules map[string]review.Rule) string {
	ids := append([]string(nil), ruleCatalogIDs...)
	sort.Strings(ids)

	keys := make([]string, 0, len(dynamicRules))
	for k := range dynamicRules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rules := make([]review.Rule, 0, len(keys))
	for _, k := range keys {
		rules = append(rules, dynamicRules[k])
	}

	body, err := json.Marshal(struct {
		Catalog []string
		Rules   []review.Rule
	}{ids, rules})
	if err != nil {
		body = []byte("marshal-error")
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// partitionByCache resolves, for every file in diff.Files, whether c
// already holds that file's findings under model and promptVersion. It
// returns the files that still need a model call, in diff order, and the
// per-file status slice (also in diff order, one entry per diff.Files
// element) that mergeCacheHits and persistFreshResults use afterward. Every
// cached finding it returns is redacted with filter (cache_redaction.go).
//
// AUR-543: promptVersion is no longer the hand-bumped cache.PromptVersion
// constant. runReview (main.go) now passes a fresh
// internal/prompt.PromptBuilder's FixedContentDigest() -- a digest of the
// fixed content that builder actually renders (instructions, built-in rule
// catalog, schema text), computed at run time -- so editing that embedded
// content, without touching any constant, changes every key this function
// computes and forces a fresh review. See docs/review-cache.md.
//
// AC-003 (cross-file evidence survives a partial hit): diff here is the
// FULL reviewed diff, not yet reduced to misses -- that reduction (toSend in
// runReview) happens strictly AFTER this call returns. The chosen design
// does not try to detect which miss file "depends on" which hit file (that
// would need a dependency graph this package does not have and this card's
// Non-goals exclude building a remote index to get one); instead it relies
// on a fact already true one layer up, in runReview: resolveCodebaseContext
// is computed from diffPaths(diff) -- the SAME full, unpartitioned diff --
// BEFORE partitionByCache ever runs, so the codebase-context pack
// (AUR-515/536: symbols, references and dependents across every changed
// path) always carries the cache-hit file's symbols/content alongside the
// miss file's, regardless of which files ultimately hit. That pack's text
// is part of reviewContextCacheKey (the "codebase" parameter), so a change
// to that cross-file picture also invalidates the right cache entries. See
// docs/review-cache.md for the alternative considered (refusing to serve a
// hit for any file the changed set depends on) and why it was rejected.
func partitionByCache(c *cache.Cache, diff *types.Diff, model, promptVersion string, filter *redaction.Filter) (miss []types.DiffFile, statuses []fileCacheStatus) {
	statuses = make([]fileCacheStatus, len(diff.Files))
	for i, f := range diff.Files {
		key := cache.Key(f, model, promptVersion)
		statuses[i] = fileCacheStatus{path: filepath.Clean(f.Path), key: key}
		entry, ok, getErr := c.Get(key)
		if getErr == nil && ok {
			statuses[i].hit = true
			// A stored entry is untrusted input (a shared cache scope):
			// it passes the model-output redaction before any sink.
			statuses[i].issues = redactCachedIssues(filter, entry.Issues)
			continue
		}
		miss = append(miss, f)
	}
	return miss, statuses
}

// mergeCacheHits appends every cache-hit file's previously-found issues
// into result.Issues, and returns how many files were reused, for the
// caller's stderr note. The gate (--fail-on) and the printed findings both
// read result.Issues after this call, so a cached finding gates and prints
// exactly like a freshly produced one.
//
// Before appending, it FIRST drops from result.Issues any fresh issue whose
// File names a cache-hit file. This is not a hypothetical: the diff sent to
// GenerateReview on a partial hit carries only the miss files, but nothing
// stops a model's answer from still naming a file it was never shown --
// internal/review/fakeprovider.go's FakeProvider is a fixed canned
// response that ignores the prompt entirely, so a fixture planted for a
// cache-hit file keeps answering for it on every call regardless of
// whether that file was sent -- and a well-behaved live model is not
// guaranteed innocent of the same thing either. Without this filter, a
// cache-hit file's already-merged cached finding and that same stale fresh
// one would both survive, printing the finding twice. filepath.Clean(path)
// and filter.Redact(that) mirror persistFreshResults' own two-attempt
// bucketing, for the same reason: an ordinary path matches on the first
// attempt (filter-identity), a secret-shaped one only after redaction.
func mergeCacheHits(result *types.ReviewResult, statuses []fileCacheStatus, filter *redaction.Filter) int {
	hitPaths := make(map[string]bool)
	for _, st := range statuses {
		if st.hit {
			hitPaths[st.path] = true
			hitPaths[filter.Redact(st.path)] = true
		}
	}
	if len(hitPaths) > 0 {
		kept := make([]types.ReviewIssue, 0, len(result.Issues))
		for _, issue := range result.Issues {
			if hitPaths[filepath.Clean(issue.File)] {
				continue // a cache-hit file was not sent this round; a fresh answer naming it is stale, never authoritative.
			}
			kept = append(kept, issue)
		}
		result.Issues = kept
	}

	reused := 0
	for _, st := range statuses {
		if !st.hit {
			continue
		}
		reused++
		result.Issues = append(result.Issues, st.issues...)
	}
	return reused
}

// persistFreshResults stores, per miss file, exactly the issues the model
// returned for that file -- including an empty slice for a clean file, so a
// clean file becomes a cache hit on the next run instead of being resent
// forever. issues is result.Issues from the GenerateReview call that just
// ran: every string in it already passed
// internal/review.redactReviewResult, before this function or its caller
// ever saw it (AUR-432), so nothing unredacted reaches cache.Put.
//
// Bucketing tries the file's own clean path first, then that same path
// passed through filter (matching internal/review.redactReviewResult,
// which redacts issue.File at the model-output boundary before returning):
// for an ordinary path this second attempt is a no-op (filter-identity),
// but for a path whose text happens to be secret-shaped, issue.File comes
// back redacted while the diff's own file.Path does not, and skipping this
// second attempt would silently cache that file as "reviewed and clean",
// making its real finding vanish on the very next run -- the determinism
// AC-001 also requires.
func persistFreshResults(c *cache.Cache, statuses []fileCacheStatus, issues []types.ReviewIssue, filter *redaction.Filter) {
	byPath := make(map[string][]types.ReviewIssue)
	for _, issue := range issues {
		p := filepath.Clean(issue.File)
		byPath[p] = append(byPath[p], issue)
	}
	for _, st := range statuses {
		if st.hit {
			continue
		}
		found := byPath[st.path]
		if len(found) == 0 {
			found = byPath[filter.Redact(st.path)]
		}
		// Caching is a best-effort performance optimization, never a
		// correctness gate: a write failure here (a full disk, a cache
		// directory that turned read-only mid-run) must not fail a review
		// that otherwise completed successfully. The next run simply sees
		// this file as a miss again, exactly as if this entry had never
		// existed.
		_ = c.Put(st.key, cache.Entry{Path: st.path, Issues: found})
	}
}
