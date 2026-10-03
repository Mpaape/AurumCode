package prompt

import "github.com/Mpaape/AurumCode/pkg/types"

// HeuristicEstimator is the production TokenEstimator: the engine's one
// character heuristic (types.EstimateTokens), the same internal/llm falls
// back to when a provider cannot count its own tokens, so the two budgets
// agree.
type HeuristicEstimator struct{}

// NewHeuristicEstimator creates the default production TokenEstimator.
func NewHeuristicEstimator() *HeuristicEstimator {
	return &HeuristicEstimator{}
}

// Estimate is types.EstimateTokens.
func (e *HeuristicEstimator) Estimate(text string) int {
	return types.EstimateTokens(text)
}
