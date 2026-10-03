package gate

// ModelOutcome is how the model pass of a review ended. Both review sources
// classify their model call into exactly one of these values; what an
// outcome means for the exit code is declared per source as data
// (NotReviewedRules), never as a second branch of code.
type ModelOutcome int

const (
	// ModelReviewed: the model answered and its answer was parsed.
	ModelReviewed ModelOutcome = iota
	// ModelSkipped: no provider was configured, so only deterministic
	// analysis ran.
	ModelSkipped
	// ModelProviderFailed: the provider never produced an answer (transport
	// failure, unavailable model, a cost-cap refusal), or a quality review
	// the caller required did not happen.
	ModelProviderFailed
	// ModelParseFailed: the model answered but the answer could not be
	// validated, so the run degraded to its deterministic half.
	ModelParseFailed
	// ModelDeliberationLimit: the model's tool conversation exceeded one of
	// its limits (rounds, tokens, tool timeout); nothing it said is a
	// verdict.
	ModelDeliberationLimit
)

// Reason is a machine-readable motive for an inconclusive run. It is what
// the policy-gate status line, the audit record and the SARIF document name.
type Reason string

// The model- and coverage-derived reasons. Scanner reasons (SAST) are
// carried as their own tokens through ReasonInputs.ScannerReason.
const (
	ReasonNone              Reason = ""
	ReasonProviderFailure   Reason = "provider_failure"
	ReasonQualitySkipped    Reason = "quality_skipped"
	ReasonModelParseFailure Reason = "model_parse_failure"
	ReasonDegradedParse     Reason = "degraded_parse"
	ReasonPartialCoverage   Reason = "partial_coverage"
	// ReasonDeliberationLimit is the motive of a deliberation stopped by a
	// limit; DeliberationLimit names which one (deliberation_limit:<limit>).
	ReasonDeliberationLimit Reason = "deliberation_limit"
)

// ReasonInputs is everything the inconclusive ranking reads.
type ReasonInputs struct {
	Model ModelOutcome
	// DegradedParse: the model's reply was accepted through the free-text
	// fallback (prompt.IsDegradedParse).
	DegradedParse bool
	// ScannerReason is the SAST pass's own inconclusive token, empty when it ran.
	ScannerReason string
	// PartialCoverage: part of the diff was never inspected.
	PartialCoverage bool
	// DeliberationLimit names the exceeded limit when Model is
	// ModelDeliberationLimit.
	DeliberationLimit string
}

// RankReason is the one ranking of why a run cannot be trusted. "Did not
// review" outranks "reviewed and found things": a provider that never
// answered outranks a skipped review, which outranks an answer that could
// not be parsed, which outranks the degraded free-text fallback, then the
// SAST pass's own reason (it also reaches the SARIF executionSuccessful
// flag), then partial coverage.
func RankReason(in ReasonInputs) Reason {
	switch {
	case in.Model == ModelDeliberationLimit:
		if in.DeliberationLimit == "" {
			return ReasonDeliberationLimit
		}
		return ReasonDeliberationLimit + Reason(":"+in.DeliberationLimit)
	case in.Model == ModelProviderFailed:
		return ReasonProviderFailure
	case in.Model == ModelSkipped:
		return ReasonQualitySkipped
	case in.Model == ModelParseFailed:
		return ReasonModelParseFailure
	case in.DegradedParse:
		return ReasonDegradedParse
	case in.ScannerReason != "":
		return Reason(in.ScannerReason)
	case in.PartialCoverage:
		return ReasonPartialCoverage
	}
	return ReasonNone
}

// NotReviewedRule says when a model outcome means "this run did not
// review": always, or only when the caller required a quality review
// (--exigir-qualidade).
type NotReviewedRule int

const (
	// NeverNotReviewed: the outcome never closes the run by itself.
	NeverNotReviewed NotReviewedRule = iota
	// NotReviewedWhenRequired: closes the run only when quality was required.
	NotReviewedWhenRequired
	// AlwaysNotReviewed: the outcome always closes the run.
	AlwaysNotReviewed
)

// QualityRequirement says whether the caller required a quality review
// (--exigir-qualidade).
type QualityRequirement int

const (
	// QualityOptional: deterministic analysis alone is an acceptable run.
	QualityOptional QualityRequirement = iota
	// QualityRequired: a run without the model's review is not a review.
	QualityRequired
)

// NotReviewedRules maps each model outcome to its rule. Each review source
// declares its own table; where the two sources differ the difference is
// visible here as data.
type NotReviewedRules map[ModelOutcome]NotReviewedRule

// NotReviewed applies the table to an outcome.
func (r NotReviewedRules) NotReviewed(outcome ModelOutcome, quality QualityRequirement) bool {
	switch r[outcome] {
	case AlwaysNotReviewed:
		return true
	case NotReviewedWhenRequired:
		return quality == QualityRequired
	}
	return false
}
