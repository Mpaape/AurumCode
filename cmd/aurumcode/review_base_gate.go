// Phase 3 of the --base path: the gate. It only prepares the shared gate.Run
// and the pipeline inputs; the contributors (review_gate_contributors.go)
// make every decision.
package main

import (
	"context"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
)

// inconclusiveReason is the base path's motive for an inconclusive run. The
// priority mirrors AUR-458's "did not review outranks reviewed and found
// things": a provider failure or an opted-out quality skip outrank a model
// reply this run could not parse (AC-008), which outranks a SAST execution
// failure (AUR-548: it must also reach the SARIF document's
// executionSuccessful), which outranks partial coverage (AUR-476).
func (b *baseReview) inconclusiveReason() string {
	switch {
	case b.qualityFailed:
		return "provider_failure"
	case b.qualitySkipped:
		return "quality_skipped"
	case prompt.IsDegradedParse(b.result):
		return "degraded_parse"
	case b.sastReason != "":
		return b.sastReason
	case b.coverage.partial():
		return "partial_coverage"
	}
	return ""
}

// decideGate runs the shared gate pipeline over the finished analyses and
// publishes its decision lines. The run's redaction filter and writers may
// be replaced by a contributor (a secret learned mid-run); they are read
// back here and flushed by runReview.
func (b *baseReview) decideGate() (int, bool) {
	// AUR-520: the verified repo identity comes from the local checkout's
	// "origin" remote, never from the model or the diff. Unknown fails
	// closed (the exceptions contributor says so).
	repoIdentity, repoIdentityOK := localRepoIdentity(b.cwd)
	run := b.run
	run.Ctx, run.Cfg, run.Diff, run.Review = context.Background(), b.repoCfg, b.diff, b.result
	run.Extra, run.Language = b.securityFindings, b.reviewLanguage
	run.Filter, run.Stdout, run.Stderr = b.filter, b.stdout, b.stderr
	run.RepoIdentity, run.RepoIdentityKnown, run.Now = repoIdentity, repoIdentityOK, time.Now

	acceptedOrigin := gateOriginRepo
	if b.centralCfg != nil {
		acceptedOrigin = gateOriginPolicy
	}
	pipeline := assembleGatePipeline(gatePipelineInputs{
		Provider: b.provider,
		VerdictKey: gateVerdictKeyInputs{
			BaseModelIdentity:  b.baseModelIdentity,
			Language:           b.reviewLanguage,
			Codebase:           b.codebaseContextText,
			Notes:              b.memoryNotesText,
			Profiles:           b.profileIdentity,
			ContextBlockDigest: b.contextBlockDigest,
			RuleCatalogDigest:  b.ruleCatalogDigest,
			PolicyDigest:       render.PolicyDigest(b.policyDir, b.centralCfg),
			BinaryIdentity:     binaryIdentity(),
			RepoIdentity:       repoIdentity,
			DiffDigest:         diffContentDigest(b.diff),
			ReviewedSHA:        os.Getenv("GITHUB_SHA"),
		},
		RawIssues:      b.rawIssues,
		AcceptedOrigin: acceptedOrigin,
		DynamicRules:   b.dynamicRules,
		SASTOrigin:     b.sastOrigin,
		SASTIssues:     b.sastIssues,
		SASTReason:     b.sastReason,
	})
	res, ok := executeGate("--base", pipeline, run, b.inconclusiveReason())
	if !ok {
		return 2, true
	}
	b.gateRes = res
	b.filter, b.stdout, b.stderr = run.Filter, run.Stdout, run.Stderr
	applyGateOutcome(run, res)
	return 0, false
}
