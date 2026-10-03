package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/testgen"
)

// collectEvidence runs the deterministic passes over the exact diff the
// model saw: the security pass (its findings become review comments),
// static analysis and the test plan, the rule config, SAST over the
// verified checkout, and the limitations.
func (p *prReview) collectEvidence() (int, bool) {
	if code, done := p.runSecurityPass(); done {
		return code, true
	}
	p.runStaticAnalysis()
	p.snapshotAndApplyRules()
	p.runSAST(p.verifiedDir, p.sastBlockedReason())
	return p.finishLimitations()
}

// runStaticAnalysis merges the zero-config static analysis and the
// proposed test plan.
func (p *prReview) runStaticAnalysis() {
	result := p.result
	mergeStaticAnalysis(p.diff, result)
	if plan := testgen.Propose(p.diff); plan != nil {
		for _, c := range plan.Cases {
			if strings.TrimSpace(c.Name) == "" {
				continue
			}
			result.TestPlan = append(result.TestPlan, fmt.Sprintf("%s (package %s)", c.Name, c.Package))
		}
	}
}

// sastBlockedReason keeps SAST off a checkout not verified as the pull
// request's own clean head (AUR-515/536): a stale or unrelated tree is
// never scanned under the reviewed pull request's name.
func (p *prReview) sastBlockedReason() string {
	if p.checkoutMismatch != "" {
		return sastReasonUnverifiedCheckout
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
	p.coverage = mergeReviewCoverage(result.Metadata, uninspectedPRNotices(p.diff, p.verifiedDir), p.rawDiffFileCount, p.ignoredPaths)
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
