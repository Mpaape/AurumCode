package main

import (
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
)

// inconclusiveReason ranks why this run's model/analysis half cannot be
// trusted (AUR-505/458/537): a provider that never answered outranks one
// whose answer could not be parsed, which outranks the degraded free-text
// fallback (AC-008), then SAST's own reason, then AUR-476's partial coverage.
func (p *prReview) inconclusiveReason() string {
	switch {
	case p.providerFailed:
		return gateReasonProviderFailure
	case p.qualityDegraded:
		return "model_parse_failure"
	case prompt.IsDegradedParse(p.result):
		return "degraded_parse"
	case p.sastReason != "":
		// Feeds the top-level reason too: the SARIF document reads only
		// the reason for executionSuccessful.
		return p.sastReason
	case p.coverage.partial():
		return "partial_coverage"
	}
	return ""
}

// runGate runs the shared pipeline over this path's inputs. On --pr the repo
// identity is the verified "owner/repo" of the pull request, never anything
// derived from its author-controlled diff or checkout.
func (p *prReview) runGate() (int, bool) {
	identity := p.owner + "/" + p.repoName
	reason := p.inconclusiveReason()
	p.run = &gateRun{
		Ctx: p.ctx, Cfg: p.cfg, Diff: p.diff, Review: p.result,
		Language: p.reviewLanguage, Filter: p.filter, Stdout: p.stdout, Stderr: p.stderr,
		RepoIdentity: identity, RepoIdentityKnown: true, Now: time.Now,
	}
	pipeline := assembleGatePipeline(gatePipelineInputs{
		VerdictKey: gateVerdictKeyInputs{
			ContextKey: func() string {
				return reviewContextCacheKey(p.provider, p.baseModelIdentity, p.reviewLanguage, p.codebaseText, p.memoryNotesText, "", p.contextBlockDig, p.ruleCatalogDig)
			},
			PolicyDigest:   render.PolicyDigest(p.opts.policyDir, p.centralCfg),
			BinaryIdentity: binaryIdentity(),
			RepoIdentity:   identity,
			DiffDigest:     diffContentDigest(p.diff),
			ReviewedSHA:    os.Getenv("GITHUB_SHA"),
		},
		RawIssues:      p.rawIssuesSnapshot,
		AcceptedOrigin: acceptedGateOrigin(p.centralCfg != nil),
		DynamicRules:   p.dynamicRules,
		SASTOrigin:     p.sastOrigin,
		SASTIssues:     p.sastIssues,
		SASTReason:     p.sastReason,
	})
	res, ok := executeGate("--pr", pipeline, p.run, reason)
	if !ok {
		return 2, true
	}
	p.gateRes = res
	// A contributor may have replaced the filter and the writers (a secret
	// learned mid-run); everything after the gate writes through them.
	p.filter, p.stdout, p.stderr = p.run.Filter, p.run.Stdout, p.run.Stderr
	applyGateOutcome(p.run, res)
	return 0, false
}
