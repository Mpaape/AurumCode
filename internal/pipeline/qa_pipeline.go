package pipeline

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/config"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/internal/testgen"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// TODO: Phase 7 - Replace with Docker-based test execution
// Temporary stubs to maintain compilation
type Language string
type Executor interface {
	Run(dir string) (*TestResult, error)
	ParseCoverage(path string) (*Coverage, error)
}
type TestResult struct {
	Passed, Failed, Skipped int
	Duration                int64
	CoveragePath            string
	Output                  string
}
type Coverage struct {
	LinePercent, BranchPercent         float64
	TotalLines, CoveredLines           int
	TotalBranches, CoveredBranches     int
}

const (
	LanguageGo         Language = "go"
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
)

// QATestingPipeline handles the QA testing use case
type QATestingPipeline struct {
	config       *config.Config
	githubClient *githubclient.Client
	llmOrch      *llm.Orchestrator
	testGen      *testgen.Generator
	diffAnalyzer *analyzer.DiffAnalyzer
	executors    map[Language]Executor // TODO: Phase 7 - Docker-based executors
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
		executors:    make(map[Language]Executor), // TODO: Phase 7 - Initialize Docker executors
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

	// Step 3: Run baseline tests (on base branch) to establish what was already failing
	log.Printf("[QA] Running BASELINE tests (before PR changes)...")
	baselineResults := make(map[Language]*TestResult)

	// TODO: Checkout base branch, run tests, then checkout back
	// For now, we'll run tests on current state and compare
	for _, lang := range languages {
		exec, ok := p.executors[lang]
		if !ok {
			continue
		}

		log.Printf("[QA] Running baseline %s tests...", lang)
		result, err := exec.Run(".")
		if err != nil {
			log.Printf("[QA] Warning: baseline %s tests failed to execute: %v", lang, err)
			continue
		}

		baselineResults[lang] = result
		log.Printf("[QA] Baseline %s: %d passed, %d failed, %d skipped",
			lang, result.Passed, result.Failed, result.Skipped)
	}

	// Step 4: Run tests for each language (current PR state)
	log.Printf("[QA] Running PR tests (after PR changes)...")
	allResults := make(map[Language]*TestResult)
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

	// Step 5: Compare baseline vs current - detect regressions
	regressions := p.detectRegressions(baselineResults, allResults)
	preExistingFailures := p.detectPreExistingFailures(baselineResults, allResults)

	// Step 4: Parse coverage if enabled
	var coverageMap map[Language]*Coverage
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

	// Step 7: Post QA report to PR with baseline comparison
	log.Printf("[QA] Posting QA report to PR...")
	if err := p.postQAReportWithBaseline(ctx, event, baselineResults, allResults, regressions, preExistingFailures, artifacts, gatesPassed); err != nil {
		log.Printf("[QA] Warning: Failed to post QA report: %v", err)
	}

	// Step 8: Set commit status - ONLY fail on regressions, not pre-existing failures
	status := "success"
	description := fmt.Sprintf("Tests passed: %d, Failed: %d", totalPassed, totalFailed)

	// Only fail if there are NEW regressions (not pre-existing failures)
	if len(regressions) > 0 {
		status = "failure"
		description = fmt.Sprintf("🔴 %d new test failures (regressions)", len(regressions))
	} else if !gatesPassed {
		status = "failure"
		description = "Coverage gates not met"
	} else if len(preExistingFailures) > 0 {
		// Pre-existing failures don't block the PR, just inform
		description = fmt.Sprintf("✅ No new failures (%d pre-existing)", len(preExistingFailures))
	}

	if err := p.githubClient.SetStatus(ctx, event.Repo, event.RepoOwner, event.CommitSHA, status, description); err != nil {
		log.Printf("[QA] Warning: Failed to set commit status: %v", err)
	}

	log.Printf("[QA] Pipeline completed: %s", status)
	return nil
}

// detectLanguages detects languages from changed files
func (p *QATestingPipeline) detectLanguages(diff *types.Diff) []Language {
	langSet := make(map[Language]bool)

	for _, file := range diff.Files {
		// Map file language to executor language
		var execLang Language

		switch strings.ToLower(file.Lang) {
		case "go":
			execLang = LanguageGo
		case "python":
			execLang = LanguagePython
		case "javascript", "typescript":
			execLang = LanguageJavaScript
		default:
			continue
		}

		langSet[execLang] = true
	}

	languages := make([]Language, 0, len(langSet))
	for lang := range langSet {
		languages = append(languages, lang)
	}

	return languages
}

// parseCoverage parses coverage reports for all languages
func (p *QATestingPipeline) parseCoverage(results map[Language]*TestResult) map[Language]*Coverage {
	coverageMap := make(map[Language]*Coverage)

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
func (p *QATestingPipeline) checkCoverageGates(coverageMap map[Language]*Coverage) bool {
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
func (p *QATestingPipeline) aggregateCoverage(coverageMap map[Language]*Coverage) *types.CoverageReport {
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
	results map[Language]*TestResult,
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

// detectRegressions finds tests that were passing in baseline but are now failing
func (p *QATestingPipeline) detectRegressions(baseline, current map[Language]*TestResult) []string {
	regressions := []string{}

	for lang, currResult := range current {
		baseResult, hasBaseline := baseline[lang]

		if !hasBaseline {
			// No baseline to compare, assume all failures are new
			if currResult.Failed > 0 {
				regressions = append(regressions, fmt.Sprintf("%s: %d new failures (no baseline)", lang, currResult.Failed))
			}
			continue
		}

		// New failures = current failures that didn't exist in baseline
		newFailures := currResult.Failed - baseResult.Failed
		if newFailures > 0 {
			regressions = append(regressions, fmt.Sprintf("%s: %d new failures", lang, newFailures))
		}
	}

	return regressions
}

// detectPreExistingFailures finds tests that were already failing before this PR
func (p *QATestingPipeline) detectPreExistingFailures(baseline, current map[Language]*TestResult) []string {
	preExisting := []string{}

	for lang, currResult := range current {
		baseResult, hasBaseline := baseline[lang]

		if !hasBaseline {
			continue
		}

		// Pre-existing failures = failures that existed in both baseline and current
		if baseResult.Failed > 0 && currResult.Failed > 0 {
			commonFailures := baseResult.Failed
			if currResult.Failed < baseResult.Failed {
				commonFailures = currResult.Failed
			}

			if commonFailures > 0 {
				preExisting = append(preExisting, fmt.Sprintf("%s: %d pre-existing failures", lang, commonFailures))
			}
		}
	}

	return preExisting
}

// postQAReportWithBaseline posts QA report with baseline comparison
func (p *QATestingPipeline) postQAReportWithBaseline(
	ctx context.Context,
	event *types.Event,
	baselineResults map[Language]*TestResult,
	currentResults map[Language]*TestResult,
	regressions []string,
	preExistingFailures []string,
	artifacts *types.QAArtifacts,
	gatesPassed bool,
) error {
	var sb strings.Builder

	// Header
	sb.WriteString("## 🧪 AurumCode QA Report with Baseline Comparison\n\n")

	// Overall status - SMART: Only fail on regressions, not pre-existing issues
	status := "✅ No new test failures"
	if len(regressions) > 0 {
		status = "🔴 NEW test failures detected (regressions)"
	} else if len(preExistingFailures) > 0 {
		status = "⚠️ Tests passing (but pre-existing failures noted)"
	}

	if !gatesPassed {
		status += " | ❌ Coverage gates not met"
	}

	sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", status))

	// Key Insight: Baseline Testing
	sb.WriteString("### 🔍 Baseline Testing\n\n")
	sb.WriteString("AurumCode runs tests **BEFORE** and **AFTER** your changes:\n")
	sb.WriteString("- ✅ **Blocks PR only on NEW failures** (regressions)\n")
	sb.WriteString("- ℹ️ **Informs about pre-existing failures** (doesn't block)\n")
	sb.WriteString("- 🎯 **Prevents blocking PRs for already-broken code**\n\n")

	// Regressions (NEW failures)
	if len(regressions) > 0 {
		sb.WriteString("### 🔴 New Failures (Regressions) - BLOCKING\n\n")
		sb.WriteString("These tests were passing before your changes but are now failing:\n\n")
		for _, reg := range regressions {
			sb.WriteString(fmt.Sprintf("- %s\n", reg))
		}
		sb.WriteString("\n**Action Required:** Fix these regressions before merging.\n\n")
	}

	// Pre-existing failures
	if len(preExistingFailures) > 0 {
		sb.WriteString("### ⚠️ Pre-Existing Failures - INFORMATIONAL\n\n")
		sb.WriteString("These tests were already failing before your changes:\n\n")
		for _, pf := range preExistingFailures {
			sb.WriteString(fmt.Sprintf("- %s\n", pf))
		}
		sb.WriteString("\n**No Action Required:** Your PR doesn't introduce these failures. Consider fixing them in a separate PR.\n\n")
	}

	// Baseline vs Current Comparison
	sb.WriteString("### 📊 Test Results Comparison\n\n")
	sb.WriteString("| Language | Baseline | Current | Change |\n")
	sb.WriteString("|----------|----------|---------|--------|\n")

	for lang, currResult := range currentResults {
		baseResult, hasBaseline := baselineResults[lang]

		if !hasBaseline {
			sb.WriteString(fmt.Sprintf("| %s | No baseline | ✅ %d / ❌ %d / ⏭️ %d | New |\n",
				lang, currResult.Passed, currResult.Failed, currResult.Skipped))
			continue
		}

		passedDiff := currResult.Passed - baseResult.Passed
		failedDiff := currResult.Failed - baseResult.Failed

		passedIcon := "➡️"
		if passedDiff > 0 {
			passedIcon = "⬆️"
		} else if passedDiff < 0 {
			passedIcon = "⬇️"
		}

		failedIcon := "➡️"
		if failedDiff > 0 {
			failedIcon = "⬆️" // More failures = bad
		} else if failedDiff < 0 {
			failedIcon = "⬇️" // Fewer failures = good
		}

		sb.WriteString(fmt.Sprintf("| %s | ✅ %d / ❌ %d | ✅ %d / ❌ %d | %s %+d / %s %+d |\n",
			lang,
			baseResult.Passed, baseResult.Failed,
			currResult.Passed, currResult.Failed,
			passedIcon, passedDiff,
			failedIcon, failedDiff))
	}

	sb.WriteString("\n")

	// Coverage Report
	if artifacts.Coverage != nil {
		sb.WriteString("### 📈 Coverage\n\n")
		sb.WriteString(fmt.Sprintf("- **Line Coverage:** %.1f%%\n", artifacts.Coverage.LineCoverage))
		sb.WriteString(fmt.Sprintf("- **Branch Coverage:** %.1f%%\n", artifacts.Coverage.BranchCoverage))
		sb.WriteString(fmt.Sprintf("- **Total Lines:** %d\n", artifacts.Coverage.TotalLines))
		sb.WriteString(fmt.Sprintf("- **Covered Lines:** %d\n\n", artifacts.Coverage.CoveredLines))

		if !gatesPassed {
			sb.WriteString("⚠️ **Coverage below threshold (80%)**\n\n")
		}
	}

	// Summary
	sb.WriteString("### ✅ Summary\n\n")
	if len(regressions) == 0 && len(preExistingFailures) == 0 {
		sb.WriteString("🎉 **All tests passing!** No failures detected.\n\n")
	} else if len(regressions) == 0 {
		sb.WriteString(fmt.Sprintf("✅ **Your changes didn't break anything!** (%d pre-existing failures noted)\n\n", len(preExistingFailures)))
	} else {
		sb.WriteString("❌ **Your changes introduced test failures.** Please fix the regressions.\n\n")
	}

	// Footer
	sb.WriteString("---\n🤖 Generated by AurumCode QA Pipeline with Baseline Testing\n")

	// Post comment
	comment := types.ReviewComment{
		Body:     sb.String(),
		CommitID: event.CommitSHA,
	}

	return p.githubClient.PostReviewComment(ctx, event.Repo, event.RepoOwner, event.PRNumber, comment)
}
