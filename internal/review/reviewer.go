package review

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/llm"
	"aurumcode/internal/prompt"
	"aurumcode/internal/review/iso25010"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"path/filepath"
)

// Reviewer orchestrates the code review process
type Reviewer struct {
	orchestrator  *llm.Orchestrator
	diffAnalyzer  *analyzer.DiffAnalyzer
	promptBuilder *prompt.PromptBuilder
	parser        *prompt.ResponseParser
	rulesLoader   *RulesLoader
	isoConfig     *iso25010.Config
	scorer        *iso25010.Scorer
}

// Config holds reviewer configuration
type Config struct {
	RulesDir       string
	ISOConfigPath  string
	MaxTokens      int
	Temperature    float64
}

// NewReviewer creates a new reviewer with configuration
func NewReviewer(orchestrator *llm.Orchestrator, cfg Config) (*Reviewer, error) {
	// Load rules
	rulesLoader := NewRulesLoader(cfg.RulesDir)
	if err := rulesLoader.Load(); err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}

	// Load ISO/IEC 25010 configuration
	isoConfig, err := iso25010.LoadConfig(cfg.ISOConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load ISO config: %w", err)
	}

	// Create scorer
	scorer := iso25010.NewScorer(isoConfig)

	return &Reviewer{
		orchestrator:  orchestrator,
		diffAnalyzer:  analyzer.NewDiffAnalyzer(),
		promptBuilder: prompt.NewPromptBuilder(),
		parser:        prompt.NewResponseParser(),
		rulesLoader:   rulesLoader,
		isoConfig:     isoConfig,
		scorer:        scorer,
	}, nil
}

// GenerateReview generates a comprehensive code review
func (r *Reviewer) GenerateReview(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
	// Analyze diff
	metrics := r.diffAnalyzer.AnalyzeDiff(diff)

	// Build prompt with token budgeting
	opts := prompt.BuildOptions{
		MaxTokens:    4000,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: 1000,
	}

	promptParts, err := r.promptBuilder.BuildPrompt(diff, metrics, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Combine system and user prompts
	fullPrompt := promptParts.System + "\n\n" + promptParts.User

	// Call LLM
	resp, err := r.orchestrator.Complete(ctx, fullPrompt, llm.Options{
		MaxTokens:   opts.MaxTokens - opts.ReserveReply,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	// Parse response
	result, err := r.parser.ParseReviewResponse(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	// Map findings to rules
	r.mapRulesToIssues(result)

	// Compute ISO/IEC 25010 scores
	isoScores := r.scorer.Score(result, metrics)
	result.ISOScores = isoScores

	// Add metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["total_files"] = fmt.Sprintf("%d", metrics.TotalFiles)
	result.Metadata["lines_added"] = fmt.Sprintf("%d", metrics.LinesAdded)
	result.Metadata["lines_deleted"] = fmt.Sprintf("%d", metrics.LinesDeleted)
	result.Metadata["segments_used"] = promptParts.Meta["segments_used"]
	result.Metadata["estimated_tokens"] = promptParts.Meta["estimated_tokens"]

	return result, nil
}

// mapRulesToIssues enriches issues with rule metadata
func (r *Reviewer) mapRulesToIssues(result *types.ReviewResult) {
	for i := range result.Issues {
		issue := &result.Issues[i]

		// Try to get rule details
		if rule, ok := r.rulesLoader.Get(issue.RuleID); ok {
			// Enrich with rule metadata if needed
			if issue.Message == "" {
				issue.Message = rule.Description
			}

			// Normalize severity
			if issue.Severity == "" {
				issue.Severity = rule.Severity
			}
		}

		// Normalize file paths
		if issue.File != "" {
			issue.File = filepath.Clean(issue.File)
		}
	}
}

// GetRules returns all loaded rules
func (r *Reviewer) GetRules() []Rule {
	return r.rulesLoader.GetAll()
}

// GetRulesByCategory returns rules for a specific category
func (r *Reviewer) GetRulesByCategory(category string) []Rule {
	return r.rulesLoader.GetByCategory(category)
}
