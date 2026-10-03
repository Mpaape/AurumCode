// AUR-537: on `--pr`, a provider/transport failure that happens while the
// model is actually being called -- every configured provider failed in the
// orchestrator (llm.ErrAllProvidersFailed) or --limite refused the call
// before any provider was ever reached (llm.ErrBudgetExceeded) -- used to
// return exit code 1 immediately, before the AUR-519 policy gate
// (evaluateGate, policygate.go) was ever reached: no aurumcode/policy-gate
// status, `inconclusive: warn` never honored, and no audit/SARIF written at
// all. This left a mandatory check failing closed (acceptable on its own)
// but silently -- a reviewer reading the pull request saw no status and no
// reason, and a policy that explicitly asked to warn-and-continue on this
// exact failure could never do so.
//
// runPRReview (pr.go) now routes that failure through the gate as the
// inconclusive reason gateReasonProviderFailure, exactly once a gate is
// declared (reviewConfig.Gate.Declared()) -- the same token name
// --base's own runReview (main.go) already uses for its own, structurally
// different "provider failed after selection" case, so an operator reading
// either path's published reason sees one vocabulary. Without a gate
// declared, pr.go's behavior is untouched byte-for-byte (AC-003): the same
// diagnosis, the same exit code, no status, no audit, no SARIF -- this file
// adds no new decision there.
//
// This file owns only the one new piece of published text the routed path
// needs that the existing AUR-505 degraded-parse machinery does not already
// supply: providerFailureNotice. Everything else -- the gate's own
// inconclusive-mode decision, the commit status, the audit record, the
// SARIF document, the withheld-approval marker -- is the exact, already
// shipped and tested machinery AUR-519/520/521 built for AUR-505's sibling
// reason (a model answer that failed to parse): this card only supplies the
// trigger, never a second gate or a second writer. See
// docs/specs/AUR-537.md.
package main

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
	if language == "pt-BR" || language == "pt" {
		return "Revisão de qualidade inconclusiva: a revisão não foi executada -- nenhum provedor de modelo respondeu (falha de transporte) ou a chamada foi recusada antes de ser feita (--limite); os achados determinísticos (análise estática e segurança), quando houver, foram publicados."
	}
	return "Quality review inconclusive: the review did not run -- no model provider answered (transport failure) or the call was refused before it was made (--limite); the deterministic findings (static analysis and security), if any, were still published."
}
