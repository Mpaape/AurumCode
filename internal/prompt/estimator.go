package prompt

import "github.com/Mpaape/AurumCode/internal/llm/tokens"

// HeuristicEstimator is the production TokenEstimator: the engine's one
// character heuristic (tokens.Estimate), the same internal/llm falls
// back to when a provider cannot count its own tokens, so the two budgets
// agree.
type HeuristicEstimator struct{}

// NewHeuristicEstimator creates the default production TokenEstimator.
func NewHeuristicEstimator() *HeuristicEstimator {
	return &HeuristicEstimator{}
}

// Estimate is tokens.Estimate.
func (e *HeuristicEstimator) Estimate(text string) int {
	return tokens.Estimate(text)
}
