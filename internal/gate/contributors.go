// The Contributors of the gate pipeline. Each one connects a gate source of
// this package (policy.go, sast.go, sources.go, analysisdata.go, dtrack.go,
// exceptions.go, verdict.go) to the Contributor interface: no decision is
// made here. What a source needs from the command (the prompt digest, the
// review-context key) is injected by the pipeline's assembly.
package gate

import (
	"context"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	ContributorExceptions   = "exceptions"
	ContributorVerdict      = "verdict-reuse"
	ContributorSkills       = "policy-skills"
	ContributorSAST         = "sast"
	ContributorAnalysis     = "embedded-analysis"
	ContributorSecurity     = "security-pass"
	ContributorAnalysisData = "analysis-data"
	ContributorDTrack       = "dependency-track"
)

// ExceptionsContributor declares, when exceptions are configured but the
// repository identity could not be verified, that no exception can match
// (AUR-520). The matching itself is applied by the skills and analysis
// contributors, where a finding is judged.
type ExceptionsContributor struct{}

func (ExceptionsContributor) Name() string   { return ContributorExceptions }
func (ExceptionsContributor) Origin() string { return "exceptions" }
func (ExceptionsContributor) Apply(_ context.Context, run *Run, _ *Result) error {
	if run.RepoIdentityKnown || !ExceptionsConfigured(run.Cfg) {
		return nil
	}
	notice := RepoIdentityUnavailableNotice(run.Language)
	fmt.Fprintf(run.Stderr, "aurumcode review: %s\n", notice)
	run.Review.Limitations = append(run.Review.Limitations, notice)
	return nil
}

// VerdictReuseContributor reuses or publishes a concluded gate verdict
// (AUR-524).
type VerdictReuseContributor struct {
	Key VerdictKeyInputs
	Raw []types.ReviewIssue
	// PromptDigest yields the digest of the fixed prompt content; the
	// command owns the prompt builder.
	PromptDigest func() (string, error)
}

func (VerdictReuseContributor) Name() string   { return ContributorVerdict }
func (VerdictReuseContributor) Origin() string { return "verdict-cache" }
func (c VerdictReuseContributor) Apply(_ context.Context, run *Run, res *Result) error {
	digest, digestErr := c.PromptDigest()
	c.Key.PromptVersionDigest = digest
	run.Review.Issues = ReuseOrStoreGateVerdict(run.Stderr, &run.Review.Limitations, run.Cfg.Gate.Declared(), digestErr == nil, c.Key, run.Cfg, res.Reason, run.Review.Issues, c.Raw)
	return nil
}

// PolicySkillsContributor evaluates the policy's skill sections (AUR-519).
type PolicySkillsContributor struct {
	AcceptedOrigin string
	Dynamic        map[string]review.Rule
}

func (PolicySkillsContributor) Name() string   { return ContributorSkills }
func (PolicySkillsContributor) Origin() string { return OriginSkills }
func (c PolicySkillsContributor) Apply(_ context.Context, run *Run, res *Result) error {
	d, err := EvaluateGate(run.Cfg.Gate, c.AcceptedOrigin, c.Dynamic, run.IssuesForGate(), res.Reason, run.Cfg.Exceptions, run.RepoIdentity, run.Clock())
	if err != nil {
		return Fatal(err)
	}
	d.Reason, d.Trail = res.Reason, res.Trail
	*res = d
	return nil
}

// SASTContributor folds quality_gates.sast's own decision (AUR-548).
type SASTContributor struct {
	SectionOrigin string
	Issues        []types.ReviewIssue
	Reason        string
}

func (SASTContributor) Name() string   { return ContributorSAST }
func (SASTContributor) Origin() string { return OriginSAST }
func (c SASTContributor) Apply(_ context.Context, run *Run, res *Result) error {
	return Fatal(ApplySASTGate(res, run.Cfg.QualityGates.Sast, c.SectionOrigin, SASTIssues(run.Cfg.Gate, c.Issues), c.Reason))
}

// EmbeddedAnalysisContributor counts the embedded analysis catalog (AUR-556).
type EmbeddedAnalysisContributor struct{}

func (EmbeddedAnalysisContributor) Name() string   { return ContributorAnalysis }
func (EmbeddedAnalysisContributor) Origin() string { return OriginAnalysis }
func (EmbeddedAnalysisContributor) Apply(_ context.Context, run *Run, res *Result) error {
	return Fatal(ApplyAnalysisGate(res, run.Cfg.Gate, AnalysisIssuesForGate(run.Diff, run.Cfg), run.Cfg.Exceptions, run.RepoIdentity, run.Clock()))
}

// SecurityPassContributor counts the --seguranca pass's deterministic findings
// (AUR-569). Without it a security finding reaches the gate only when a policy
// skill happens to cite the same rule id.
type SecurityPassContributor struct{}

func (SecurityPassContributor) Name() string   { return ContributorSecurity }
func (SecurityPassContributor) Origin() string { return OriginSecurity }
func (SecurityPassContributor) Apply(_ context.Context, run *Run, res *Result) error {
	return Fatal(ApplySecurityGate(res, run.Cfg.Gate, config.ApplyRuleConfig(run.Security, run.Cfg), run.Cfg.Exceptions, run.RepoIdentity, run.Clock()))
}

// AnalysisDataContributor gates the analysis-data artifact (AUR-533).
type AnalysisDataContributor struct{}

func (AnalysisDataContributor) Name() string   { return ContributorAnalysisData }
func (AnalysisDataContributor) Origin() string { return "analysis-data" }
func (AnalysisDataContributor) Apply(ctx context.Context, run *Run, res *Result) error {
	mode, _ := run.Cfg.Gate.InconclusiveMode()
	adResult, adReason, adAudit := ApplyAnalysisDataGate(ctx, run.Cfg.AnalysisData, mode)
	merged, reason := MergeDTrackGate(*res, res.Reason, adResult, adReason)
	trail := res.Trail
	*res = merged
	res.Reason, res.Trail, res.AnalysisData = reason, trail, adAudit
	return nil
}

// DependencyTrackContributor gates the Dependency-Track submission (AUR-550).
// When it learns a secret it replaces the run's redaction filter and wraps
// its writers; the path reads them back from the run.
type DependencyTrackContributor struct{}

func (DependencyTrackContributor) Name() string   { return ContributorDTrack }
func (DependencyTrackContributor) Origin() string { return "dependency-track" }
func (DependencyTrackContributor) Apply(ctx context.Context, run *Run, res *Result) error {
	mode, _ := run.Cfg.Gate.InconclusiveMode()
	dtResult, dtReason, nextFilter := ApplyDTrackGate(ctx, run.Cfg.QualityGates.SsorDtrack, mode, run.Filter)
	merged, reason := MergeDTrackGate(*res, res.Reason, dtResult, dtReason)
	analysis, trail := res.AnalysisData, res.Trail
	*res = merged
	res.Reason, res.Trail, res.AnalysisData = reason, trail, analysis
	if nextFilter != run.Filter {
		run.Filter = nextFilter
		if wrapped, w := WrapWriterWithFilter(redaction.SinkStderr, run.Stderr, run.Filter); w != nil {
			run.Stderr = wrapped
			run.OnFlush(func() { w.Flush() })
		}
		if wrapped, w := WrapWriterWithFilter(redaction.SinkStdout, run.Stdout, run.Filter); w != nil {
			run.Stdout = wrapped
			run.OnFlush(func() { w.Flush() })
		}
	}
	return nil
}
