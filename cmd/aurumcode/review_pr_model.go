package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/session"
)

// runModelPass selects the model, wraps it with the trusted context and
// runs the review.
func (p *prReview) runModelPass() (int, bool) {
	steps := []session.Step{
		p.selectProvider, p.wrapContext, p.resolveProfiles, p.resolveSkills, p.buildReviewer,
		p.generateReview, p.noteModelOutcome, p.joinEvidence,
	}
	for _, step := range steps {
		if code, done := step(); done {
			return code, true
		}
	}
	return 0, false
}

// selectProvider picks the model (--modelo, AUR-451, or the default
// selection) and captures its real identity before any context wrapping
// (AUR-524, mirroring --base's baseModelIdentity).
func (p *prReview) selectProvider() (int, bool) {
	var providerVia string
	var err error
	if p.opts.modelo != "" {
		p.provider, providerVia, err = selectProviderForModel(p.opts.modelo)
	} else {
		p.provider, err = selectProvider()
	}
	if err != nil {
		if p.opts.modelo != "" {
			return reportModelUnavailable(p.stderr, p.opts.modelo, err), true
		}
		fmt.Fprintf(p.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	if p.opts.modelo != "" {
		fmt.Fprintf(p.stderr, "aurumcode review: reviewing with model %q (%s)\n", p.opts.modelo, providerVia)
	}
	p.baseModelIdentity = modelCacheKey(p.provider)
	return 0, false
}

// wrapContext renders the repository context read from the trusted base
// ref for the prompt's repository-context slot (a pull request may not change the prompt, skills or
// docs used to judge it). The policy's own context files come first
// (AUR-518, AC-004). The digest of the redacted block feeds the verdict key.
func (p *prReview) wrapContext() (int, bool) {
	stderr := p.stderr
	p.contextRef = p.env().baseSHA
	if strings.TrimSpace(p.contextRef) == "" {
		p.contextRef = p.env().githubSHA
	}
	providers, contextErr := loadPullRequestContext(p.ctx, p.client, p.owner, p.repoName, p.cfg, p.contextRef)
	if contextErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: loading repository review context: %v\n", contextErr)
		return 1, true
	}
	if p.centralCfg != nil {
		providers = append(config.ConfiguredProviders(p.opts.policyDir, p.centralCfg), providers...)
	}
	providers = append(providers, mcpContextProviders(trustedMCPSources(p.cfg, strings.TrimSpace(p.env().baseSHA) != "", p.centralCfg), p.filter)...)
	var policySkills skills.Source
	if p.centralCfg != nil {
		policySkills = localSkillSource(p.opts.policyDir, "policy")
	}
	catalog, catalogErr := resolveSkillCatalog(policySkills, remoteSkillSource{ctx: p.ctx, client: p.client, owner: p.owner, repo: p.repoName, ref: p.contextRef})
	if catalogErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", catalogErr)
		return 1, true
	}
	excludeListedSkills(catalog, p.centralCfg, p.cfg)
	p.skillCatalog = catalog
	p.skillNotices = skillSelectionNotices(catalog, diffPaths(p.diff), p.filter)
	providers = append(providers, catalog)
	p.contextBlockDigest = contextBlockCacheDigest(providers, diffPaths(p.diff), p.filter)
	return p.buildRepositoryContext(providers)
}

// resolveSkills builds the dynamic, skill-section rule set this run accepts
// citations against (AUR-519): policy skills read locally, repo skills read
// at the trusted base ref, never the pull request's own head.
func (p *prReview) resolveSkills() (int, bool) {
	policySkillRules := map[string]review.Rule{}
	if p.centralCfg != nil {
		policySkillRules = dynamicRulesFromLocalSkills(p.opts.policyDir, p.centralCfg.Review.Context.Skills, gateOriginPolicy)
	}
	repoSkillRules := dynamicRulesFromRemoteSkills(p.ctx, p.client, p.owner, p.repoName, p.cfg.Review.Context.Skills, p.contextRef, gateOriginRepo)
	// The skill directories the catalog read at the same trusted ref.
	catalogPolicy, catalogRepo := dynamicRulesFromCatalog(p.skillCatalog, diffPaths(p.diff))
	p.dynamicRules = mergeDynamicRules(unionRules(policySkillRules, catalogPolicy), unionRules(repoSkillRules, catalogRepo))
	p.ruleCatalogIDs = mergedRuleCatalogIDs(prompt.DefaultRuleCatalog, p.dynamicRules)
	p.ruleCatalogDigest = ruleCatalogCacheDigest(p.ruleCatalogIDs, p.dynamicRules)
	return 0, false
}

// buildReviewer wires --limite's cost tracker (AUR-451), the orchestrator
// and the reviewer taught the expanded catalog, and reads the PR history.
func (p *prReview) buildReviewer() (int, bool) {
	stderr := p.stderr
	if p.opts.limiteSet {
		price, err := costPrice()
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2, true
		}
		modelKey := costModelKey(p.opts.modelo)
		p.provider = &fixedModelProvider{Provider: p.provider, model: modelKey}
		p.tracker = buildCostTracker(p.limiteUSD, modelKey, price)
		printCostEstimate(stderr, estimateCostUSD(p.diff, price), p.limiteUSD)
	}
	orchestrator := llm.NewOrchestrator(p.provider, nil, p.tracker)
	p.verifyCaller = orchestrator
	p.reviewer = review.NewReviewer(orchestrator, review.DefaultConfig())
	p.reviewer.SetBatchLimits(configuredBatchLimits(p.cfg))
	p.reviewer.SetDynamicRules(p.dynamicRules)
	if err := p.reviewer.SetRuleCatalog(p.ruleCatalogIDs); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	p.prepareDeliberation(orchestrator, toolsCapable(orchestrator, p.profilesApplied()), p.reviewer)
	p.history, p.historyEntries, p.historyErr = pullRequestHistoryContext(p.ctx, p.client, p.owner, p.repoName, p.prNumber,
		p.env().githubSHA, p.env().baseSHA, p.filter)
	if p.historyErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: PR history unavailable: %s; reviewing the current diff without conversation history\n", p.filter.Redact(p.historyErr.Error()))
		p.history = historyUnavailableNotice(p.reviewLanguage)
	}
	return 0, false
}

// noteModelOutcome reports what the engine discarded and what the run cost.
func (p *prReview) noteModelOutcome() (int, bool) {
	stderr, result := p.stderr, p.result
	if p.historyErr != nil {
		// The caller's coverage notice survives even if the model omits it.
		result.Limitations = append(result.Limitations, historyUnavailableNotice(p.reviewLanguage))
	}
	if warning := result.Metadata["discard_warning"]; warning != "" {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
	}
	if warning := result.Metadata["scope_discard_warning"]; warning != "" {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
	}
	if warning := result.Metadata[review.AssessmentDiscardWarningKey]; warning != "" {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
	}
	if sections := result.Metadata["optional_sections_discarded"]; sections != "" {
		fmt.Fprintf(stderr, "aurumcode review: optional model sections discarded for schema mismatch: %s\n", sections)
	}
	if p.opts.limiteSet {
		printRealCost(stderr, realCostUSD(p.tracker, p.limiteUSD), p.limiteUSD)
	}
	return 0, false
}
