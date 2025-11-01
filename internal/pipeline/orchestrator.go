package pipeline

import (
	"aurumcode/internal/config"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"log"
	"sync"
)

// MainOrchestrator coordinates all three use case pipelines
type MainOrchestrator struct {
	config       *config.Config
	githubClient *githubclient.Client
	llmOrch      *llm.Orchestrator

	// Three main pipelines
	reviewPipeline *ReviewPipeline
	docsPipeline   *DocumentationPipeline
	qaPipeline     *QATestingPipeline
}

// NewMainOrchestrator creates a new main orchestrator
func NewMainOrchestrator(
	cfg *config.Config,
	githubClient *githubclient.Client,
	llmOrch *llm.Orchestrator,
) *MainOrchestrator {
	return &MainOrchestrator{
		config:       cfg,
		githubClient: githubClient,
		llmOrch:      llmOrch,
		reviewPipeline:   NewReviewPipeline(cfg, githubClient, llmOrch),
		docsPipeline:     NewDocumentationPipeline(cfg, githubClient, llmOrch),
		qaPipeline:       NewQATestingPipeline(cfg, githubClient, llmOrch),
	}
}

// ProcessEvent processes a GitHub event and runs appropriate pipelines
func (o *MainOrchestrator) ProcessEvent(ctx context.Context, event *types.Event) error {
	log.Printf("[Pipeline] Processing event: type=%s repo=%s pr=%d", event.EventType, event.Repo, event.PRNumber)

	// Determine which pipelines to run based on configuration and event type
	var wg sync.WaitGroup
	errs := make(chan error, 3)

	// Pipeline #1: Code Review
	if o.shouldRunReview(event) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("[Pipeline] Running Code Review pipeline")
			if err := o.reviewPipeline.Run(ctx, event); err != nil {
				log.Printf("[Pipeline] Code Review failed: %v", err)
				errs <- fmt.Errorf("review pipeline: %w", err)
			} else {
				log.Printf("[Pipeline] Code Review completed successfully")
			}
		}()
	}

	// Pipeline #2: Documentation Generation
	if o.shouldRunDocs(event) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("[Pipeline] Running Documentation pipeline")
			if err := o.docsPipeline.Run(ctx, event); err != nil {
				log.Printf("[Pipeline] Documentation failed: %v", err)
				errs <- fmt.Errorf("docs pipeline: %w", err)
			} else {
				log.Printf("[Pipeline] Documentation completed successfully")
			}
		}()
	}

	// Pipeline #3: QA Testing
	if o.shouldRunQA(event) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("[Pipeline] Running QA Testing pipeline")
			if err := o.qaPipeline.Run(ctx, event); err != nil {
				log.Printf("[Pipeline] QA Testing failed: %v", err)
				errs <- fmt.Errorf("qa pipeline: %w", err)
			} else {
				log.Printf("[Pipeline] QA Testing completed successfully")
			}
		}()
	}

	// Wait for all pipelines to complete
	wg.Wait()
	close(errs)

	// Collect errors
	var allErrs []error
	for err := range errs {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) > 0 {
		return fmt.Errorf("pipeline errors: %v", allErrs)
	}

	log.Printf("[Pipeline] All pipelines completed successfully")
	return nil
}

// shouldRunReview determines if code review pipeline should run
func (o *MainOrchestrator) shouldRunReview(event *types.Event) bool {
	// Check if code review is enabled
	if !o.config.Features.CodeReview {
		return false
	}

	// Code review runs on pull_request events
	if event.EventType == "pull_request" {
		return true
	}

	// Also run on push events if configured
	if event.EventType == "push" && o.config.Features.CodeReviewOnPush {
		return true
	}

	return false
}

// shouldRunDocs determines if documentation pipeline should run
func (o *MainOrchestrator) shouldRunDocs(event *types.Event) bool {
	// Check if documentation is enabled
	if !o.config.Features.Documentation {
		return false
	}

	// Documentation runs on push to main branch (after merge)
	if event.EventType == "push" && event.Branch == "main" {
		return true
	}

	// Also run on merged pull requests if configured
	if event.EventType == "pull_request" && event.Action == "closed" && event.Merged {
		return true
	}

	return false
}

// shouldRunQA determines if QA testing pipeline should run
func (o *MainOrchestrator) shouldRunQA(event *types.Event) bool {
	// Check if QA testing is enabled
	if !o.config.Features.QATesting {
		return false
	}

	// QA testing runs on pull_request events
	if event.EventType == "pull_request" && event.Action != "closed" {
		return true
	}

	return false
}
