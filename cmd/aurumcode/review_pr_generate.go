// The --pr model phase: one provider-backed review of the pull request diff,
// with the transport-failure and budget branches that keep a declared gate
// from being skipped.
package main

import (
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// generateReview calls the model. An unparseable answer degrades to the
// deterministic half (AUR-505, never a crash); a provider/transport failure
// returns immediately unless a gate is declared, in which case it falls
// through as the gate's own inconclusive reason (AUR-537, AC-003).
func (p *prReview) generateReview() (int, bool) {
	// stderr, limiteUSD and gateDeclared stay named as in the original
	// function: tests/acceptance/AUR-537.sh anchors its mutations on them.
	stderr, limiteUSD := p.stderr, p.limiteUSD
	gateDeclared := p.cfg.Gate.Declared()
	result, err := p.reviewer.GenerateReviewWithContext(p.ctx, p.diff, p.reviewContext(review.ReviewContext{
		CI:              readCIContext(),
		Language:        p.reviewLanguage,
		History:         p.history,
		CodebaseContext: p.codebaseText,
		MemoryNotes:     p.memoryNotesText,
	}))
	p.result = result
	p.transcript = p.reviewer.Transcript()
	if err == nil {
		return 0, false
	}
	if code, failed := reportDeliberationFailure(stderr, err); failed {
		p.reportDeliberation()
		return code, true
	}
	var parseErr *prompt.ParseError
	switch {
	case errors.Is(err, llm.ErrBudgetExceeded):
		// --limite: nothing was spent; checked first, like --base.
		rc := reportBudgetExceeded(stderr, limiteUSD, err)
		if !gateDeclared {
			return rc, true
		}
		p.model = modelProviderFailed
	case errors.As(err, &parseErr):
		p.degradeUnparseable(parseErr)
	case errors.Is(err, llm.ErrAllProvidersFailed):
		var rc int
		if p.opts.modelo != "" {
			rc = reportModelUnavailable(stderr, p.opts.modelo, err)
		} else {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			rc = 1
		}
		if !gateDeclared {
			return rc, true
		}
		p.model = modelProviderFailed
	default:
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	if p.model == modelProviderFailed {
		// Reuses the "quality_degraded" key on purpose: the shared verdict
		// rendering already turns it into "never approve".
		p.result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}}
		p.result.Limitations = append(p.result.Limitations, providerFailureNotice(p.reviewLanguage))
	}
	return 0, false
}
