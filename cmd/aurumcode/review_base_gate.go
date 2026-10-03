// Phase 3 of the --base path: the gate. It only prepares the shared gate.Run
// and the pipeline inputs; the contributors (review_gate_contributors.go)
// make every decision.
package main

import (
	"context"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/render"
)

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

	pipeline := assembleGatePipeline(gatePipelineInputs{
		VerdictKey: gateVerdictKeyInputs{
			ContextKey: func() string {
				return reviewContextCacheKey(b.provider, b.baseModelIdentity, b.reviewLanguage, b.codebaseContextText, b.memoryNotesText, b.profileIdentity, b.contextBlockDigest, b.ruleCatalogDigest)
			},
			PolicyDigest:   render.PolicyDigest(b.policyDir, b.centralCfg),
			BinaryIdentity: binaryIdentity(),
			RepoIdentity:   repoIdentity,
			DiffDigest:     diffContentDigest(b.diff),
			ReviewedSHA:    os.Getenv("GITHUB_SHA"),
		},
		RawIssues:      b.rawIssues,
		AcceptedOrigin: acceptedGateOrigin(b.centralCfg != nil),
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
