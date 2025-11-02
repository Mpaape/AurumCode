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

	// 4. Post detailed line-by-line comments
	if len(review.LineComments) > 0 {
		log.Printf("[Review] Posting %d line comments to PR", len(review.LineComments))
		for _, lineComment := range review.LineComments {
			ghComment := githubclient.ReviewComment{
				Body:     lineComment.Body,
				Path:     lineComment.Path,
				CommitID: event.CommitSHA,
				Line:     lineComment.Line,
			}

			if err := p.githubClient.PostReviewComment(
				ctx,
				event.RepoOwner,
				event.Repo,
				event.PRNumber,
				ghComment,
				"", // no idempotency key
			); err != nil {
				log.Printf("[Review] Failed to post line comment: %v", err)
				// Continue posting other comments even if one fails
			}
		}
	}

	// 5. Post file-level summary comments
	if len(review.FileComments) > 0 {
		log.Printf("[Review] Posting %d file summary comments to PR", len(review.FileComments))
		for _, fileComment := range review.FileComments {
			ghComment := githubclient.ReviewComment{
				Body:     fileComment.Body,
				Path:     fileComment.Path,
				CommitID: event.CommitSHA,
				Line:     0, // File-level comment (no specific line)
			}

			if err := p.githubClient.PostReviewComment(
				ctx,
				event.RepoOwner,
				event.Repo,
				event.PRNumber,
				ghComment,
				"", // no idempotency key
			); err != nil {
				log.Printf("[Review] Failed to post file comment: %v", err)
			}
		}
	}

	// 6. Post commit-level overall summary
	if review.CommitComment != "" {
		log.Printf("[Review] Posting commit-level summary comment")
		if err := p.githubClient.PostIssueComment(
			ctx,
			event.RepoOwner,
			event.Repo,
			event.PRNumber,
			review.CommitComment,
		); err != nil {
			log.Printf("[Review] Failed to post commit comment: %v", err)
		}
	}

	// 7. Set commit status
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
	commitStatus := githubclient.CommitStatus{
		State:       status,
		Description: description,
		Context:     "aurumcode/review",
	}

	if err := p.githubClient.SetStatus(
		ctx,
		event.RepoOwner,
		event.Repo,
		event.CommitSHA,
		commitStatus,
	); err != nil {
		return fmt.Errorf("set status: %w", err)
	}

	log.Printf("[Review] Pipeline completed successfully")
	log.Printf("[Review] Posted %d line comments, %d file comments, and commit summary",
		len(review.LineComments), len(review.FileComments))
	return nil
}

