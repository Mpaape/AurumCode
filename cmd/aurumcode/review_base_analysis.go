// Phase 2 of the --base path: run the analyses. The model's quality pass
// lives in review_base_quality.go; this file selects the provider, runs the
// deterministic passes (security, static analysis, rule config, SAST) and
// records coverage.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// analyze runs every analysis whose output the gate and the report consume.
func (b *baseReview) analyze() (int, bool) {
	steps := []func() (int, bool){
		b.selectProvider, b.settleQualityStatus, b.runQualityPass, b.runSecurityPass, b.runDeterministicPasses,
	}
	for _, step := range steps {
		if code, done := step(); done {
			return code, true
		}
	}
	return 0, false
}

// selectProvider picks the provider (--modelo commands which model reviews,
// AUR-436), computes the dynamic skill-section rule set this run accepts
// citations against (AUR-519: policy skills first, then the repository's,
// computed once before the model is taught a catalog) and wraps the
// provider with the context providers (AUR-452/518: the policy's own
// context first). The model identity is captured BEFORE wrapping because
// wrapping hides llm.ModelResolver (AUR-513).
func (b *baseReview) selectProvider() (int, bool) {
	if b.f.modelo != "" {
		b.provider, b.providerVia, b.providerErr = selectProviderForModel(b.f.modelo)
	} else {
		b.provider, b.providerErr = selectProvider()
	}
	if b.providerErr != nil {
		return 0, false
	}
	b.baseModelIdentity = modelCacheKey(b.provider)
	policySkillRules := map[string]review.Rule{}
	if b.centralCfg != nil {
		policySkillRules = dynamicRulesFromLocalSkills(b.policyDir, b.centralCfg.Review.Context.Skills, gateOriginPolicy)
	}
	repoSkillRules := dynamicRulesFromLocalSkills(b.cwd, b.repoCfg.Review.Context.Skills, gateOriginRepo)
	b.dynamicRules = mergeDynamicRules(policySkillRules, repoSkillRules)
	b.ruleCatalogIDs = mergedRuleCatalogIDs(prompt.DefaultRuleCatalog, b.dynamicRules)
	b.ruleCatalogDigest = ruleCatalogCacheDigest(b.ruleCatalogIDs, b.dynamicRules)

	contextProviders := config.ConfiguredProviders(b.cwd, b.repoCfg)
	if b.centralCfg != nil {
		contextProviders = append(config.ConfiguredProviders(b.policyDir, b.centralCfg), contextProviders...)
	}
	// AUR-513: digest the SAME redacted block the model will receive.
	b.contextBlockDigest = contextBlockCacheDigest(contextProviders, diffPaths(b.diff), b.filter)
	wrapped, warnings, wrapErr := config.WrapProviderWithWarnings(context.Background(), b.provider, contextProviders, diffPaths(b.diff), b.filter)
	if wrapErr != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", wrapErr)
		return 1, true
	}
	for _, warning := range warnings {
		fmt.Fprintf(b.stderr, "aurumcode review: context provider %q unavailable: %s; continuing without that context\n", warning.Provider, warning.Reason)
	}
	b.provider = wrapped
	return 0, false
}

// settleQualityStatus decides what a provider selection failure means.
//   - Nothing configured and no --modelo (AUR-449/490): the quality review
//     is skipped (qualitySkipped), deterministic analysis still runs, and
//     stderr says so plainly; --exigir-qualidade turns it into a failure
//     (qualityFailed, AUR-458).
//   - Any other failure: the published refusal (exit 1), except that with
//     --seguranca and no --modelo the deterministic pass still runs and the
//     run is marked qualityFailed (AUR-458/473).
func (b *baseReview) settleQualityStatus() (int, bool) {
	f := b.f
	if b.providerErr != nil && f.modelo == "" && errors.Is(b.providerErr, errNoProviderConfigured) {
		b.qualitySkipped = true
		fmt.Fprintln(b.stderr, "aurumcode review: no LLM provider configured: quality review skipped; running deterministic analysis only")
		// AUR-542: the complete AURUMCODE_LLM_FIXTURE teaching text.
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", b.providerErr)
		if f.exigirQualidade {
			b.qualityFailed = true
			fmt.Fprintln(b.stderr, "aurumcode review: --exigir-qualidade: the quality review did not run, so this run is not a clean review")
		}
	} else if b.providerErr != nil {
		rc := 1
		if f.modelo != "" {
			rc = reportModelUnavailable(b.stderr, f.modelo, b.providerErr)
		} else {
			fmt.Fprintf(b.stderr, "aurumcode review: %v\n", b.providerErr)
		}
		// AUR-473: a --modelo the user named that cannot be served is a
		// usage-class failure, before any work is computed.
		if !f.seguranca || f.modelo != "" {
			return rc, true
		}
		b.qualityFailed = true
		fmt.Fprintln(b.stderr, "aurumcode review: quality review failed; running --seguranca only -- this run reviewed HALF of what was asked")
	}
	if f.modelo != "" && !b.qualityFailed {
		fmt.Fprintf(b.stderr, "aurumcode review: reviewing with model %q (%s)\n", f.modelo, b.providerVia)
	}
	return 0, false
}

// runSecurityPass is the deterministic --seguranca pass (AUR-435): it runs
// before anything prints, so a broken rules catalog fails loudly with
// nothing on stdout, and reports its own coverage on stderr (AUR-450).
func (b *baseReview) runSecurityPass() (int, bool) {
	if !b.f.seguranca {
		return 0, false
	}
	findings, applied, total, err := review.SecurityScanWithCoverage(b.diff)
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	b.securityFindings = findings
	printSecurityCoverage(b.stderr, applied, total)
	return 0, false
}

// runDeterministicPasses merges static analysis, applies the repository's
// explicit rule config to both finding sets (AUR-452), runs the SAST pass
// (AUR-548) and records coverage and the declared limitations.
func (b *baseReview) runDeterministicPasses() (int, bool) {
	mergeStaticAnalysis(b.diff, b.result)
	// AUR-524 v2: snapshot the RAW issues, before this run's rule config
	// filters/overrides them, so a stored verdict can be re-evaluated
	// against a later run's rule config.
	b.rawIssues = append([]types.ReviewIssue(nil), b.result.Issues...)
	b.result.Issues = config.ApplyRuleConfig(b.result.Issues, b.repoCfg)
	b.securityFindings = config.ApplyRuleConfig(b.securityFindings, b.repoCfg)
	b.runSAST()
	b.recordCoverage()
	return 0, false
}

// runSAST runs quality_gates.sast's Semgrep pass over the reviewed tree. Its
// issues never pass through config.ApplyRuleConfig (deterministic evidence a
// `rules:` override was never meant to reach) and join result.Issues
// directly. The origin is "policy" only when the CENTRAL POLICY ITSELF
// declares quality_gates.sast (AUR-548).
func (b *baseReview) runSAST() {
	b.sastOrigin = gateOriginRepo
	if b.centralCfg != nil && b.centralCfg.QualityGates.Sast != nil {
		b.sastOrigin = gateOriginPolicy
	}
	b.sastIssues, b.sastReason = runSASTPass(context.Background(), b.cwd, b.repoCfg.QualityGates.Sast, b.sastOrigin == gateOriginPolicy, b.filter, realSemgrepRunner)
	if b.sastReason != "" {
		notice := sastInconclusiveNotice(b.reviewLanguage, b.sastReason)
		fmt.Fprintf(b.stderr, "aurumcode review: %s\n", notice)
		b.result.Limitations = append(b.result.Limitations, notice)
	} else if len(b.sastIssues) > 0 {
		b.result.Issues = append(b.result.Issues, b.sastIssues...)
	}
}

// recordCoverage runs the deterministic coverage pass (AUR-476/522): it
// reads only the diff, the config and the prompt builder's own metadata,
// never the model's summary, so the notice survives a model that claims
// complete coverage. It also joins the changelog and policy-warning
// limitations the terminal already showed.
func (b *baseReview) recordCoverage() {
	b.coverage = mergeReviewCoverage(b.result.Metadata, b.notices, b.rawDiffFileCount, b.ignoredPaths)
	applyStructuralCoverage(grammar.Default(), b.diff, &b.coverage, b.result)
	b.coverageText = coverageNotice(reviewCopyFor(b.reviewLanguage), b.coverage)
	if b.coverageText != "" {
		b.result.Limitations = append(b.result.Limitations, b.coverageText)
	}
	if b.changelogLimitation != "" {
		b.result.Limitations = append(b.result.Limitations, b.changelogLimitation)
	}
	for _, warning := range b.policyWarnings {
		b.result.Limitations = append(b.result.Limitations, warning.Provider+": "+warning.Reason)
	}
}
