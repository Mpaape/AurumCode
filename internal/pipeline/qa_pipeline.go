package pipeline

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/config"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/internal/testgen"
	"aurumcode/internal/testing/executor"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// QATestingPipeline handles the QA testing use case
type QATestingPipeline struct {
	config       *config.Config
	githubClient *githubclient.Client
	llmOrch      *llm.Orchestrator
	testGen      *testgen.Generator
	diffAnalyzer *analyzer.DiffAnalyzer
	executors    map[executor.Language]executor.Executor
}

// NewQATestingPipeline creates a new QA testing pipeline
func NewQATestingPipeline(
	cfg *config.Config,
	githubClient *githubclient.Client,
	llmOrch *llm.Orchestrator,
) *QATestingPipeline {
	return &QATestingPipeline{
		config:       cfg,
		githubClient: githubClient,
		llmOrch:      llmOrch,
		testGen:      testgen.NewGenerator(llmOrch),
		diffAnalyzer: analyzer.NewDiffAnalyzer(),
		executors: map[executor.Language]executor.Executor{
			executor.LanguageGo:         executor.NewGoExecutor(),
			executor.LanguagePython:     executor.NewPythonExecutor(),
			executor.LanguageJavaScript: executor.NewJSExecutor(),
		},
	}
}

// Run executes the QA testing pipeline
func (p *QATestingPipeline) Run(ctx context.Context, event *types.Event) error {
	log.Printf("[QA] Starting QA testing for PR #%d", event.PRNumber)

	// Only run on PR events
	if event.EventType != "pull_request" {
		log.Printf("[QA] Skipping: not a pull request event")
		return nil
	}

	// Step 1: Fetch PR diff
	log.Printf("[QA] Fetching PR diff...")
	diffText, err := p.githubClient.GetPullRequestDiff(ctx, event.Repo, event.RepoOwner, event.PRNumber)
	if err != nil {
		return fmt.Errorf("fetch diff: %w", err)
	}

	// Parse diff
	diff, err := p.diffAnalyzer.Parse(diffText)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}

	if len(diff.Files) == 0 {
		log.Printf("[QA] No files changed, skipping tests")
		return nil
	}

	// Step 2: Detect languages in diff
	log.Printf("[QA] Analyzing %d changed file(s)...", len(diff.Files))
	languages := p.detectLanguages(diff)
	log.Printf("[QA] Detected languages: %v", languages)

	// Step 3: Run tests for each language
	allResults := make(map[executor.Language]*executor.TestResult)
	var totalPassed, totalFailed, totalSkipped int

	for _, lang := range languages {
		exec, ok := p.executors[lang]
		if !ok {
			log.Printf("[QA] Warning: No executor for language %s, skipping", lang)
			continue
		}

		log.Printf("[QA] Running %s tests...", lang)
		result, err := exec.Run(".")
		if err != nil {
			log.Printf("[QA] Warning: %s tests failed to execute: %v", lang, err)
			continue
		}

		allResults[lang] = result
		totalPassed += result.Passed
		totalFailed += result.Failed
		totalSkipped += result.Skipped

		log.Printf("[QA] %s tests: %d passed, %d failed, %d skipped (duration: %dms)",
			lang, result.Passed, result.Failed, result.Skipped, result.Duration)
	}

	// Step 4: Parse coverage if enabled
	var coverageMap map[executor.Language]*executor.Coverage
	if p.config.Outputs.GenerateTests {
		log.Printf("[QA] Parsing coverage reports...")
		coverageMap = p.parseCoverage(allResults)
	}

	// Step 5: Check coverage gates
	gatesPassed := true
	if coverageMap != nil {
		gatesPassed = p.checkCoverageGates(coverageMap)
	}

	// Step 6: Generate QA artifacts
	artifacts := &types.QAArtifacts{
		Coverage: p.aggregateCoverage(coverageMap),
	}

	// Step 7: Post QA report to PR
	log.Printf("[QA] Posting QA report to PR...")
	if err := p.postQAReport(ctx, event, allResults, artifacts, gatesPassed); err != nil {
		log.Printf("[QA] Warning: Failed to post QA report: %v", err)
	}

	// Step 8: Set commit status
	status := "success"
	description := fmt.Sprintf("Tests passed: %d, Failed: %d", totalPassed, totalFailed)

	if totalFailed > 0 {
		status = "failure"
	} else if !gatesPassed {
		status = "failure"
		description = "Coverage gates not met"
	}

	if err := p.githubClient.SetStatus(ctx, event.Repo, event.RepoOwner, event.CommitSHA, status, description); err != nil {
		log.Printf("[QA] Warning: Failed to set commit status: %v", err)
	}

	log.Printf("[QA] Pipeline completed: %s", status)
	return nil
}

// detectLanguages detects languages from changed files
func (p *QATestingPipeline) detectLanguages(diff *types.Diff) []executor.Language {
	langSet := make(map[executor.Language]bool)

	for _, file := range diff.Files {
		// Map file language to executor language
		var execLang executor.Language

		switch strings.ToLower(file.Lang) {
		case "go":
			execLang = executor.LanguageGo
		case "python":
			execLang = executor.LanguagePython
		case "javascript", "typescript":
			execLang = executor.LanguageJavaScript
		default:
			continue
		}

		langSet[execLang] = true
	}

	languages := make([]executor.Language, 0, len(langSet))
	for lang := range langSet {
		languages = append(languages, lang)
	}

	return languages
}

// parseCoverage parses coverage reports for all languages
func (p *QATestingPipeline) parseCoverage(results map[executor.Language]*executor.TestResult) map[executor.Language]*executor.Coverage {
	coverageMap := make(map[executor.Language]*executor.Coverage)

	for lang, result := range results {
		if result.CoveragePath == "" {
			continue
		}

		// Check if coverage file exists
		if _, err := os.Stat(result.CoveragePath); os.IsNotExist(err) {
			log.Printf("[QA] Coverage file not found: %s", result.CoveragePath)
			continue
		}

		exec := p.executors[lang]
		coverage, err := exec.ParseCoverage(result.CoveragePath)
		if err != nil {
			log.Printf("[QA] Warning: Failed to parse %s coverage: %v", lang, err)
			continue
		}

		coverageMap[lang] = coverage
		log.Printf("[QA] %s coverage: %.1f%% line, %.1f%% branch",
			lang, coverage.LinePercent, coverage.BranchPercent)
	}

	return coverageMap
}

// checkCoverageGates checks if coverage meets configured thresholds
func (p *QATestingPipeline) checkCoverageGates(coverageMap map[executor.Language]*executor.Coverage) bool {
	// Default threshold: 80%
	threshold := 80.0

	allPassed := true
	for lang, coverage := range coverageMap {
		if coverage.LinePercent < threshold {
			log.Printf("[QA] ❌ %s coverage %.1f%% below threshold %.1f%%",
				lang, coverage.LinePercent, threshold)
			allPassed = false
		} else {
			log.Printf("[QA] ✅ %s coverage %.1f%% meets threshold %.1f%%",
				lang, coverage.LinePercent, threshold)
		}
	}

	return allPassed
}

// aggregateCoverage aggregates coverage from all languages
func (p *QATestingPipeline) aggregateCoverage(coverageMap map[executor.Language]*executor.Coverage) *types.CoverageReport {
	if len(coverageMap) == 0 {
		return nil
	}

	var totalLines, coveredLines int
	var totalBranches, coveredBranches int

	for _, coverage := range coverageMap {
		totalLines += coverage.TotalLines
		coveredLines += coverage.CoveredLines
		totalBranches += coverage.TotalBranches
		coveredBranches += coverage.CoveredBranches
	}

	linePercent := 0.0
	if totalLines > 0 {
		linePercent = float64(coveredLines) / float64(totalLines) * 100.0
	}

	branchPercent := 0.0
	if totalBranches > 0 {
		branchPercent = float64(coveredBranches) / float64(totalBranches) * 100.0
	}

	return &types.CoverageReport{
		LineCoverage:   linePercent,
		BranchCoverage: branchPercent,
		TotalLines:     totalLines,
		CoveredLines:   coveredLines,
	}
}

// postQAReport posts comprehensive QA report to PR
func (p *QATestingPipeline) postQAReport(
	ctx context.Context,
	event *types.Event,
	results map[executor.Language]*executor.TestResult,
	artifacts *types.QAArtifacts,
	gatesPassed bool,
) error {
	var sb strings.Builder

	// Header
	sb.WriteString("## 🧪 AurumCode QA Report\n\n")

	// Overall status
	status := "✅ All tests passed"
	if !gatesPassed {
		status = "❌ Coverage gates not met"
	}

	for _, result := range results {
		if result.Failed > 0 {
			status = "❌ Tests failed"
			break
		}
	}

	sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", status))

	// Test Results by Language
	sb.WriteString("### Test Results\n\n")

	totalPassed := 0
	totalFailed := 0
	totalSkipped := 0

	for lang, result := range results {
		totalPassed += result.Passed
		totalFailed += result.Failed
		totalSkipped += result.Skipped

		icon := "✅"
		if result.Failed > 0 {
			icon = "❌"
		}

		sb.WriteString(fmt.Sprintf("**%s %s**\n", icon, lang))
		sb.WriteString(fmt.Sprintf("- Passed: %d\n", result.Passed))
		sb.WriteString(fmt.Sprintf("- Failed: %d\n", result.Failed))
		sb.WriteString(fmt.Sprintf("- Skipped: %d\n", result.Skipped))
		sb.WriteString(fmt.Sprintf("- Duration: %dms\n\n", result.Duration))
	}

	sb.WriteString(fmt.Sprintf("**Total:** %d passed, %d failed, %d skipped\n\n",
		totalPassed, totalFailed, totalSkipped))

	// Coverage Report
	if artifacts.Coverage != nil {
		sb.WriteString("### Coverage\n\n")
		sb.WriteString(fmt.Sprintf("- **Line Coverage:** %.1f%%\n", artifacts.Coverage.LineCoverage))
		sb.WriteString(fmt.Sprintf("- **Branch Coverage:** %.1f%%\n", artifacts.Coverage.BranchCoverage))
		sb.WriteString(fmt.Sprintf("- **Total Lines:** %d\n", artifacts.Coverage.TotalLines))
		sb.WriteString(fmt.Sprintf("- **Covered Lines:** %d\n\n", artifacts.Coverage.CoveredLines))

		if !gatesPassed {
			sb.WriteString("⚠️ **Coverage below threshold (80%)**\n\n")
		}
	}

	// Failed tests details
	hasFailures := false
	for _, result := range results {
		if result.Failed > 0 && result.Output != "" {
			if !hasFailures {
				sb.WriteString("### Failed Tests Output\n\n")
				hasFailures = true
			}

			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", result.Output))
		}
	}

	// Footer
	sb.WriteString("---\n🤖 Generated by AurumCode QA Pipeline\n")

	// Post comment
	comment := types.ReviewComment{
		Body:     sb.String(),
		CommitID: event.CommitSHA,
	}

	return p.githubClient.PostReviewComment(ctx, event.Repo, event.RepoOwner, event.PRNumber, comment)
}
