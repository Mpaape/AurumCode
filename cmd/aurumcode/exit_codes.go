// Exit codes of `aurumcode review` and the model-failure report that picks one.
package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
)

// exitFindings is the exit code for "the review ran fine and found at least
// one issue at or above the --fail-on threshold". It is distinct from 0
// (clean run), 1 (the review itself failed) and 2 (usage error) so a CI
// pipeline can tell "gate closed" apart from "tool broke". Documented in
// docs/specs/AUR-431.md.
const exitFindings = gateExitFindings

// exitQualityNotReviewed is AUR-458's exit code for "the user asked for a
// quality review and this run did not produce one". It is deliberately 1
// -- the existing taxonomy's "behavioral failure" -- and NOT a new code:
// every other could-not-review path in this command (no provider,
// unavailable --modelo, unparseable response, --limite refusal) already
// returns 1, so a CI job that already treats 1 as "do not merge" needs no
// change to be protected. What changes is that this code now also covers
// the runs where the security pass DID deliver findings, which previously
// reported 0 (nothing configured) or lost the security pass entirely (a
// provider that failed). See docs/specs/AUR-458.md for the full table.
const exitQualityNotReviewed = gateExitBehavioral

// exitArtifactNotWritten is AUR-568's exit code for a run whose requested
// --auditoria or --sarif file could not be written: the compliance trail is
// part of the verdict, so the run never ends as success. Like
// exitQualityNotReviewed it is the existing behavioral-failure code.
const exitArtifactNotWritten = gateExitBehavioral

// reportQualityFailure prints the diagnosis for a quality review that
// could not complete after a provider was successfully selected, and
// returns the exit code for the no---seguranca path. It is the exact
// cascade runReview used to inline, extracted verbatim (AUR-458) so the
// two callers -- the published "return 1 immediately" path and the new
// "keep the security pass alive, then exit 1" path -- can never drift
// into printing different diagnoses for the same failure.
func reportQualityFailure(stderr io.Writer, err error, modelo string, limiteUSD float64) int {
	// --limite (AUR-433): the tracker refused before the model was
	// called, so nothing was spent. This is checked first because it is
	// a distinct, more specific outcome than "the provider chain could
	// not complete" below. Not gated on limiteSet: without --limite,
	// tracker is nil and the orchestrator cannot produce this error, so
	// the check is a no-op then and unconditionally correct whenever it
	// does fire (limiteUSD is 0 only in the case where it cannot).
	if errors.Is(err, llm.ErrBudgetExceeded) {
		return reportBudgetExceeded(stderr, limiteUSD, err)
	}
	var parseErr *prompt.ParseError
	if errors.As(err, &parseErr) {
		fmt.Fprintf(stderr, "aurumcode review: could not understand the model's response (%s)\n", parseErr.Kind)
		return 1
	}
	// With --modelo, a provider chain that could not complete means the
	// chosen model is unavailable (endpoint down, wrong URL, model not
	// served): name the model and say how to fix it, instead of only
	// surfacing the transport error.
	if modelo != "" && errors.Is(err, llm.ErrAllProvidersFailed) {
		return reportModelUnavailable(stderr, modelo, err)
	}
	fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
	return 1
}
