// The Contributor adapters of the gate pipeline. They only connect the
// existing gate sources (policygate.go, gate_sources.go, aur548.go,
// aur550.go, aur533.go, aur520.go, aur524.go) to gate.Contributor: no
// decision is made here.
package main

import (
	"context"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	contributorExceptions  = "exceptions"
	contributorVerdict     = "verdict-reuse"
	contributorSkills      = "policy-skills"
	contributorSAST        = "sast"
	contributorAnalysis    = "embedded-analysis"
	contributorAnalysisDat = "analysis-data"
	contributorDTrack      = "dependency-track"
)

// exceptionsContributor declares, when exceptions are configured but the
// repository identity could not be verified, that no exception can match
// (AUR-520). The matching itself is applied by the skills and analysis
// contributors, where a finding is judged.
type exceptionsContributor struct{}

func (exceptionsContributor) Name() string   { return contributorExceptions }
func (exceptionsContributor) Origin() string { return "exceptions" }
func (exceptionsContributor) Apply(_ context.Context, run *gate.Run, _ *gate.Result) error {
	if run.RepoIdentityKnown || !exceptionsConfigured(run.Cfg) {
		return nil
	}
	notice := repoIdentityUnavailableNotice(run.Language)
	fmt.Fprintf(run.Stderr, "aurumcode review: %s\n", notice)
	run.Review.Limitations = append(run.Review.Limitations, notice)
	return nil
}

// verdictReuseContributor reuses or publishes a concluded gate verdict
// (AUR-524).
type verdictReuseContributor struct {
	provider llm.Provider
	key      gateVerdictKeyInputs
	raw      []types.ReviewIssue
}

func (verdictReuseContributor) Name() string   { return contributorVerdict }
func (verdictReuseContributor) Origin() string { return "verdict-cache" }
func (c verdictReuseContributor) Apply(_ context.Context, run *gate.Run, res *gate.Result) error {
	digest, digestErr := newCacheDigestBuilder().FixedContentDigest()
	c.key.PromptVersionDigest = digest
	run.Review.Issues = reuseOrStoreGateVerdict(run.Stderr, &run.Review.Limitations, run.Cfg.Gate.Declared(), digestErr == nil, c.provider, c.key, run.Cfg, res.Reason, run.Review.Issues, c.raw)
	return nil
}

// policySkillsContributor evaluates the policy's skill sections (AUR-519).
type policySkillsContributor struct {
	origin  string
	dynamic map[string]review.Rule
}

func (policySkillsContributor) Name() string   { return contributorSkills }
func (policySkillsContributor) Origin() string { return gateOriginSkills }
func (c policySkillsContributor) Apply(_ context.Context, run *gate.Run, res *gate.Result) error {
	d, err := evaluateGate(run.Cfg.Gate, c.origin, c.dynamic, run.IssuesForGate(), res.Reason, run.Cfg.Exceptions, run.RepoIdentity, run.Clock())
	if err != nil {
		return gate.Fatal(err)
	}
	d.Reason, d.Trail = res.Reason, res.Trail
	*res = d
	return nil
}

// sastContributor folds quality_gates.sast's own decision (AUR-548).
type sastContributor struct {
	origin string
	issues []types.ReviewIssue
	reason string
}

func (sastContributor) Name() string   { return contributorSAST }
func (sastContributor) Origin() string { return gateOriginSAST }
func (c sastContributor) Apply(_ context.Context, run *gate.Run, res *gate.Result) error {
	return gate.Fatal(applySASTGate(res, run.Cfg.QualityGates.Sast, c.origin, gateSASTIssues(run.Cfg.Gate, c.issues), c.reason))
}

// embeddedAnalysisContributor counts the embedded analysis catalog (AUR-556).
type embeddedAnalysisContributor struct{}

func (embeddedAnalysisContributor) Name() string   { return contributorAnalysis }
func (embeddedAnalysisContributor) Origin() string { return gateOriginAnalysis }
func (embeddedAnalysisContributor) Apply(_ context.Context, run *gate.Run, res *gate.Result) error {
	return gate.Fatal(applyAnalysisGate(res, run.Cfg.Gate, analysisIssuesForGate(run.Diff, run.Cfg), run.Cfg.Exceptions, run.RepoIdentity, run.Clock()))
}

// analysisDataContributor gates the analysis-data artifact (AUR-533).
type analysisDataContributor struct{}

func (analysisDataContributor) Name() string   { return contributorAnalysisDat }
func (analysisDataContributor) Origin() string { return "analysis-data" }
func (analysisDataContributor) Apply(ctx context.Context, run *gate.Run, res *gate.Result) error {
	mode, _ := run.Cfg.Gate.InconclusiveMode()
	adResult, adReason, adAudit := applyAnalysisDataGate(ctx, run.Cfg.AnalysisData, mode)
	merged, reason := mergeDTrackGate(*res, res.Reason, adResult, adReason)
	trail := res.Trail
	*res = merged
	res.Reason, res.Trail, res.AnalysisData = reason, trail, adAudit
	return nil
}

// dependencyTrackContributor gates the Dependency-Track submission (AUR-550).
// When it learns a secret it replaces the run's redaction filter and wraps
// its writers; the path reads them back from the run.
type dependencyTrackContributor struct{}

func (dependencyTrackContributor) Name() string   { return contributorDTrack }
func (dependencyTrackContributor) Origin() string { return "dependency-track" }
func (dependencyTrackContributor) Apply(ctx context.Context, run *gate.Run, res *gate.Result) error {
	mode, _ := run.Cfg.Gate.InconclusiveMode()
	dtResult, dtReason, nextFilter := applyDTrackGate(ctx, run.Cfg.QualityGates.SsorDtrack, mode, run.Filter)
	merged, reason := mergeDTrackGate(*res, res.Reason, dtResult, dtReason)
	analysis, trail := res.AnalysisData, res.Trail
	*res = merged
	res.Reason, res.Trail, res.AnalysisData = reason, trail, analysis
	if nextFilter != run.Filter {
		run.Filter = nextFilter
		if wrapped, w := wrapWriterWithFilter(redaction.SinkStderr, run.Stderr, run.Filter); w != nil {
			run.Stderr = wrapped
			run.OnFlush(func() { w.Flush() })
		}
		if wrapped, w := wrapWriterWithFilter(redaction.SinkStdout, run.Stdout, run.Filter); w != nil {
			run.Stdout = wrapped
			run.OnFlush(func() { w.Flush() })
		}
	}
	return nil
}
