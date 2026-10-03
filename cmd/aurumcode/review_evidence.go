// The evidence phase steps both sources share: the --seguranca pass, the
// one place that decides where its findings live, the verdict-reuse
// snapshot, the repository's rule configuration and SAST.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// runSecurityPass is the deterministic --seguranca pass (AUR-435/451) over
// the exact diff the model will see. It runs before the model, so its
// findings are evidence the model weighs; it touches no review result. A
// broken rules catalog fails loudly (exit 1).
func (s *reviewState) runSecurityPass() (int, bool) {
	if !s.seguranca {
		return 0, false
	}
	findings, applied, total, err := review.SecurityScanWithCoverage(s.diff)
	if err != nil {
		fmt.Fprintf(s.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	s.securityFindings = withOrigin(findings, gateOriginSecurity)
	s.securityApplied, s.securityTotal = applied, total
	return 0, false
}

// reportSecurityPass prints the pass's own coverage on stderr (AUR-450) and
// places its findings, once the model's answer exists.
func (s *reviewState) reportSecurityPass() {
	if !s.seguranca {
		return
	}
	applied, total := s.securityApplied, s.securityTotal
	printSecurityCoverage(s.stderr, applied, total)
	s.joinSecurityFindings()
}

// joinSecurityFindings is the one place that decides where the security
// pass's findings live: in the review's issues when the source publishes
// them as comments, otherwise kept apart for their own report section.
// Either way securityApart and the snapshot below see each finding once.
func (s *reviewState) joinSecurityFindings() {
	if !s.source.SecurityInIssues || len(s.securityFindings) == 0 {
		return
	}
	combined := make([]types.ReviewIssue, 0, len(s.result.Issues)+len(s.securityFindings))
	combined = append(combined, s.result.Issues...)
	combined = append(combined, s.securityFindings...)
	s.result.Issues = combined
}

// securityApart is the security findings not already in the review's
// issues: what the gate reads beside them.
func (s *reviewState) securityApart() []types.ReviewIssue {
	if s.source.SecurityInIssues {
		return nil
	}
	return s.securityFindings
}

// snapshotAndApplyRules snapshots the RAW findings for verdict reuse
// (AUR-524 v2) -- the model's, the static analysis' and the security
// pass's, the same set on every source -- immediately before this run's
// own rule config filters/overrides them, so a stored verdict can be
// re-evaluated against a later run's rule config.
func (s *reviewState) snapshotAndApplyRules() {
	s.rawIssues = append(append([]types.ReviewIssue(nil), s.result.Issues...), s.securityApart()...)
	s.result.Issues = config.ApplyRuleConfig(s.result.Issues, s.cfg)
	if !s.source.SecurityInIssues {
		s.securityFindings = config.ApplyRuleConfig(s.securityFindings, s.cfg)
	}
}

// runSAST runs quality_gates.sast's Semgrep pass over root (AUR-548),
// before the model: its findings are evidence the model weighs. A
// non-empty blocked reason (an unverified --pr checkout, AUR-515/536)
// makes SAST inconclusive without invoking Semgrep. Its issues never pass
// through config.ApplyRuleConfig (deterministic evidence a `rules:`
// override was never meant to reach). The origin is "policy" only when the
// CENTRAL POLICY ITSELF declares quality_gates.sast.
func (s *reviewState) runSAST(root, blocked string) {
	s.sastOrigin = gateOriginRepo
	if s.centralCfg != nil && s.centralCfg.QualityGates.Sast != nil {
		s.sastOrigin = gateOriginPolicy
	}
	switch {
	case blocked != "" && s.cfg.QualityGates.Sast.IsEnabled():
		s.sastReason = blocked
	case blocked == "":
		s.sastIssues, s.sastReason = runSASTPass(s.ctx, root, s.cfg.QualityGates.Sast, s.sastOrigin == gateOriginPolicy, s.filter, s.deps.semgrep)
	}
	s.sastIssues = withOrigin(s.sastIssues, gateOriginSAST)
}

// joinSAST states an inconclusive SAST pass or joins its issues to the
// review's, once the model's answer exists.
func (s *reviewState) joinSAST() {
	if s.sastReason != "" {
		notice := sastInconclusiveNotice(s.reviewLanguage, s.sastReason)
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", notice)
		s.result.Limitations = append(s.result.Limitations, notice)
	} else if len(s.sastIssues) > 0 {
		s.result.Issues = append(s.result.Issues, s.sastIssues...)
	}
}

// findingsAtThreshold counts the gated findings (the security pass's
// included) at or above --fail-on; 0 when --fail-on is off.
func (s *reviewState) findingsAtThreshold() int {
	if s.threshold <= 0 {
		return 0
	}
	return countAtOrAbove(s.run.IssuesForGate(), s.threshold)
}

// writeArtifacts writes AUR-521's audit record and SARIF from the gate's
// final decision and the gate run's own findings, once per session; a
// no-op unless --auditoria or --sarif was given.
func (s *reviewState) writeArtifacts() []artifactFailure {
	res := s.gateRes
	return writeComplianceArtifacts(complianceArtifactInputs{
		auditoriaPath:          s.auditoriaPath,
		sarifPath:              s.sarifPath,
		policyDir:              s.policyDir,
		centralCfg:             s.centralCfg,
		repo:                   s.artifactRepo,
		reviewedSHA:            s.artifactCommit,
		model:                  firstNonEmpty(s.modelFlag, s.env().llmModel),
		verdict:                canonicalVerdict(s.result),
		gate:                   *res,
		gateInconclusiveReason: res.Reason,
		analysisData:           res.AnalysisData,
		diff:                   s.diff,
		issues:                 s.run.IssuesForGate(),
		dynamicRules:           s.dynamicRules,
		coverageComplete:       !s.coverage.partial(),
		omittedFiles:           append(append([]string{}, s.coverage.IgnoredPaths...), s.coverage.FilteredPaths...),
		proposedExceptions:     s.proposedExceptions,
	}, s.run, res, s.filter, s.stderr)
}
