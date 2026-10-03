// The gate phase both review paths (--base and --pr) share. A path resolves
// its inputs, runs its analyses, then hands the same declared pipeline to
// executeGate; only the diff source and the publication differ. Adding a
// gate source means one Contributor (review_gate_contributors.go) and one
// line in assembleGatePipeline.
package main

import (
	"fmt"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The command's vocabulary for the gate package's types and constants. This
// is the only production file of cmd/aurumcode that imports internal/gate
// (TestAUR558GateImportedOnlyByAssembly); every other file uses these names.
type (
	gateDecision         = gate.Result
	gateRun              = gate.Run
	gateVerdictKeyInputs = gate.VerdictKeyInputs
	exceptionMatchStatus = gate.ExceptionMatchStatus
	artifactFailure      = gate.ArtifactFailure
)

const (
	gateOriginPolicy        = gate.OriginPolicy
	gateOriginRepo          = gate.OriginRepo
	acceptedExceptionMarker = gate.AcceptedExceptionMarker
	expiredExceptionMarker  = gate.ExpiredExceptionMarker

	gateExitFindings   = gate.ExitFindings
	gateExitBehavioral = gate.ExitBehavioral

	gateReasonAuditWriteFailed = gate.ReasonAuditWriteFailed
	gateReasonSARIFWriteFailed = gate.ReasonSARIFWriteFailed
)

// applyArtifactFailures folds compliance artifacts that could not be written
// into the gate decision (AUR-568).
var applyArtifactFailures = gate.ApplyArtifactFailures

// findingOriginKey identifies a finding for origin lookup in the audit.
var findingOriginKey = gate.FindingOriginKey

// evaluateGate is the policy gate's decision, used by the commands that
// gate outside a review (sbom).
func evaluateGate(cfg config.GateConfig, acceptedOrigin string, dynamic map[string]review.Rule, issues []types.ReviewIssue, reason string, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) (gateDecision, error) {
	return gate.EvaluateGate(cfg, acceptedOrigin, dynamic, issues, reason, exceptions, repoIdentity, now)
}

// applyInconclusiveModeValue turns an inconclusive decision into a failure
// under the resolved block mode, for the commands that gate outside the
// pipeline.
var applyInconclusiveModeValue = gate.ApplyInconclusiveModeValue

// The model pass's typed outcome (gate.ModelOutcome) under the command's
// names.
type modelOutcome = gate.ModelOutcome

const (
	modelReviewed       = gate.ModelReviewed
	modelSkipped        = gate.ModelSkipped
	modelProviderFailed = gate.ModelProviderFailed
	modelParseFailed    = gate.ModelParseFailed
)

// acceptedGateOrigin is the dynamic-rule origin that counts toward the skills
// gate: the central policy's when one is active, otherwise the repository's.
func acceptedGateOrigin(centralPolicyActive bool) string {
	if centralPolicyActive {
		return gate.OriginPolicy
	}
	return gate.OriginRepo
}

// gatePipelineInputs is everything the contributors capture that is not
// part of the shared gate.Run: results of earlier phases of the path.
type gatePipelineInputs struct {
	// VerdictKey/RawIssues feed verdict reuse (AUR-524).
	VerdictKey gateVerdictKeyInputs
	RawIssues  []types.ReviewIssue
	// PromptDigest digests the fixed prompt content that versions a stored
	// verdict.
	PromptDigest func() (string, error)

	// AcceptedOrigin is the dynamic-rule origin that counts toward the
	// skills gate: gateOriginPolicy when a central policy is active,
	// otherwise gateOriginRepo.
	AcceptedOrigin string
	DynamicRules   map[string]review.Rule

	// SAST* are runSASTPass's outputs and the origin its section came from.
	SASTOrigin string
	SASTIssues []types.ReviewIssue
	SASTReason string
}

// assembleGatePipeline declares the one gate pipeline, in the order the
// contributors apply. Both --base and --pr call exactly this function.
func assembleGatePipeline(in gatePipelineInputs) *gate.Pipeline {
	return gate.NewPipeline(
		gate.ExceptionsContributor{},
		gate.VerdictReuseContributor{Key: in.VerdictKey, Raw: in.RawIssues, PromptDigest: in.PromptDigest},
		gate.PolicySkillsContributor{AcceptedOrigin: in.AcceptedOrigin, Dynamic: in.DynamicRules},
		gate.SASTContributor{SectionOrigin: in.SASTOrigin, Issues: in.SASTIssues, Reason: in.SASTReason},
		gate.EmbeddedAnalysisContributor{},
		gate.SecurityPassContributor{},
		gate.AnalysisDataContributor{},
		gate.DependencyTrackContributor{},
	)
}

// applyGateOutcome publishes the decision's lines and withholds approval when
// the gate failed or was inconclusive (shared by --base and --pr).
func applyGateOutcome(run *gateRun, res *gateDecision) { gate.ApplyOutcome(run, res) }

// inconclusiveReason ranks why this run's model/analysis half cannot be
// trusted (gate.RankReason, the one ranking both sources use).
func (s *reviewState) inconclusiveReason() string {
	return string(gate.RankReason(gate.ReasonInputs{
		Model:           s.model,
		DegradedParse:   prompt.IsDegradedParse(s.result),
		SASTReason:      s.sastReason,
		PartialCoverage: s.coverage.partial(),
	}))
}

// runGate is the gate phase of every session: it fills the shared gate.Run
// from the finished analyses, runs the one declared pipeline and publishes
// its decision lines. The repository identity is the source's verified one,
// never anything derived from the model or the diff. A contributor may
// replace the filter and the writers (a secret learned mid-run); everything
// after the gate writes through them.
func (s *reviewState) runGate() (int, bool) {
	run := s.run
	run.Ctx, run.Cfg, run.Diff, run.Review = s.ctx, s.cfg, s.diff, s.result
	run.Extra, run.Security, run.Language = s.securityApart(), s.securityFindings, s.reviewLanguage
	run.Filter, run.Stdout, run.Stderr = s.filter, s.stdout, s.stderr
	run.RepoIdentity, run.RepoIdentityKnown, run.Now = s.repoIdentity, s.repoIdentityKnown, s.deps.clock
	pipeline := assembleGatePipeline(s.gatePipelineInputs())
	reason := s.inconclusiveReason()
	res, ok := s.executeGate(pipeline, reason)
	if !ok {
		return 2, true
	}
	s.gateRes = res
	s.filter, s.stdout, s.stderr = run.Filter, run.Stdout, run.Stderr
	applyGateOutcome(run, res)
	return 0, false
}

// gatePipelineInputs gathers what the contributors capture besides the run.
func (s *reviewState) gatePipelineInputs() gatePipelineInputs {
	return gatePipelineInputs{
		VerdictKey: gateVerdictKeyInputs{
			ContextKey: func() string {
				return reviewContextCacheKey(s.provider, s.baseModelIdentity, s.reviewLanguage, s.codebaseText, s.memoryNotesText, s.profileIdentity, s.contextBlockDigest, s.ruleCatalogDigest)
			},
			PolicyDigest:   render.PolicyDigest(s.policyDir, s.centralCfg),
			BinaryIdentity: binaryIdentity(),
			RepoIdentity:   s.repoIdentity,
			DiffDigest:     gate.DiffContentDigest(s.diff),
			ReviewedSHA:    s.env().githubSHA,
		},
		RawIssues:      s.rawIssues,
		AcceptedOrigin: acceptedGateOrigin(s.centralCfg != nil),
		DynamicRules:   s.dynamicRules,
		SASTOrigin:     s.sastOrigin,
		SASTIssues:     s.sastIssues,
		SASTReason:     s.sastReason,
		PromptDigest:   s.deps.digestBuilder().FixedContentDigest,
	}
}

// executeGate runs pipeline with reason as the initial inconclusive motive,
// after showing it to the session's observer. A configuration error ends
// the review (exit 2): the message is printed here and ok is false.
func (s *reviewState) executeGate(pipeline *gate.Pipeline, reason string) (res *gate.Result, ok bool) {
	s.deps.gateObserver(s.source.Label, pipeline.Names())
	res = &gate.Result{Reason: reason}
	if err := pipeline.Run(s.run.Ctx, s.run, res); err != nil {
		fmt.Fprintf(s.run.Stderr, "aurumcode review: gate: %v\n", err)
		return res, false
	}
	return res, true
}

// decideExit is the one exit decision of a session: gate.ExitPolicy over
// typed inputs. pub carries what only a publishing source produces (zero
// for the local report).
func (s *reviewState) decideExit(pub publishOutcome) int {
	d := gate.ExitPolicy(gate.ExitInputs{
		PublishFailures:     pub.failures,
		NotReviewed:         s.notReviewed(),
		CheckStatusExit:     pub.checkExit,
		GateStatusExit:      pub.gateCheckExit,
		Gate:                s.gateRes,
		ArtifactsMissing:    pub.artifactsMissing,
		FindingsAtThreshold: s.findingsAtThreshold(),
	})
	switch d.Cause {
	case gate.CauseNotReviewed:
		if s.source.NotReviewedNotice != "" {
			fmt.Fprintln(s.stderr, s.source.NotReviewedNotice)
		}
	case gate.CauseFailOn:
		fmt.Fprintf(s.stderr, "aurumcode review: %d finding(s) at severity %s or above (--fail-on %s)\n", s.findingsAtThreshold(), s.thresholdName, s.thresholdName)
	}
	return d.Code
}

// publishOutcome is what publishing produced that the exit decision reads.
type publishOutcome struct {
	failures         int
	checkExit        int
	gateCheckExit    int
	artifactsMissing bool
}
