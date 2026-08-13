package prompt

// HeuristicEstimator is the production TokenEstimator. builder.go in the
// restored c12d7ab engine constructed its default estimator as
// &StubEstimator{}, but StubEstimator only ever existed in types_test.go --
// it never compiled into a production binary (see AUR-430's restoration
// audit). This type is that estimator, promoted to production: the same
// ~4-characters-per-token heuristic already used, unrelated to any vendor,
// by internal/llm/estimator.go's heuristicTokens fallback. Keeping the same
// ratio here means a budget computed by this package roughly agrees with
// the one internal/llm falls back to when a provider cannot report its own
// token count.
type HeuristicEstimator struct{}

// NewHeuristicEstimator creates the default production TokenEstimator.
func NewHeuristicEstimator() *HeuristicEstimator {
	return &HeuristicEstimator{}
}

// Estimate approximates token count at ~4 characters per token, never
// returning zero for non-empty input so a non-empty segment can never be
// budgeted as free.
func (e *HeuristicEstimator) Estimate(text string) int {
	if text == "" {
		return 0
	}
	if count := len(text) / 4; count > 0 {
		return count
	}
	return 1
}
