// The --base model and evidence phases. The model's quality pass lives in
// review_base_quality.go; this file selects the provider, settles what a
// selection failure means, and runs the evidence steps the session shares
// (review_evidence.go) plus the local coverage notice.
package main

import (
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/session"
)

// runModelPass selects the provider, settles what a selection failure
// means and runs the quality pass.
func (b *baseReview) runModelPass() (int, bool) {
	for _, step := range []session.Step{b.selectProvider, b.settleQualityStatus, b.runQualityPass, b.joinEvidence} {
		if code, done := step(); done {
			return code, true
		}
	}
	return 0, false
}

// collectEvidence runs the deterministic passes before the model, so their
// findings reach it as evidence to weigh: the security pass, the embedded
// analysis and SAST. Nothing here touches the review result.
func (b *baseReview) collectEvidence() (int, bool) {
	if code, done := b.runSecurityPass(); done {
		return code, true
	}
	b.analysisIssues = staticAnalysisIssues(b.diff, b.reviewLanguage)
	b.runScanners(b.cwd, localScanRange(b.cwd, b.f.base), "")
	b.offerEvidence()
	return 0, false
}

// joinEvidence closes the model phase: the model's assessments are copied
// onto the evidence, the evidence joins the result, the rule config and the
// verdict snapshot apply, and the coverage is recorded.
func (b *baseReview) joinEvidence() (int, bool) {
	b.settleDeferredScans()
	b.attachAssessments()
	b.reportSecurityPass()
	b.result.Issues = append(b.result.Issues, b.analysisIssues...)
	b.snapshotAndApplyRules()
	b.joinScanners()
	b.runDependencyCheck()
	b.explainDependencyReach()
	b.settleCIStatus(noCIContext())
	b.recordCoverage()
	return 0, false
}

// qualityDidNotRun reports a quality review that was skipped or failed.
func (b *baseReview) qualityDidNotRun() bool {
	return b.model == modelSkipped || b.model == modelProviderFailed || b.model == modelDeliberationLimit
}

// selectProvider picks the provider (--modelo commands which model reviews,
// AUR-436), computes the dynamic skill-section rule set this run accepts
// citations against (AUR-519: policy skills first, then the repository's,
// computed once before the model is taught a catalog) and renders the
// context providers' block for the prompt's repository-context slot
// (AUR-452/518: the policy's own context first). The provider itself is
// never wrapped, so its capabilities stay visible (AUR-513).
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
	contextProviders := config.ConfiguredProviders(b.cwd, b.cfg)
	if b.centralCfg != nil {
		contextProviders = append(config.ConfiguredProviders(b.policyDir, b.centralCfg), contextProviders...)
	}
	contextProviders = append(contextProviders, mcpContextProviders(trustedMCPSources(b.cfg, b.localMCPTrusted(), b.centralCfg), b.filter)...)
	catalog, dynamicRules, catalogErr := resolveSkillRules(skillLayers{cwd: b.cwd, policyDir: b.policyDir, cfg: b.cfg, centralCfg: b.centralCfg}, diffPaths(b.diff))
	if catalogErr != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", catalogErr)
		return 1, true
	}
	b.dynamicRules = dynamicRules
	b.ruleCatalogIDs = mergedRuleCatalogIDs(prompt.DefaultRuleCatalog, b.dynamicRules)
	b.ruleCatalogDigest = ruleCatalogCacheDigest(b.ruleCatalogIDs, b.dynamicRules)
	b.skillNotices = skillSelectionNotices(catalog, diffPaths(b.diff), b.filter)
	contextProviders = append(contextProviders, catalog)
	// AUR-513: digest the SAME redacted block the model will receive.
	b.contextBlockDigest = contextBlockCacheDigest(contextProviders, diffPaths(b.diff), b.filter)
	return b.buildRepositoryContext(contextProviders)
}

// settleQualityStatus decides what a provider selection failure means.
//   - Nothing configured and no --modelo (AUR-449/490): the quality review
//     is skipped (modelSkipped), deterministic analysis still runs, and
//     stderr says so plainly; --exigir-qualidade turns it into a failure
//     (modelProviderFailed, AUR-458).
//   - Any other failure: the published refusal (exit 1), except that with
//     --seguranca and no --modelo the deterministic pass still runs and the
//     run is marked modelProviderFailed (AUR-458/473).
func (b *baseReview) settleQualityStatus() (int, bool) {
	f := b.f
	if b.providerErr != nil && f.modelo == "" && errors.Is(b.providerErr, errNoProviderConfigured) {
		b.model = modelSkipped
		fmt.Fprintf(b.stderr, "aurumcode review: %s\n", i18n.Text(b.reviewLanguage, "terminal.quality_skipped"))
		// AUR-542: the complete AURUMCODE_LLM_FIXTURE teaching text.
		printLines(b.stderr, "aurumcode review: ", noProviderText(b.reviewLanguage))
		if f.exigirQualidade {
			b.model = modelProviderFailed
			fmt.Fprintf(b.stderr, "aurumcode review: %s\n", i18n.Text(b.reviewLanguage, "terminal.quality_required"))
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
		b.model = modelProviderFailed
		fmt.Fprintln(b.stderr, "aurumcode review: quality review failed; running --seguranca only -- this run reviewed HALF of what was asked")
	}
	if f.modelo != "" && b.model != modelProviderFailed {
		fmt.Fprintf(b.stderr, "aurumcode review: reviewing with model %q (%s)\n", f.modelo, b.providerVia)
	}
	return 0, false
}

// recordCoverage runs the deterministic coverage pass (AUR-476/522): it
// reads only the diff, the config and the prompt builder's own metadata,
// never the model's summary, so the notice survives a model that claims
// complete coverage. It also joins the changelog and policy-warning
// limitations the terminal already showed.
func (b *baseReview) recordCoverage() {
	b.result.Limitations = capped(b.result.Limitations, maxModelLimitations)
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
	b.result.Limitations = append(b.result.Limitations, b.skillNotices...)
}
