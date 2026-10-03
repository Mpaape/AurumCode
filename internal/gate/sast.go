package gate

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// ApplySASTGate folds quality_gates.sast's own, independent decision into
// an already-computed Result (EvaluateGate's return value,
// policygate.go) IN PLACE. It never calls or changes EvaluateGate: SAST's
// Active/Fail/Breach/Inconclusive are computed from sast/issues/reason
// alone, so a config with quality_gates.sast but NO `gate:` section at all
// still publishes a status, withholds approval and writes an audit/SARIF
// record naming the breach -- exactly the publication path AUR-519/520/
// 521 already built, now driven by a second, independent gate. Calling
// this when sast.IsEnabled() is false is a no-op (d is left exactly as
// EvaluateGate returned it).
//
//   - sast is the already precedence-resolved SastConfig
//     (repoCfg.QualityGates.Sast, after config.ApplyCentralPolicy already
//     picked policy-over-repo wholesale when a policy is in play -- see
//     that function's own comment: there is only ever one effective SAST
//     section per run, never a repo-and-policy merge, so this function
//     takes no separate "accepted origin" filter).
//   - origin names, for the published line only, whether this run's
//     effective section came from the central policy or the repository's
//     own opt-in (the same test runReview/runPRReview already does for
//     AUR-519's gateOrigin: centralCfg != nil means "policy").
//   - issues is runSASTPass's own, Semgrep-sourced slice ONLY -- never
//     result.Issues or any other slice the model's JSON response could
//     have contributed to -- so a model reply that tries to recite,
//     contradict or omit a Semgrep finding has no bearing on this
//     decision (AC-005: the model may explain, never remove or
//     downgrade a Semgrep finding).
//   - reason is runSASTPass's own inconclusive-reason token ("" for a
//     trustworthy scan, including zero findings).
func ApplySASTGate(d *Result, sast *config.SastConfig, origin string, issues []types.ReviewIssue, reason string) error {
	if !sast.IsEnabled() {
		return nil
	}
	d.Active = true
	if reason != "" {
		d.Inconclusive = true
		d.Lines = append(d.Lines, fmt.Sprintf("SAST (semgrep, origem %s, secao %s) inconclusivo (%s)", OriginSAST, origin, reason))
		return nil
	}
	rank, name, err := sast.Threshold()
	if err != nil {
		// config.Parse already validates fail_on_severity before any run
		// reaches here; this is defensive only.
		return err
	}
	for _, issue := range issues {
		issueRank, ok := SeverityRankOf(issue.Severity)
		if !ok || issueRank < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, FindingLine(issue.RuleID, issue.Message, issue.Severity, name, OriginSAST+", secao "+origin))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID:   issue.RuleID,
			Path:     issue.File,
			Line:     issue.Line,
			Severity: issue.Severity,
			Origin:   OriginSAST,
		})
	}
	return nil
}
