package review

import (
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// RequestCacheKey returns the cache key of the review request
// GenerateReviewWithContext would send for diff and reviewContext: the
// digest of the final prompt text plus the digests of the evidence offered
// and of toolResults (any JSON-encodable record of the tool results the
// engine consumed; nil when none).
func (r *Reviewer) RequestCacheKey(diff *types.Diff, reviewContext ReviewContext, toolResults any) (string, error) {
	promptDigest, err := r.PromptDigest(diff, reviewContext)
	if err != nil {
		return "", err
	}
	evidenceDigest, err := cache.DigestOf(reviewContext.Evidence)
	if err != nil {
		return "", err
	}
	toolDigest, err := cache.DigestOf(toolResults)
	if err != nil {
		return "", err
	}
	return cache.RequestKey(cache.RequestKeyInput{
		PromptDigest:      promptDigest,
		EvidenceDigest:    evidenceDigest,
		ToolResultsDigest: toolDigest,
	}), nil
}
