package review

import (
	"errors"
	"strings"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/llm/tokens"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The codebase context and the evidence are support for the review, not what is reviewed.
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
	// Evidence and EvidenceKept count the deterministic evidence items of
	// the most reduced prompt and how many it carried, both zero when the
	// evidence went whole.
	Evidence, EvidenceKept int
}

// Reduced reports whether the context or the evidence was reduced.
func (c ContextReduction) Reduced() bool {
	return c.Reduction.Reduced() || c.EvidenceKept < c.Evidence
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

// prepareWithinWindow is preparePrompt under a learned ceiling: the
// evidence is bounded when, with the instructions and policy, it alone
// fills the window, and the codebase context is reduced to its share (see
// the top of this file) before the prompt is assembled.
func (r *Reviewer) prepareWithinWindow(diff *types.Diff, reviewContext ReviewContext) (preparedPrompt, error) {
	reviewContext, base, err := r.fitEvidence(reviewContext)
	if err != nil {
		// The instructions and policy alone leave no room: nothing that
		// supports the review can be reduced to help, and explainNoRoom
		// says so.
		return preparedPrompt{}, err
	}
	allowance := (r.promptBudget() - metaCount(base.parts.Meta, "estimated_tokens")) / 2
	noteFits := true
	for {
		text, reduction := r.fitContext(reviewContext.CodebaseContext, allowance)
		if text == "" && reduction.Discarded && noteFits {
			// Left out whole, the context still says so to the model, as a
			// partial reduction does in the pack.
			text = omittedContextNote
		}
		withContext := reviewContext
		withContext.CodebaseContext = text
		prepared, err := r.assemblePrompt(diff, withContext)
		var noRoom *prompt.NoRoomError
		if errors.As(err, &noRoom) && text == omittedContextNote {
			noteFits = false
			continue
		}
		if errors.As(err, &noRoom) && text != "" {
			allowance /= 2
			continue
		}
		if err == nil {
			r.noteContextReduction(reviewContext, text, reduction, prepared)
		}
		return prepared, err
	}
}

// fitEvidence assembles what goes with every part, without a change and
// without the codebase context. When the evidence tied to no file makes that
// alone longer than the window, the evidence gets half of what the instructions and
// policy leave, halving again while it still does not fit; the items left
// out are counted in the prompt and never reach EvidenceAdmittedMetaKey, so
// the model's silence on them is never read as a verdict.
func (r *Reviewer) fitEvidence(reviewContext ReviewContext) (ReviewContext, preparedPrompt, error) {
	bare := reviewContext
	bare.CodebaseContext = ""
	base, err := r.assemblePrompt(&types.Diff{}, bare)
	var noRoom *prompt.NoRoomError
	if !errors.As(err, &noRoom) {
		return reviewContext, base, err
	}
	// Evidence tied to a file travels with that file's batch (batches.go),
	// which already divides it; only the evidence tied to no file goes with
	// every part, and only when it alone fills the window is it bounded.
	fileless := bare
	fileless.Evidence = nil
	for _, item := range bare.Evidence {
		if item.File == "" {
			fileless.Evidence = append(fileless.Evidence, item)
		}
	}
	if len(fileless.Evidence) == 0 {
		return reviewContext, base, err
	}
	if _, filelessErr := r.assemblePrompt(&types.Diff{}, fileless); !errors.As(filelessErr, &noRoom) {
		return reviewContext, base, err
	}
	none := bare
	none.Evidence = nil
	fixed, err := r.assemblePrompt(&types.Diff{}, none)
	if err != nil {
		return reviewContext, preparedPrompt{}, err
	}
	allowance := (r.promptBudget() - metaCount(fixed.parts.Meta, "estimated_tokens")) / 2
	for {
		if allowance < 1 {
			allowance = 1
		}
		bare.evidenceMaxTokens = allowance
		base, err = r.assemblePrompt(&types.Diff{}, bare)
		if errors.As(err, &noRoom) && allowance > 1 {
			allowance /= 2
			continue
		}
		reviewContext.evidenceMaxTokens = allowance
		return reviewContext, base, err
	}
}

// omittedContextNote takes the codebase context's place when it was left
// out whole to fit the window.
const omittedContextNote = "codebase context omitted to fit the model's context window"

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
func (r *Reviewer) noteContextReduction(reviewContext ReviewContext, kept string, reduction codebasectx.Reduction, prepared preparedPrompt) {
	next := ContextReduction{Reduction: reduction, OriginalTokens: tokens.Estimate(reviewContext.CodebaseContext), KeptTokens: tokens.Estimate(kept)}
	if reviewContext.evidenceMaxTokens > 0 {
		next.Evidence = len(reviewContext.Evidence)
		next.EvidenceKept = admittedCount(prepared.parts.Meta[prompt.EvidenceAdmittedMetaKey])
	}
	if !next.Reduced() {
		return
	}
	if r.contextReduction == nil {
		r.contextReduction = &next
		return
	}
	current := *r.contextReduction
	if next.Reduction.Reduced() && (!current.Reduction.Reduced() || next.OriginalTokens > current.OriginalTokens ||
		(next.OriginalTokens == current.OriginalTokens && next.KeptTokens < current.KeptTokens)) {
		current.Reduction, current.OriginalTokens, current.KeptTokens = next.Reduction, next.OriginalTokens, next.KeptTokens
	}
	if next.Evidence-next.EvidenceKept > current.Evidence-current.EvidenceKept {
		current.Evidence, current.EvidenceKept = next.Evidence, next.EvidenceKept
	}
	r.contextReduction = &current
}

// admittedCount counts the comma-separated ids of EvidenceAdmittedMetaKey.
func admittedCount(list string) int {
	if list == "" {
		return 0
	}
	return len(strings.Split(list, ","))
}
