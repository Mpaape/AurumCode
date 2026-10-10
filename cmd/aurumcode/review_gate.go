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
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
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
	gateScan             = gate.Scan
)

const (
	gateOriginPolicy        = gate.OriginPolicy
	gateOriginRepo          = gate.OriginRepo
	gateOriginSecurity      = gate.OriginSecurity
	gateOriginAnalysis      = gate.OriginAnalysis
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

// The model pass's typed outcome (gate.ModelOutcome) and the quality
// requirement under the command's names.
type (
	modelOutcome       = gate.ModelOutcome
	qualityRequirement = gate.QualityRequirement
)

const (
	modelReviewed          = gate.ModelReviewed
	modelSkipped           = gate.ModelSkipped
	modelProviderFailed    = gate.ModelProviderFailed
	modelParseFailed       = gate.ModelParseFailed
	modelDeliberationLimit = gate.ModelDeliberationLimit

	qualityOptional = gate.QualityOptional
	qualityRequired = gate.QualityRequired
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

	// Scans are the scanner pass's outcomes, one per enabled engine.
	Scans []gateScan

	// Dependencies is the dependency check's report (nil: not declared).
	Dependencies *dependencies.Report
}

// assembleGatePipeline declares the one gate pipeline, in the order the
// contributors apply. Both --base and --pr call exactly this function.
func assembleGatePipeline(in gatePipelineInputs) *gate.Pipeline {
	return gate.NewPipeline(
		gate.ExceptionsContributor{},
		redactedVerdictReuse{gate.VerdictReuseContributor{Key: in.VerdictKey, Raw: in.RawIssues, PromptDigest: in.PromptDigest}},
		gate.PolicySkillsContributor{AcceptedOrigin: in.AcceptedOrigin, Dynamic: in.DynamicRules},
		gate.ScannerContributor{Scans: in.Scans},
		gate.EmbeddedAnalysisContributor{},
		gate.SecurityPassContributor{},
		gate.AnalysisDataContributor{},
		gate.DependencyTrackContributor{},
		gate.DependenciesContributor{Report: in.Dependencies},
	)
}

// applyGateOutcome publishes the decision's lines and withholds approval when
// the gate failed or was inconclusive (shared by --base and --pr).
func applyGateOutcome(run *gateRun, res *gateDecision) { gate.ApplyOutcome(run, res) }

// modelReason is why this run's model half cannot be trusted, by the same
// ranking (gate.RankReason) over the model's inputs alone: "" when the
// model answered and its answer parsed.
func (s *reviewState) modelReason() string {
	return string(gate.RankReason(gate.ReasonInputs{
		Model:             s.model,
		DegradedParse:     prompt.IsDegradedParse(s.result),
		DeliberationLimit: s.deliberationLimit(),
	}))
}

// inconclusiveReason ranks why this run's model/analysis half cannot be
// trusted (gate.RankReason, the one ranking both sources use).
func (s *reviewState) inconclusiveReason() string {
	return string(gate.RankReason(gate.ReasonInputs{
		Model:             s.model,
		DegradedParse:     prompt.IsDegradedParse(s.result),
		ScannerReason:     s.scannerReason(),
		PartialCoverage:   s.coverage.partial(),
		DeliberationLimit: s.deliberationLimit(),
	}))
}

// runGate is the gate phase of every session: it fills the shared gate.Run
// from the finished analyses, runs the one declared pipeline and publishes
// its decision lines. The repository identity is the source's verified one,
// never anything derived from the model or the diff. A contributor may
// replace the filter and the writers (a secret learned mid-run); everything
// after the gate writes through them.
func (s *reviewState) runGate() (int, bool) {
	s.verifyModelFindings()
	run := s.run
	run.Ctx, run.Cfg, run.Diff, run.Review = s.ctx, s.cfg, s.diff, s.result
	run.Extra, run.Security, run.Language = s.securityApart(), s.securityFindings, s.reviewLanguage
	run.Filter, run.Stdout, run.Stderr = s.filter, s.stdout, s.stderr
	run.RepoIdentity, run.RepoIdentityKnown, run.Now = s.repoIdentity, s.repoIdentityKnown, s.deps.clock
	run.Triage = s.triage()
	pipeline := assembleGatePipeline(s.gatePipelineInputs())
	reason := s.inconclusiveReason()
	res, ok := s.executeGate(pipeline, reason)
	if !ok {
		return 2, true
	}
	s.gateRes = res
	s.filter, s.stdout, s.stderr = run.Filter, run.Stdout, run.Stderr
	applyGateOutcome(run, res)
	s.reportTriage(run.Demoted)
	s.reportTriageNotRun(res)
	return 0, false
}

// triage is what the model's assessment may change in this run's gate.
// A model that did not answer cleanly (no provider, a failed or degraded
// parse, a deliberation limit) never demotes: one degraded profile or batch
// beside healthy ones leaves the whole consolidated answer untrusted. Only
// a dispute with a justification is recorded (review.DisputeCounts), and
// only for the sources triageSources lets the model demote.
func (s *reviewState) triage() gate.Triage {
	t := gate.Triage{Disputed: map[string]bool{}, BySource: map[string]bool{}}
	if s.modelReason() != "" {
		return t
	}
	for _, issue := range s.disputedEvidence() {
		if review.DisputeCounts(issue.Assessment) {
			t.Disputed[gate.DisputeKey(issue.Origin, issue.RuleID, issue.File, issue.Line)] = true
		}
	}
	for source, byModel := range s.triageSources() {
		t.BySource[source] = byModel
	}
	return t
}

// triageSources is, per gate.sources name, whether the repository lets the
// model's dispute demote that source's evidence: with a declared gate and
// without a central policy, gate.triage decides (model unless it wrote
// none); with no declared gate, or under a central policy, nothing is
// demoted, whatever either configuration says (nil): evidence of policy
// origin always counts.
func (s *reviewState) triageSources() map[string]bool {
	if s.centralCfg != nil || s.cfg == nil || !s.cfg.Gate.Declared() {
		return nil
	}
	sources := map[string]bool{}
	for _, source := range []string{config.GateSourceSkills, config.GateSourceAnalysis} {
		sources[source] = s.cfg.Gate.TriageByModel(source)
	}
	// Engines of one category share its key: an explicit none on any of
	// them keeps the whole category counting (fail closed).
	for _, scan := range s.scans {
		byModel := s.cfg.Gate.TriageByModelFor(scan.Engine.Answers)
		if prior, seen := sources[scan.Source()]; seen {
			byModel = byModel && prior
		}
		sources[scan.Source()] = byModel
	}
	return sources
}

// reportTriage states every finding a dispute demoted, and proposes an
// exception for every disputed finding that still counts.
func (s *reviewState) reportTriage(demoted []gate.Demotion) {
	s.triageDemoted = len(demoted)
	for _, d := range demoted {
		line := fmt.Sprintf("gate.triage (%s: model): %s:%d %s contestado pelo modelo deixou de contar", d.Source, d.Issue.File, d.Issue.Line, d.Issue.RuleID)
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
		s.result.Limitations = append(s.result.Limitations, line)
	}
	// Without a declared gate nothing counts, so there is nothing to except.
	if !s.cfg.Gate.Declared() {
		return
	}
	s.proposedExceptions = gate.RenderProposedExceptions(gate.ProposeExceptions(s.disputedEvidence(), demoted, s.repoIdentity))
}

// gatePipelineInputs gathers what the contributors capture besides the run.
func (s *reviewState) gatePipelineInputs() gatePipelineInputs {
	return gatePipelineInputs{
		VerdictKey: gateVerdictKeyInputs{
			ContextKey:     s.contextCacheKey,
			PolicyDigest:   render.PolicyDigest(s.policyDir, s.centralCfg),
			BinaryIdentity: binaryIdentity(),
			RepoIdentity:   s.repoIdentity,
			DiffDigest:     gate.DiffContentDigest(s.diff),
			ReviewedSHA:    s.env().githubSHA,
		},
		RawIssues:      s.rawIssues,
		AcceptedOrigin: acceptedGateOrigin(s.centralCfg != nil),
		DynamicRules:   s.dynamicRules,
		Scans:          s.scans,
		Dependencies:   s.dependencyReportForGate(),
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

// blockingRule is what this run's review calls blocking: the declared
// gate's decision, or the historical severity reading without a gate.
func (s *reviewState) blockingRule() blocking.Rule {
	if s.gateRes == nil || s.cfg == nil {
		return blocking.Ungated()
	}
	return blocking.FromGate(s.cfg.Gate.Declared(), *s.gateRes)
}

// modelFindingBlocks reports whether the model's issue, alone, would block
// this run: at or above --fail-on, failing the declared gate (its own
// decision over the issue, exceptions included), or an error or warning
// when no gate is declared. Only such findings are verified.
func (s *reviewState) modelFindingBlocks(issue types.ReviewIssue) bool {
	if s.threshold > 0 && severityRank(issue.Severity) >= s.threshold {
		return true
	}
	if s.cfg == nil || !s.cfg.Gate.Declared() {
		return blocking.Ungated().Blocks(issue)
	}
	res, err := gate.EvaluateGate(s.cfg.Gate, acceptedGateOrigin(s.centralCfg != nil), s.dynamicRules,
		[]types.ReviewIssue{issue}, "", s.cfg.Exceptions, s.repoIdentity, s.deps.clock())
	return err != nil || res.Fail
}
