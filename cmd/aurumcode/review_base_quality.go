// The model's quality pass of the --base path: cost cap (--limite), review
// cache, profile passes and the single review call.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// qualityCache is the per-file cache state of one quality pass. err != nil
// degrades to AUR-430's original behavior: every file sent, every run.
type qualityCache struct {
	store    *cache.Cache
	err      error
	statuses []fileCacheStatus
	toSend   *types.Diff
}

// runQualityPass runs everything that talks to a model. When the quality
// review was skipped or already failed, result stays the zero ReviewResult:
// no quality issues, nothing to gate or print for that section (AUR-449).
func (b *baseReview) runQualityPass() (int, bool) {
	if b.qualityDidNotRun() {
		b.result = &types.ReviewResult{}
		return 0, false
	}
	if code, done := b.setupCostCap(); done {
		return code, true
	}
	orchestrator := llm.NewOrchestrator(b.provider, nil, b.tracker)
	b.verifyCaller = orchestrator
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())
	reviewer.SetBatchLimits(configuredBatchLimits(b.cfg))
	// AUR-519: teach the model the expanded catalog and accept its
	// citations against the same dynamic set computed earlier.
	reviewer.SetDynamicRules(b.dynamicRules)
	if err := reviewer.SetRuleCatalog(b.ruleCatalogIDs); err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	b.prepareDeliberation(orchestrator, toolsCapable(orchestrator, b.profilesApplied), reviewer)
	qc := b.prepareCache()
	// A diff with no files at all is the pre-existing "nothing changed"
	// edge case and is still sent; only a genuine full cache hit skips the
	// call.
	if qc.err != nil || len(qc.toSend.Files) > 0 || len(b.diff.Files) == 0 {
		if code, done := b.callModel(reviewer, qc); done {
			return code, true
		}
	} else {
		b.result = &types.ReviewResult{}
	}
	b.reportQualityOutcome(qc)
	return 0, false
}

// setupCostCap wires --limite (AUR-433): the tracker estimates the cost and
// refuses -- before the model is ever called -- when it exceeds the ceiling.
func (b *baseReview) setupCostCap() (int, bool) {
	if !b.limiteSet {
		return 0, false
	}
	price, err := costPrice()
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	modelKey := costModelKey(b.f.modelo)
	b.provider = &fixedModelProvider{Provider: b.provider, model: modelKey}
	b.tracker = buildCostTracker(b.limiteUSD, modelKey, price)
	printCostEstimate(b.stderr, estimateCostUSD(b.diff, price), b.limiteUSD)
	return 0, false
}

// callModel sends the cache misses to the model (one call, or one pass per
// profile sharing the cost tracker) and persists fresh results. With
// --seguranca a failed quality review diverts to the security pass alone
// (AUR-458); without it the published exit-1 refusal stays.
func (b *baseReview) callModel(reviewer *review.Reviewer, qc *qualityCache) (int, bool) {
	reviewCtx := b.reviewContext(review.ReviewContext{
		Language:        b.reviewLanguage,
		CodebaseContext: b.codebaseText,
		MemoryNotes:     b.memoryNotesText,
	})
	var err error
	if b.profilesApplied {
		// AUR-519: every profile's Reviewer accepts the same dynamic rules
		// and expanded catalog as the single-reviewer path.
		b.result, err = runProfilePasses(b.ctx, b.provider, b.tracker, b.profileRes.Profiles, qc.toSend, reviewCtx, b.dynamicRules, b.ruleCatalogIDs)
	} else {
		b.result, err = reviewer.GenerateReviewWithContext(b.ctx, qc.toSend, reviewCtx)
		b.transcript = reviewer.Transcript()
		b.noteBatches(reviewer.Batches())
	}
	if b.noteDeliberationLimit(err) {
		return 0, false
	}
	if err != nil && b.f.seguranca {
		reportQualityFailure(b.stderr, err, b.f.modelo, b.limiteUSD)
		fmt.Fprintln(b.stderr, "aurumcode review: quality review failed; running --seguranca only -- this run reviewed HALF of what was asked")
		b.model = modelProviderFailed
		b.result = &types.ReviewResult{}
	} else if err != nil {
		return reportQualityFailure(b.stderr, err, b.f.modelo, b.limiteUSD), true
	}
	// B3: a degraded parse or a partially covered file is never persisted:
	// a later cache hit would claim a complete review.
	cachePartial := mergeReviewCoverage(b.result.Metadata, b.notices, b.rawDiffFileCount, b.ignoredPaths).partial()
	// A review answered by a fallback provider is never stored under the
	// primary's cache identity (AUR-605).
	if qc.err == nil && b.model != modelProviderFailed && !prompt.IsDegradedParse(b.result) && !cachePartial && !answeredByFallback(b.provider) {
		persistFreshResults(qc.store, qc.statuses, b.result.Issues, b.filter)
	}
	return 0, false
}

// reportQualityOutcome prints the discard warnings (AUR-448/459, never
// silent), merges cache hits into the answer and reports cache reuse and
// the real cost. None of it applies to a failed review (AUR-458).
func (b *baseReview) reportQualityOutcome(qc *qualityCache) {
	for _, key := range []string{"discard_warning", "scope_discard_warning", "parse_discard_warning", review.AssessmentDiscardWarningKey} {
		if warning := b.result.Metadata[key]; warning != "" {
			fmt.Fprintf(b.stderr, "aurumcode review: %s\n", warning)
		}
	}
	reused := 0
	if qc.err == nil && b.model != modelProviderFailed {
		reused = mergeCacheHits(b.result, qc.statuses, b.filter)
	}
	if reused > 0 {
		fmt.Fprintf(b.stderr, "aurumcode review: reused %d file(s) from cache (not resent to the model)\n", reused)
	}
	if b.limiteSet && b.model != modelProviderFailed {
		printRealCost(b.stderr, realCostUSD(b.tracker, b.limiteUSD), b.limiteUSD)
	}
}
