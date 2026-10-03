// Provider failure on --pr: when no model provider answers, or --limite
// refuses the call, the review still reaches the gate and publishes the
// deterministic findings, declaring that the quality review did not run.
package main

import "github.com/Mpaape/AurumCode/internal/i18n"

// gateReasonProviderFailure is AUR-537's own token for
// evaluateGate/writeComplianceArtifacts' inconclusiveReason parameter: a
// genuine provider/transport failure (llm.ErrAllProvidersFailed) or a
// pre-call --limite refusal (llm.ErrBudgetExceeded) reaches the model call
// in runPRReview, not a response the model actually returned. It is a
// distinct string from qualityDegraded's own "model_parse_failure" (the
// model DID answer, but the parser could not validate the response) and
// from AUR-476's "partial_coverage" -- three different, stable,
// non-model-authored reasons the published policy-gate status and the
// compliance audit record/SARIF document can name. It is the exact literal
// --base's own runReview (main.go) already publishes for its own
// "provider configured and failed at runtime" case, so both paths speak the
// same vocabulary for what is, from an auditor's point of view, the same
// kind of failure.
const gateReasonProviderFailure = "provider_failure"

// providerFailureNotice is AUR-537's declared limitation for a run whose
// quality half never produced a model answer at all: the --limite tracker
// refused the call before any provider was reached, or every configured
// provider failed the transport call itself. It mirrors
// modelInvalidOutputNotice's (passes.go) shape and placement -- a
// Limitations entry, never a finding -- but names a different, earlier
// failure: AUR-505's own notice means the model DID answer and the parser
// rejected it, while this one means the model was never successfully
// reached at all. Both set result.Metadata["quality_degraded"] = "true" so
// the published verdict, formal review event and summary text already
// never claim approval (reviewVerdictForLanguage/formalReviewEvent/
// reviewSummaryTextForLanguage, pr.go) -- this notice is the one piece of
// published text specific to THIS failure that those shared paths do not
// already carry, so the review body also says in its own words that the
// review did not run, not only that it is inconclusive.
func providerFailureNotice(language string) string {
	return i18n.Text(language, "notice.provider_failure")
}
