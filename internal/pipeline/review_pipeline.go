package pipeline

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/config"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/internal/prompt"
	"aurumcode/internal/reviewer"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"log"
)

// ReviewPipeline handles the code review use case
type ReviewPipeline struct {
	config       *config.Config
	githubClient *githubclient.Client
	reviewer     *reviewer.Reviewer
	analyzer     *analyzer.DiffAnalyzer
}

// NewReviewPipeline creates a new review pipeline
func NewReviewPipeline(
	cfg *config.Config,
	githubClient *githubclient.Client,
	llmOrch *llm.Orchestrator,
) *ReviewPipeline {
	return &ReviewPipeline{
		config:       cfg,
		githubClient: githubClient,
		reviewer:     reviewer.NewReviewer(llmOrch),
		analyzer:     analyzer.NewDiffAnalyzer(),
	}
}

// Run executes the code review pipeline
func (p *ReviewPipeline) Run(ctx context.Context, event *types.Event) error {
	// 1. Fetch PR diff from GitHub
	log.Printf("[Review] Fetching diff for PR #%d", event.PRNumber)
	diff, err := p.githubClient.GetPullRequestDiff(
		ctx,
		event.Repo,
		event.RepoOwner,
		event.PRNumber,
	)
	if err != nil {
		return fmt.Errorf("fetch diff: %w", err)
	}

	// 2. Analyze diff
	log.Printf("[Review] Analyzing diff: %d files", len(diff.Files))
	metrics := p.analyzer.AnalyzeDiff(diff)
	log.Printf("[Review] Metrics: %d lines added, %d deleted, %d languages",
		metrics.LinesAdded, metrics.LinesDeleted, len(metrics.LanguageBreakdown))

	// 3. Perform code review
	log.Printf("[Review] Running AI code review")
	review, err := p.reviewer.Review(ctx, diff)
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}

	log.Printf("[Review] Found %d issues (%.2f score)", len(review.Issues), review.OverallScore)

	// 4. Post review comments to GitHub
	if len(review.Issues) > 0 {
		log.Printf("[Review] Posting %d comments to PR", len(review.Issues))
		for _, issue := range review.Issues {
			comment := types.ReviewComment{
				Path:     issue.File,
				Line:     issue.Line,
				Body:     p.formatIssueComment(issue),
				CommitID: event.CommitSHA,
			}

			if err := p.githubClient.PostReviewComment(
				ctx,
				event.Repo,
				event.RepoOwner,
				event.PRNumber,
				comment,
			); err != nil {
				log.Printf("[Review] Failed to post comment: %v", err)
				// Continue posting other comments even if one fails
			}
		}
	}

	// 5. Set commit status
	status := "success"
	description := fmt.Sprintf("Review complete: %d issues found", len(review.Issues))

	// Fail status if there are errors (not warnings)
	errorCount := 0
	for _, issue := range review.Issues {
		if issue.Severity == "error" {
			errorCount++
		}
	}

	if errorCount > 0 {
		status = "failure"
		description = fmt.Sprintf("Review failed: %d errors, %d total issues", errorCount, len(review.Issues))
	}

	log.Printf("[Review] Setting commit status: %s", status)
	if err := p.githubClient.SetStatus(
		ctx,
		event.Repo,
		event.RepoOwner,
		event.CommitSHA,
		status,
		description,
	); err != nil {
		return fmt.Errorf("set status: %w", err)
	}

	// 6. Post summary comment
	summary := p.formatSummaryComment(review, metrics)
	summaryComment := types.ReviewComment{
		Path:     "", // Empty path = general PR comment
		Line:     0,
		Body:     summary,
		CommitID: event.CommitSHA,
	}

	if err := p.githubClient.PostReviewComment(
		ctx,
		event.Repo,
		event.RepoOwner,
		event.PRNumber,
		summaryComment,
	); err != nil {
		log.Printf("[Review] Failed to post summary: %v", err)
	}

	log.Printf("[Review] Pipeline completed successfully")
	return nil
}

// formatIssueComment formats an issue as a GitHub comment
func (p *ReviewPipeline) formatIssueComment(issue types.Issue) string {
	severityEmoji := map[string]string{
		"error":   "🔴",
		"warning": "⚠️",
		"info":    "ℹ️",
	}

	emoji := severityEmoji[issue.Severity]
	if emoji == "" {
		emoji = "•"
	}

	comment := fmt.Sprintf("%s **%s** `%s`\n\n", emoji, issue.Severity, issue.RuleID)
	comment += fmt.Sprintf("%s\n\n", issue.Message)

	if issue.Suggestion != "" {
		comment += fmt.Sprintf("**Suggestion:**\n%s\n", issue.Suggestion)
	}

	return comment
}

// formatSummaryComment formats the review summary
func (p *ReviewPipeline) formatSummaryComment(review *types.ReviewResult, metrics *analyzer.DiffMetrics) string {
	summary := "## 🤖 AurumCode Review Summary\n\n"

	// Issues breakdown
	errorCount := 0
	warningCount := 0
	infoCount := 0
	for _, issue := range review.Issues {
		switch issue.Severity {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "info":
			infoCount++
		}
	}

	summary += fmt.Sprintf("**Issues Found:** %d total\n", len(review.Issues))
	if errorCount > 0 {
		summary += fmt.Sprintf("- 🔴 %d errors\n", errorCount)
	}
	if warningCount > 0 {
		summary += fmt.Sprintf("- ⚠️ %d warnings\n", warningCount)
	}
	if infoCount > 0 {
		summary += fmt.Sprintf("- ℹ️ %d info\n", infoCount)
	}

	// Diff metrics
	summary += fmt.Sprintf("\n**Changes:**\n")
	summary += fmt.Sprintf("- %d files changed\n", metrics.TotalFiles)
	summary += fmt.Sprintf("- +%d / -%d lines\n", metrics.LinesAdded, metrics.LinesDeleted)

	// ISO scores (if available)
	if review.ISOScores != nil {
		summary += fmt.Sprintf("\n**ISO/IEC 25010 Scores:**\n")
		summary += fmt.Sprintf("- Functionality: %.1f/10\n", review.ISOScores.Functionality)
		summary += fmt.Sprintf("- Reliability: %.1f/10\n", review.ISOScores.Reliability)
		summary += fmt.Sprintf("- Security: %.1f/10\n", review.ISOScores.Security)
		summary += fmt.Sprintf("- Maintainability: %.1f/10\n", review.ISOScores.Maintainability)
	}

	// Cost
	if review.Cost.Tokens > 0 {
		summary += fmt.Sprintf("\n**Cost:** $%.4f (%d tokens)\n", review.Cost.CostUSD, review.Cost.Tokens)
	}

	summary += fmt.Sprintf("\n_Generated by [AurumCode](https://github.com/yourusername/aurumcode)_\n")

	return summary
}
