// AUR-441: do not pay twice for the same file.
//
// internal/review.Reviewer.GenerateReview sends the whole reviewed diff to
// the model in a single prompt, one Complete call per invocation
// (internal/review/reviewer.go:109,117,120; internal/prompt/builder.go
// folds every file into that one prompt) -- there is no per-file send for a
// cache to intercept. So the wiring lives here, in cmd/aurumcode, one layer
// above GenerateReview: filter diff.Files down to the files
// internal/review/cache does not already hold an entry for BEFORE calling
// GenerateReview (zero misses skips the call to the model entirely), merge
// the cache hits' previously-found issues into the printed result, and
// report how many files were reused.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/llm"
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

// modelCacheKey identifies, for the cache, which "model" is answering this
// run -- not simply provider.Name(). The offline fixture provider
// (AURUMCODE_LLM_FIXTURE; see selectProvider and selectProviderForModel
// above) always reports the same Name() ("fixture", or the chosen
// --modelo value) no matter which fixture file is configured, because
// Name() distinguishes vendors/models, not canned responses -- but two
// different fixture files ARE, for caching purposes, two different
// "models": each answers deterministically, but a different way, so an
// entry cached under one must never be served under the other (this is
// exactly what tests/acceptance/AUR-432.sh's nominal_case exercises,
// reviewing the same repository under three different fixtures in a row).
// When AURUMCODE_LLM_FIXTURE is set, its content digest is folded into the
// key; when it is not (a live provider, selected by LLM_API_KEY and
// LLM_BASE_URL), provider.Name() is what actually determines the answer,
// so it is used verbatim.
func modelCacheKey(provider llm.Provider) string {
	name := provider.Name()
	fixturePath := os.Getenv("AURUMCODE_LLM_FIXTURE")
	if fixturePath == "" {
		return name
	}
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		// Provider selection already read this exact file successfully
		// (selectProvider / selectProviderForModel, moments earlier); a
		// failure here would be a race against the filesystem in between.
		// Fail toward "this run never serves, and never becomes, a cache
		// hit" rather than a stale or colliding key.
		return name + ":fixture:unreadable"
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s:fixture:%x", name, sum)
}

// partitionByCache resolves, for every file in diff.Files, whether c
// already holds that file's findings under model. It returns the files
// that still need a model call, in diff order, and the per-file status
// slice (also in diff order, one entry per diff.Files element) that
// mergeCacheHits and persistFreshResults use afterward.
func partitionByCache(c *cache.Cache, diff *types.Diff, model string) (miss []types.DiffFile, statuses []fileCacheStatus) {
	statuses = make([]fileCacheStatus, len(diff.Files))
	for i, f := range diff.Files {
		key := cache.Key(f, model, cache.PromptVersion)
		statuses[i] = fileCacheStatus{path: filepath.Clean(f.Path), key: key}
		entry, ok, getErr := c.Get(key)
		if getErr == nil && ok {
			statuses[i].hit = true
			statuses[i].issues = entry.Issues
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
