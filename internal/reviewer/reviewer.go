package reviewer

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/llm"
	"aurumcode/internal/prompt"
	"aurumcode/pkg/types"
	"context"
	"fmt"
)

type Reviewer struct {
	orchestrator *llm.Orchestrator
	diffAnalyzer *analyzer.DiffAnalyzer
	promptBuilder *prompt.PromptBuilder
	parser *prompt.ResponseParser
}

func NewReviewer(orchestrator *llm.Orchestrator) *Reviewer {
	return &Reviewer{
		orchestrator: orchestrator,
		diffAnalyzer: analyzer.NewDiffAnalyzer(),
		promptBuilder: prompt.NewPromptBuilder(),
		parser: prompt.NewResponseParser(),
	}
}

func (r *Reviewer) Review(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
	metrics := r.diffAnalyzer.AnalyzeDiff(diff)
	reviewPrompt := r.promptBuilder.BuildReviewPrompt(diff, metrics)

	resp, err := r.orchestrator.Complete(ctx, reviewPrompt, llm.Options{
		MaxTokens: 4000,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	result, err := r.parser.ParseReviewResponse(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	totalTokens := resp.TokensIn + resp.TokensOut
	result.Cost = types.CostSummary{
		Tokens: totalTokens,
		CostUSD: float64(totalTokens) * 0.0001,
		Provider: "llm",
		Model: resp.Model,
	}

	return result, nil
}
