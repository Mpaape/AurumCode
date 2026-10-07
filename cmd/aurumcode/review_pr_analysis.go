package main

import (
	"github.com/Mpaape/AurumCode/internal/analyzer"

	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/testgen"
)

// collectEvidence runs the deterministic passes before the model, so their
// findings reach it as evidence to weigh: the security pass (its findings
// later become review comments), the embedded analysis and SAST over the
// verified checkout. Nothing here touches the review result.
func (p *prReview) collectEvidence() (int, bool) {
	if code, done := p.runSecurityPass(); done {
		return code, true
	}
	p.analysisIssues = staticAnalysisIssues(p.diff)
	p.runScanners(p.verifiedDir, p.prScanRange(), p.scanBlockedReason())
	p.offerEvidence()
	return 0, false
}

// joinEvidence closes the model phase: the model's assessments are copied
// onto the evidence, the evidence and the test plan join the result, the
// rule config and the verdict snapshot apply, and the limitations close.
func (p *prReview) joinEvidence() (int, bool) {
	p.settleDeferredScans()
	p.attachAssessments()
	p.reportSecurityPass()
	p.runStaticAnalysis()
	p.snapshotAndApplyRules()
	p.joinScanners()
	p.runDependencyCheck()
	p.settleCIStatus(p.ciFacts)
	return p.finishLimitations()
}

// runStaticAnalysis joins the zero-config static analysis and the proposed
// test plan.
func (p *prReview) runStaticAnalysis() {
	result := p.result
	result.Issues = append(result.Issues, p.analysisIssues...)
	if plan := testgen.Propose(p.diff); plan != nil {
		for _, c := range plan.Cases {
			if strings.TrimSpace(c.Name) == "" {
				continue
			}
			result.TestPlan = append(result.TestPlan, fmt.Sprintf("%s (package %s)", c.Name, c.Package))
		}
	}
}

// scanBlockedReason keeps every scanner off a checkout not verified as the
// pull request's own clean head (AUR-515/536): a stale or unrelated tree is
// never scanned under the reviewed pull request's name.
func (p *prReview) scanBlockedReason() string {
	if p.checkoutMismatch != "" {
		return scanReasonUnverifiedCheckout
	}
	return ""
}

// finishLimitations trims suggestions/limitations to the diff and appends
// the engine-owned notices. The coverage notice is added AFTER
// filterLimitationsAgainstDiff on purpose (AUR-476): that filter removes a
// model limitation naming a changed path as unavailable, while the coverage
// notice is derived from the diff and the prompt builder's own metadata and
// must survive even though it names filtered paths.
func (p *prReview) finishLimitations() (int, bool) {
	result := p.result
	result.Suggestions = filterSuggestionsToChangedLines(p.diff, result.Suggestions)
	suppressOperationalStrengths(p.diff, result)
	result.Limitations = filterLimitationsAgainstDiff(p.diff, result.Limitations)
	p.coverage = mergeReviewCoverage(result.Metadata, append(append([]analyzer.DiffNotice{}, p.binaryNotices...), uninspectedPRNotices(p.diff, p.verifiedDir)...), p.rawDiffFileCount, p.ignoredPaths)
	applyStructuralCoverage(grammar.Default(), p.diff, &p.coverage, result)
	if notice := coverageNotice(reviewCopyFor(p.reviewLanguage), p.coverage); notice != "" {
		result.Limitations = append(result.Limitations, notice)
	}
	if p.changelogLimitation != "" {
		result.Limitations = append(result.Limitations, p.changelogLimitation)
	}
	if p.codebaseLimitation != "" {
		result.Limitations = append(result.Limitations, p.codebaseLimitation)
	}
	// AUR-518: the policy warnings already printed also join the review body.
	for _, warning := range p.policyWarnings {
		result.Limitations = append(result.Limitations, warning.Provider+": "+warning.Reason)
	}
	result.Limitations = append(result.Limitations, p.skillNotices...)
	return 0, false
}
