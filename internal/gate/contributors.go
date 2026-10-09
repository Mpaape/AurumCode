// The Contributors of the gate pipeline. Each one connects a gate source of
// this package (policy.go, scanner.go, sources.go, analysisdata.go, dtrack.go,
// exceptions.go, verdict.go) to the Contributor interface: no decision is
// made here. What a source needs from the command (the prompt digest, the
// review-context key) is injected by the pipeline's assembly.
package gate

import (
	"context"
	"fmt"
	"github.com/Mpaape/AurumCode/internal/analysis"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	ContributorExceptions   = "exceptions"
	ContributorVerdict      = "verdict-reuse"
	ContributorSkills       = "policy-skills"
	ContributorScanners     = "scanners"
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
func (ExceptionsContributor) Apply(_ context.Context, run *Run, _ Result) (Result, error) {
	if run.RepoIdentityKnown || !ExceptionsConfigured(run.Cfg) {
		return Result{}, nil
	}
	notice := RepoIdentityUnavailableNotice(run.Language)
	fmt.Fprintf(run.Stderr, "aurumcode review: %s\n", notice)
	run.Review.Limitations = append(run.Review.Limitations, notice)
	return Result{}, nil
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
func (c VerdictReuseContributor) Apply(_ context.Context, run *Run, prior Result) (Result, error) {
	digest, digestErr := c.PromptDigest()
	c.Key.PromptVersionDigest = digest
	run.Review.Issues = ReuseOrStoreGateVerdict(run.Language, run.Stderr, &run.Review.Limitations, run.Cfg.Gate.Declared(), digestErr == nil, c.Key, run.Cfg, prior.Reason, run.Review.Issues, c.Raw)
	return Result{}, nil
}

// PolicySkillsContributor evaluates the policy's skill sections (AUR-519).
type PolicySkillsContributor struct {
	AcceptedOrigin string
	Dynamic        map[string]review.Rule
}

func (PolicySkillsContributor) Name() string   { return ContributorSkills }
func (PolicySkillsContributor) Origin() string { return OriginSkills }
func (c PolicySkillsContributor) Apply(_ context.Context, run *Run, prior Result) (Result, error) {
	issues := run.IssuesForGate()
	if c.AcceptedOrigin != OriginPolicy {
		issues = run.keepSkills(issues, func(id string) bool { _, ok := c.Dynamic[id]; return ok })
	}
	d, err := EvaluateGateIn(run.Language, run.Cfg.Gate, c.AcceptedOrigin, c.Dynamic, issues, prior.Reason, run.Cfg.Exceptions, run.RepoIdentity, run.Clock())
	if err != nil {
		return Result{}, Fatal(err)
	}
	// The inconclusive reason the gate was judged against is already the
	// run's; the partial adds only the decision.
	d.Reason = ""
	return d, nil
}

// ScannerContributor folds every configured scanner engine's own decision,
// in declaration order. It knows no engine: what differs between engines is
// data in each Scan.
type ScannerContributor struct {
	Scans []Scan
}

func (ScannerContributor) Name() string   { return ContributorScanners }
func (ScannerContributor) Origin() string { return ContributorScanners }
func (c ScannerContributor) Apply(_ context.Context, run *Run, _ Result) (Result, error) {
	var part Result
	for _, scan := range c.Scans {
		var issues []types.ReviewIssue
		if scan.countsUnder(run.Cfg.Gate) {
			issues = scan.Issues
		}
		if scan.Section != OriginPolicy {
			issues = run.keep(scan.Source(), scan.Origin(), issues)
		}
		if err := ApplyScannerGateIn(run.Language, &part, scan, issues); err != nil {
			return part, Fatal(err)
		}
	}
	return part, nil
}

// EmbeddedAnalysisContributor counts the embedded analysis catalog (AUR-556).
type EmbeddedAnalysisContributor struct{}

func (EmbeddedAnalysisContributor) Name() string   { return ContributorAnalysis }
func (EmbeddedAnalysisContributor) Origin() string { return OriginAnalysis }
func (EmbeddedAnalysisContributor) Apply(_ context.Context, run *Run, _ Result) (Result, error) {
	var part Result
	return part, Fatal(ApplyAnalysisGate(&part, run.Cfg.Gate, run.keep(config.GateSourceAnalysis, OriginAnalysis, analysis.LocalizeIssues(run.Language, AnalysisIssuesForGate(run.Diff, run.Cfg))), run.Cfg.Exceptions, run.RepoIdentity, run.Clock()))
}

// SecurityPassContributor counts the --seguranca pass's deterministic findings
// (AUR-569). Without it a security finding reaches the gate only when a policy
// skill happens to cite the same rule id.
type SecurityPassContributor struct{}

func (SecurityPassContributor) Name() string   { return ContributorSecurity }
func (SecurityPassContributor) Origin() string { return OriginSecurity }
func (SecurityPassContributor) Apply(_ context.Context, run *Run, _ Result) (Result, error) {
	var part Result
	return part, Fatal(ApplySecurityGate(&part, run.Cfg.Gate, run.keep(config.GateSourceAnalysis, OriginSecurity, config.ApplyRuleConfig(run.Security, run.Cfg)), run.Cfg.Exceptions, run.RepoIdentity, run.Clock()))
}

// AnalysisDataContributor gates the analysis-data artifact (AUR-533).
type AnalysisDataContributor struct{}

func (AnalysisDataContributor) Name() string   { return ContributorAnalysisData }
func (AnalysisDataContributor) Origin() string { return OriginAnalysisData }
func (AnalysisDataContributor) Apply(ctx context.Context, run *Run, _ Result) (Result, error) {
	part, reason, audit := ApplyAnalysisDataGate(ctx, run.Language, run.Cfg.AnalysisData)
	part.Reason, part.AnalysisData = reason, audit
	return part, nil
}

// DependencyTrackContributor gates the Dependency-Track submission (AUR-550).
// When it learns a secret it replaces the run's redaction filter and wraps
// its writers; the path reads them back from the run.
type DependencyTrackContributor struct{}

func (DependencyTrackContributor) Name() string   { return ContributorDTrack }
func (DependencyTrackContributor) Origin() string { return OriginDTrack }
func (DependencyTrackContributor) Apply(ctx context.Context, run *Run, _ Result) (Result, error) {
	part, reason, nextFilter := ApplyDTrackGate(ctx, run.Language, run.Cfg.QualityGates.SsorDtrack, run.Filter)
	part.Reason = reason
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
	return part, nil
}
