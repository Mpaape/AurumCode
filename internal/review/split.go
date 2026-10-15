package review

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A prompt the model refuses as too long is split, not given up on.
//
// The product puts no ceiling on a prompt (issue #179), so the first attempt
// sends the whole diff. When the provider answers that the prompt is longer
// than its context window, the reviewer learns a ceiling from the refusal --
// half of the prompt it just sent -- and tries again with the machinery that
// trims by whole hunks and, when a diff has several files, reviews it in
// batches (batches.go). Every refusal halves the ceiling again. The loop ends
// when a prompt is accepted, or when the fixed part of the prompt alone no
// longer leaves room for a single change: then the review fails loudly,
// naming the refusal. The ceiling lives only for the current review.

// PromptRefusal is one prompt a provider refused as longer than its context
// window, with its estimated size.
type PromptRefusal struct {
	// EstimatedTokens is the size of the refused prompt by the product's
	// own estimate; Limit and Requested are the model's window and the
	// refused request's size in the provider's tokens, zero when the
	// provider's message did not say.
	EstimatedTokens int
	Limit           int
	Requested       int
	// Cut is set when the model did not refuse the prompt but cut its reply
	// short (finish reason "length"): the window left too little room, or
	// the reply outgrew the model's own cap, and a smaller part per request
	// is the same remedy.
	Cut bool
}

// PromptRefusals returns the prompts the provider refused during the last
// review, in order; nil when none was refused.
func (r *Reviewer) PromptRefusals() []PromptRefusal { return r.refusals }

// promptRefused is the provider's refusal of one prompt, with the prompt's
// estimated size.
type promptRefused struct {
	refusal PromptRefusal
	err     error
}

func (e *promptRefused) Error() string { return e.err.Error() }
func (e *promptRefused) Unwrap() error { return e.err }

// GenerateReviewWithContext generates a review using the diff and optional
// external evidence such as completed CI check statuses. It runs four
// stages: assemble the redacted prompt, ask the model, parse its reply, and
// pass the parsed findings through the engine's gates.
//
// A diff the model refuses as too long is reviewed in smaller parts (see the
// top of this file), and one that does not fit a prompt the caller bounded is
// reviewed in batches (batches.go): the same four stages per batch, one
// consolidated result.
func (r *Reviewer) GenerateReviewWithContext(ctx context.Context, diff *types.Diff, reviewContext ReviewContext) (*types.ReviewResult, error) {
	r.learnedBudget, r.refusals = 0, nil
	r.contextFits, r.contextReduction = nil, nil
	for {
		result, err := r.reviewOnce(ctx, diff, reviewContext)
		var refused *promptRefused
		if err == nil || !errors.As(err, &refused) {
			return result, r.explainNoRoom(err)
		}
		r.refusals = append(r.refusals, refused.refusal)
		next := learnedCeiling(refused.refusal, r.learnedBudget)
		if next <= 0 {
			return nil, err
		}
		r.learnedBudget = next
	}
}

// reviewOnce is one attempt at the current ceiling: a single prompt, or
// batches when the single prompt left a file out.
func (r *Reviewer) reviewOnce(ctx context.Context, diff *types.Diff, reviewContext ReviewContext) (*types.ReviewResult, error) {
	r.batches = nil
	prepared, err := r.preparePrompt(diff, reviewContext)
	if err != nil {
		// What goes with every file (the evidence of the whole diff, above
		// all) can fill a ceiling that the evidence of one batch's files
		// does not: with several files, the batches get their turn.
		var noRoom *prompt.NoRoomError
		if errors.As(err, &noRoom) && splittable(diff) {
			return r.reviewInBatches(ctx, diff, reviewContext)
		}
		return nil, err
	}
	split, err := r.needsBatches(diff, prepared, reviewContext)
	if err != nil {
		return nil, err
	}
	if split {
		return r.reviewInBatches(ctx, diff, reviewContext)
	}
	return r.reviewPrepared(ctx, prepared)
}

// splittable reports whether the diff has several files with a patch, which
// is what batches divide.
func splittable(diff *types.Diff) bool {
	if diff == nil {
		return false
	}
	withPatch, _ := splitByPatch(diff.Files)
	return len(withPatch) >= 2
}

// refuseCutReply treats a reply the model cut short (finish reason
// "length") as the prompt having left too little room; any other parse error
// passes through.
func refuseCutReply(err error, prepared preparedPrompt) error {
	var parseErr *prompt.ParseError
	if !errors.As(err, &parseErr) || parseErr.FinishReason != "length" {
		return err
	}
	return &promptRefused{refusal: PromptRefusal{EstimatedTokens: metaCount(prepared.parts.Meta, "estimated_tokens"), Cut: true}, err: err}
}

// refuse wraps a provider's refusal of the prompt with the prompt's size;
// any other error passes through.
func refuse(err error, prepared preparedPrompt) error {
	tooLong, ok := llm.AsPromptTooLong(err)
	if !ok {
		return err
	}
	return &promptRefused{
		refusal: PromptRefusal{EstimatedTokens: metaCount(prepared.parts.Meta, "estimated_tokens"), Limit: tooLong.Limit, Requested: tooLong.Requested},
		err:     err,
	}
}

// promptMargin is the share of the model's window a split prompt may take:
// the rest is left for the reply, which the product does not cap.
const promptMargin = 0.9

// learnedCeiling is the prompt ceiling, in the product's estimated tokens, to
// try after a refusal. When the provider said its window and the size it
// received, the ceiling keeps that window's share of the refused prompt (the
// product's estimate and the provider's count differ by content, so the
// ratio is measured on this very prompt); with only the window said, it is
// that window's share; otherwise it halves the refused prompt. It is always
// smaller than the refused prompt and than the previous ceiling, which is
// what ends the loop. Zero means nothing smaller is left.
func learnedCeiling(refusal PromptRefusal, previous int) int {
	next := refusal.EstimatedTokens / 2
	switch {
	case refusal.Limit > 0 && refusal.Requested > refusal.Limit:
		next = int(float64(refusal.EstimatedTokens) * float64(refusal.Limit) / float64(refusal.Requested) * promptMargin)
	case refusal.Limit > 0 && refusal.Requested == 0:
		// The provider said its window but not the size it counted (a
		// request larger than the tokens per minute allowed): the window,
		// with the margin, is the ceiling.
		next = int(float64(refusal.Limit) * promptMargin)
	}
	if next >= refusal.EstimatedTokens {
		next = refusal.EstimatedTokens - 1
	}
	if previous > 0 && next >= previous {
		next = previous / 2
	}
	if next < 0 {
		return 0
	}
	return next
}

// explainNoRoom says what happened when the loop ended because even the
// smallest split leaves no room for a change: the policy and the
// instructions alone are longer than the model accepts.
func (r *Reviewer) explainNoRoom(err error) error {
	var noRoom *prompt.NoRoomError
	if err == nil || len(r.refusals) == 0 || !errors.As(err, &noRoom) {
		return err
	}
	what := "refused a prompt of about %d tokens as too long"
	if r.refusals[0].Cut {
		what = "cut its reply short on a prompt of about %d tokens"
	}
	return fmt.Errorf("the model "+what+", and splitting the review down to %d tokens still leaves no room for a code change: what goes with every part (instructions, policy, context and evidence) takes about %d tokens; shorten the policy or use a model with a larger context window: %w",
		r.refusals[0].EstimatedTokens, noRoom.MaxTokens, noRoom.FixedTokens, err)
}
