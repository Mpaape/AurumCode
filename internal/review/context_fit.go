package review

import (
	"errors"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/llm/tokens"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The codebase context is support for the review, not what is reviewed.
// Once a provider has refused a prompt for its length (split.go), the
// context goes with every part of the split review, and a context larger
// than the window would leave no room for any change: the review would end
// in NoRoom however small the parts. So, under a learned ceiling, the
// context takes at most half of what the instructions, policy and evidence
// leave, the other half stays for the code under review (which is split in
// batches, never reduced), and the context is reduced to that share by
// codebasectx.Fit. When the change still does not fit, the share halves
// again, down to no context at all. Every reduction is declared: in the
// pack itself, for the model, and in ContextReduction, for the terminal,
// the result and the audit. Without a refusal the context goes whole.

// ContextReduction is how the codebase context was reduced to fit the
// learned window: what each part kept, and the context's size before and
// after, in the product's estimated tokens.
type ContextReduction struct {
	codebasectx.Reduction
	OriginalTokens int
	KeptTokens     int
}

// ContextReduction returns the largest reduction of the codebase context in
// the last review, false when the context went whole.
func (r *Reviewer) ContextReduction() (ContextReduction, bool) {
	if r.contextReduction == nil {
		return ContextReduction{}, false
	}
	return *r.contextReduction, true
}

// contextFitKey names one reduction: the context and the share it got.
type contextFitKey struct {
	text      string
	allowance int
}

// contextFit is one reduction already computed.
type contextFit struct {
	text      string
	reduction codebasectx.Reduction
}

// prepareWithinWindow is preparePrompt under a learned ceiling with a
// codebase context: the context is reduced to its share (see the top of
// this file) and the prompt assembled with it.
func (r *Reviewer) prepareWithinWindow(diff *types.Diff, reviewContext ReviewContext) (preparedPrompt, error) {
	bare := reviewContext
	bare.CodebaseContext = ""
	base, err := r.assemblePrompt(&types.Diff{}, bare)
	if err != nil {
		// The instructions, policy and evidence alone leave no room: no
		// reduction of the context helps, and explainNoRoom says so.
		return preparedPrompt{}, err
	}
	allowance := (r.promptBudget() - metaCount(base.parts.Meta, "estimated_tokens")) / 2
	for {
		text, reduction := r.fitContext(reviewContext.CodebaseContext, allowance)
		withContext := reviewContext
		withContext.CodebaseContext = text
		prepared, err := r.assemblePrompt(diff, withContext)
		var noRoom *prompt.NoRoomError
		if errors.As(err, &noRoom) && text != "" {
			allowance /= 2
			continue
		}
		if err == nil {
			r.noteContextReduction(reviewContext.CodebaseContext, text, reduction)
		}
		return prepared, err
	}
}

// fitContext reduces text to allowance tokens, once per text and share: a
// split review prepares the same prompt many times while it plans its
// batches.
func (r *Reviewer) fitContext(text string, allowance int) (string, codebasectx.Reduction) {
	if allowance < 0 {
		allowance = 0
	}
	key := contextFitKey{text: text, allowance: allowance}
	if fit, ok := r.contextFits[key]; ok {
		return fit.text, fit.reduction
	}
	fitted, reduction := codebasectx.Fit(text, allowance*charsPerEstimatedToken)
	if r.contextFits == nil {
		r.contextFits = map[contextFitKey]contextFit{}
	}
	r.contextFits[key] = contextFit{text: fitted, reduction: reduction}
	return fitted, reduction
}

// charsPerEstimatedToken converts the product's token estimate to bytes,
// the unit codebasectx.Fit measures in.
const charsPerEstimatedToken = 4

// noteContextReduction keeps the largest reduction of the review: the one
// of the whole diff's context when the batches carry smaller ones.
func (r *Reviewer) noteContextReduction(original, kept string, reduction codebasectx.Reduction) {
	if !reduction.Reduced() {
		return
	}
	next := ContextReduction{Reduction: reduction, OriginalTokens: tokens.Estimate(original), KeptTokens: tokens.Estimate(kept)}
	if r.contextReduction == nil || next.OriginalTokens > r.contextReduction.OriginalTokens ||
		(next.OriginalTokens == r.contextReduction.OriginalTokens && next.KeptTokens < r.contextReduction.KeptTokens) {
		r.contextReduction = &next
	}
}
