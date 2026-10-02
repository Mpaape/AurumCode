// The embedded analysis catalog (analysis/*) counts toward the
// policy gate. Like applySASTGate (aur548.go) it folds its own decision
// into the gateDecision evaluateGate already returned, so evaluateGate's
// signature and every AUR-519/520/521/548 consumer stay untouched. The
// findings come from a fresh, deterministic analysis.Runner pass over the
// diff -- never from result.Issues, where a model reply could forge or
// omit an "analysis/*" id -- and are subject to the same evidence gate
// (they are produced from the diff's own added lines), the same rule
// config (config.ApplyRuleConfig) and the same AUR-520 exceptions as any
// other finding. With no gate declared, nothing here runs.
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// gateOriginAnalysis labels findings of the embedded analysis catalog.
const gateOriginAnalysis = "analysis"

// findingOriginKey is the key of gateDecision.FindingOrigins.
func findingOriginKey(ruleID, path string, line int) string {
	return fmt.Sprintf("%s|%s|%d", ruleID, path, line)
}

// analysisIssuesForGate runs the embedded catalog over diff and applies the
// effective rule config, exactly as the review's own merge pass does.
func analysisIssuesForGate(diff *types.Diff, cfg *config.Config) []types.ReviewIssue {
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

// gateSASTIssues returns the SAST issues applySASTGate may count: none when
// a declared gate restricts gate.sources to exclude sast.
func gateSASTIssues(gate config.GateConfig, issues []types.ReviewIssue) []types.ReviewIssue {
	if gate.Declared() && !gate.SourceEnabled(config.GateSourceSAST) {
		return nil
	}
	return issues
}

// applyAnalysisGate folds the analysis origin into d in place. A complete
// no-op unless the gate is declared, has a severity threshold and does not
// exclude the analysis source via gate.sources.
func applyAnalysisGate(d *gateDecision, gate config.GateConfig, issues []types.ReviewIssue, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) error {
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
		if exc, status := matchException(exceptions, repoIdentity, issue.RuleID, issue.File, now); status != exceptionNone {
			switch status {
			case exceptionActive:
				d.Lines = append(d.Lines, acceptedExceptionLine(exc, issue))
				d.AppliedExceptions = append(d.AppliedExceptions, render.AuditException{
					RuleID:        issue.RuleID,
					Path:          issue.File,
					Justification: fmt.Sprintf("dono: %s, motivo: %s, validade: %s", exc.Owner, exc.Reason, exc.Expires),
				})
				continue
			case exceptionExpired:
				d.Lines = append(d.Lines, expiredExceptionLine(exc, issue))
			}
		}
		issueRank, ok := severityRankOf(issue.Severity)
		if !ok || issueRank < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, fmt.Sprintf("%s: %s (severidade %s, limiar %s, origem %s)", issue.RuleID, issue.Message, issue.Severity, name, gateOriginAnalysis))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID: issue.RuleID, Path: issue.File, Line: issue.Line, Severity: issue.Severity,
		})
		if d.FindingOrigins == nil {
			d.FindingOrigins = map[string]string{}
		}
		d.FindingOrigins[findingOriginKey(issue.RuleID, issue.File, issue.Line)] = gateOriginAnalysis
	}
	return nil
}

// foldGateSources is the single entry point both review paths (--base and
// --pr) call after evaluateGate: it folds the SAST and embedded-analysis
// origins into d, honoring gate.sources. Which origins are enabled is
// decided by config.GateConfig.SourceEnabled; this function only adapts
// the results to the cmd-level gateDecision.
func foldGateSources(d *gateDecision, cfg *config.Config, diff *types.Diff, sastOrigin string, sastIssues []types.ReviewIssue, sastReason, repoIdentity string, now time.Time) error {
	if err := applySASTGate(d, cfg.QualityGates.Sast, sastOrigin, gateSASTIssues(cfg.Gate, sastIssues), sastReason); err != nil {
		return err
	}
	return applyAnalysisGate(d, cfg.Gate, analysisIssuesForGate(diff, cfg), cfg.Exceptions, repoIdentity, now)
}
