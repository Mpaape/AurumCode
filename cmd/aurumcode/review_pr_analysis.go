package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/testgen"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// runDeterministicPasses runs --seguranca's pass (AUR-451) over the exact
// diff already fetched and merges its findings into the published ones: a
// security finding is its own comment, its Message already carries its rule
// citation.
func (p *prReview) runDeterministicPasses() (int, bool) {
	if p.opts.seguranca {
		var applied []string
		var total int
		var err error
		p.securityFindings, applied, total, err = review.SecurityScanWithCoverage(p.diff)
		if err != nil {
			fmt.Fprintf(p.stderr, "aurumcode review: %v\n", err)
			return 1, true
		}
		printSecurityCoverage(p.stderr, applied, total)
	}
	if len(p.securityFindings) > 0 {
		combined := make([]types.ReviewIssue, 0, len(p.result.Issues)+len(p.securityFindings))
		combined = append(combined, p.result.Issues...)
		combined = append(combined, p.securityFindings...)
		p.result.Issues = combined
	}
	return 0, false
}

// runStaticAnalysis merges the zero-config static analysis and the proposed
// test plan, snapshots the raw issues for verdict reuse (AUR-524 v2), then
// applies the repository's rule configuration and runs SAST (AUR-548).
func (p *prReview) runStaticAnalysis() (int, bool) {
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
	// The snapshot is taken immediately before this run's own rule config
	// filters/overrides the issues, so a stored verdict can be re-evaluated
	// against a LATER run's rule config.
	p.rawIssuesSnapshot = append([]types.ReviewIssue(nil), result.Issues...)
	result.Issues = config.ApplyRuleConfig(result.Issues, p.cfg)
	p.runSAST()
	return 0, false
}

// runSAST runs quality_gates.sast over the EXACT verified, clean checkout
// (AUR-515/536), never an unverified one: a failed verification makes SAST
// inconclusive without invoking Semgrep. Its findings skip ApplyRuleConfig
// (deterministic evidence a repository's `rules:` was never meant to reach)
// and are appended straight into the issues. The origin is "policy" only
// when the central policy itself declares quality_gates.sast.
func (p *prReview) runSAST() {
	p.sastOrigin = gateOriginRepo
	if p.centralCfg != nil && p.centralCfg.QualityGates.Sast != nil {
		p.sastOrigin = gateOriginPolicy
	}
	if p.cfg.QualityGates.Sast.IsEnabled() {
		if p.checkoutMismatch != "" {
			p.sastReason = sastReasonUnverifiedCheckout
		} else {
			p.sastIssues, p.sastReason = runSASTPass(p.ctx, p.verifiedDir, p.cfg.QualityGates.Sast, p.sastOrigin == gateOriginPolicy, p.filter, realSemgrepRunner)
		}
	}
	if p.sastReason != "" {
		notice := sastInconclusiveNotice(p.reviewLanguage, p.sastReason)
		fmt.Fprintf(p.stderr, "aurumcode review: %s\n", notice)
		p.result.Limitations = append(p.result.Limitations, notice)
	} else if len(p.sastIssues) > 0 {
		p.result.Issues = append(p.result.Issues, p.sastIssues...)
	}
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
	return 0, false
}
