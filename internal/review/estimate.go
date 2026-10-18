package review

import (
	"errors"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// EstimatePromptTokens is the estimated size, in the product's tokens, of
// what a review of diff would send first: the prompts the same builder
// assembles (instructions, policy, codebase context, evidence and diff),
// under the declared window's ceiling when one is declared, summed over the
// batches when that ceiling splits the review. It is the --limite
// pre-flight's count (#198); a refusal the model may still answer with is
// not foreseen. An error, and no count, when no prompt can be assembled or
// batches.max_prompt_tokens admits no batch: the review would send nothing
// it could be priced by.
func (r *Reviewer) EstimatePromptTokens(diff *types.Diff, reviewContext ReviewContext) (int, error) {
	defer func() { r.learnedBudget, r.contextFits, r.contextReduction, r.batches = 0, nil, nil, nil }()
	r.learnedBudget, r.contextFits, r.contextReduction = declaredCeiling(r.cfg.ModelWindowTokens), nil, nil
	tokens, err := r.estimateOnce(diff, reviewContext)
	if r.declaredWindowTooSmall(err) {
		// As the review would: the declared window is set aside.
		r.learnedBudget, r.contextFits, r.contextReduction = 0, nil, nil
		tokens, err = r.estimateOnce(diff, reviewContext)
	}
	if err != nil {
		return 0, err
	}
	return tokens, nil
}

// estimateOnce is reviewOnce's assembly without the model: one prompt, or
// the sum of the planned batches.
func (r *Reviewer) estimateOnce(diff *types.Diff, reviewContext ReviewContext) (int, error) {
	prepared, err := r.preparePrompt(diff, reviewContext)
	var noRoom *prompt.NoRoomError
	split := false
	switch {
	case errors.As(err, &noRoom) && splittable(diff):
		split = true
	case err != nil:
		return 0, err
	default:
		if split, err = r.needsBatches(diff, prepared, reviewContext); err != nil {
			return 0, err
		}
	}
	if !split {
		return metaCount(prepared.parts.Meta, "estimated_tokens"), nil
	}
	plan, err := r.planBatches(diff, reviewContext)
	if err != nil {
		return 0, err
	}
	if len(plan.batches) == 0 {
		return 0, r.noBatchReviewed(plan)
	}
	total := 0
	for _, batch := range plan.batches {
		total += metaCount(batch.prepared.parts.Meta, "estimated_tokens")
	}
	return total, nil
}
