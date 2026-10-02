// The embedded analysis catalog (analysis/*) counts toward the
// policy gate. Like ApplySASTGate (aur548.go) it folds its own decision
// into the Result EvaluateGate already returned, so EvaluateGate's
// signature and every AUR-519/520/521/548 consumer stay untouched. The
// findings come from a fresh, deterministic analysis.Runner pass over the
// diff -- never from result.Issues, where a model reply could forge or
// omit an "analysis/*" id -- and are subject to the same evidence gate
// (they are produced from the diff's own added lines), the same rule
// config (config.ApplyRuleConfig) and the same AUR-520 exceptions as any
// other finding. With no gate declared, nothing here runs.
package gate

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// OriginAnalysis labels findings of the embedded analysis catalog.
const (
	OriginAnalysis = "analysis"
	OriginSkills   = "skills"
	OriginSAST     = "sast"
)

// FindingOriginKey identifies a finding for origin lookup.
func FindingOriginKey(ruleID, path string, line int) string {
	return fmt.Sprintf("%s|%s|%d", ruleID, path, line)
}

// AnalysisIssuesForGate runs the embedded catalog over diff and applies the
// effective rule config, exactly as the review's own merge pass does.
func AnalysisIssuesForGate(diff *types.Diff, cfg *config.Config) []types.ReviewIssue {
	if diff == nil {
		return nil
	}
	var out []types.ReviewIssue
	for _, f := range analysis.NewRunner().Analyze(diff) {
		out = append(out, types.ReviewIssue{
			File: f.Path, Line: f.Line, Side: f.Side, Severity: f.Severity, RuleID: f.RuleID,
			Message: fmt.Sprintf("%s (rule %s)", f.Message, f.RuleID),
		})
	}
	return config.ApplyRuleConfig(out, cfg)
}

// SASTIssues returns the SAST issues ApplySASTGate may count: none when
// a declared gate restricts gate.sources to exclude sast.
func SASTIssues(gate config.GateConfig, issues []types.ReviewIssue) []types.ReviewIssue {
	if gate.Declared() && !gate.SourceEnabled(config.GateSourceSAST) {
		return nil
	}
	return issues
}

// ApplyAnalysisGate folds the analysis origin into d in place. A complete
// no-op unless the gate is declared, has a severity threshold and does not
// exclude the analysis source via gate.sources.
func ApplyAnalysisGate(d *Result, gate config.GateConfig, issues []types.ReviewIssue, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) error {
	if !gate.Declared() || !gate.SourceEnabled(config.GateSourceAnalysis) {
		return nil
	}
	rank, name, ok, err := gate.Threshold()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	d.Active = true
	for _, issue := range issues {
		if !strings.HasPrefix(issue.RuleID, "analysis/") {
			continue
		}
		if exc, status := MatchException(exceptions, repoIdentity, issue.RuleID, issue.File, now); status != ExceptionNone {
			switch status {
			case ExceptionActive:
				d.Lines = append(d.Lines, AcceptedExceptionLine(exc, issue))
				d.AppliedExceptions = append(d.AppliedExceptions, render.AuditException{
					RuleID:        issue.RuleID,
					Path:          issue.File,
					Justification: fmt.Sprintf("dono: %s, motivo: %s, validade: %s", exc.Owner, exc.Reason, exc.Expires),
				})
				continue
			case ExceptionExpired:
				d.Lines = append(d.Lines, ExpiredExceptionLine(exc, issue))
			}
		}
		issueRank, ok := SeverityRankOf(issue.Severity)
		if !ok || issueRank < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, fmt.Sprintf("%s: %s (severidade %s, limiar %s, origem %s)", issue.RuleID, issue.Message, issue.Severity, name, OriginAnalysis))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID: issue.RuleID, Path: issue.File, Line: issue.Line, Severity: issue.Severity,
			Origin: OriginAnalysis,
		})
	}
	return nil
}

// FoldSources is the single entry point both review paths (--base and
// --pr) call after EvaluateGate: it folds the SAST and embedded-analysis
// origins into d, honoring gate.sources. Which origins are enabled is
// decided by config.GateConfig.SourceEnabled; this function only adapts
// the results to the cmd-level Result.
func FoldSources(d *Result, cfg *config.Config, diff *types.Diff, sastOrigin string, sastIssues []types.ReviewIssue, sastReason, repoIdentity string, now time.Time) error {
	if err := ApplySASTGate(d, cfg.QualityGates.Sast, sastOrigin, SASTIssues(cfg.Gate, sastIssues), sastReason); err != nil {
		return err
	}
	return ApplyAnalysisGate(d, cfg.Gate, AnalysisIssuesForGate(diff, cfg), cfg.Exceptions, repoIdentity, now)
}
