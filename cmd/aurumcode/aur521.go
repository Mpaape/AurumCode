// AUR-521: writes the compliance audit record and SARIF document a
// policy-governed review carries out of the process, for a workflow to
// publish the audit as a job artifact and upload the SARIF to GitHub code
// scanning (docs/specs/AUR-521.md). runReview (--base, main.go) and
// runPRReview (--pr, pr.go) call writeComplianceArtifacts once their own
// gate decision (policygate.go) is final, passing in exactly the facts this
// file needs -- it never recomputes the gate itself, only renders its
// already-made decision into the two file formats.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// complianceArtifactInputs bundles AUR-521's write-time inputs so runReview
// and runPRReview call one function instead of each reimplementing the
// audit/SARIF assembly. Every field here is a fact the caller already
// computed for its own, unrelated purpose (the gate, the coverage notice,
// the dynamic rule set); this file adds no new decision of its own.
type complianceArtifactInputs struct {
	auditoriaPath string
	sarifPath     string

	// policyDir/centralCfg feed render.PolicyDigest (AC-001); centralCfg nil
	// means no policy was declared this run, exactly as elsewhere.
	policyDir  string
	centralCfg *config.Config

	repo        string
	reviewedSHA string
	model       string
	verdict     string

	gate                   gateDecision
	gateInconclusiveReason string
	acceptedOrigin         string
	gateConfig             config.GateConfig

	issues       []types.ReviewIssue
	dynamicRules map[string]review.Rule

	coverageComplete bool
	omittedFiles     []string
}

// writeComplianceArtifacts is a complete no-op when neither --auditoria nor
// --sarif was given: today's behavior stays byte-identical. A write failure
// is reported on stderr and never changes the review's own exit code -- the
// compliance trail is additive evidence, never a second way for an already
// finished review to fail.
func writeComplianceArtifacts(in complianceArtifactInputs, filter *redaction.Filter, stderr io.Writer) {
	if in.auditoriaPath == "" && in.sarifPath == "" {
		return
	}

	policyDigest := render.PolicyDigest(in.policyDir, in.centralCfg)
	// AUR-521's own workflow-SHA convention: GITHUB_WORKFLOW_SHA (the
	// reusable workflow's own version, when a caller forwards it) takes
	// precedence over GITHUB_SHA (the commit under review -- a usable, if
	// coarser, fallback when the workflow SHA was never forwarded).
	workflowSHA := firstNonEmpty(os.Getenv("GITHUB_WORKFLOW_SHA"), os.Getenv("GITHUB_SHA"))

	decision, reason := auditGateOutcome(in.gate)
	blocking := matchedBlockingFindings(in.gateConfig, in.acceptedOrigin, in.dynamicRules, in.issues)

	if in.auditoriaPath != "" {
		rec := render.BuildAuditRecord(
			policyDigest, workflowSHA, in.repo, in.reviewedSHA, in.model, in.verdict,
			render.AuditGate{Decision: decision, Reason: reason},
			blocking,
			// AUR-520: exceptions_applied is always an explicit empty list
			// until that card lands and starts populating it (see
			// render.AuditRecord's own doc).
			nil,
			in.coverageComplete, in.omittedFiles,
		)
		if err := render.WriteAuditRecord(in.auditoriaPath, rec, filter); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: writing audit record: %v\n", err)
		}
	}

	if in.sarifPath != "" {
		findings := make([]render.SARIFFinding, 0, len(in.issues))
		for _, issue := range in.issues {
			title := ""
			if rule, ok := in.dynamicRules[issue.RuleID]; ok {
				title = rule.Title
			}
			findings = append(findings, render.SARIFFinding{
				RuleID:    issue.RuleID,
				RuleTitle: title,
				Path:      issue.File,
				Line:      issue.Line,
				Severity:  issue.Severity,
				Message:   issue.Message,
				Context:   issue.Evidence,
			})
		}
		executionSuccessful := in.gateInconclusiveReason == ""
		if err := render.WriteSARIF(in.sarifPath, version, findings, executionSuccessful, in.gateInconclusiveReason, filter); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: writing SARIF: %v\n", err)
		}
	}
}

// auditGateOutcome collapses a gateDecision (policygate.go) into the
// pass/fail/inconclusive decision AC-001's audit record names, mirroring
// publishPolicyGateStatus's own switch (policygate.go) without publishing
// anything: no gate declared, or a gate that ran clean, both read as "pass"
// -- there is nothing for this record to block on.
func auditGateOutcome(g gateDecision) (decision, reason string) {
	reason = strings.Join(g.Lines, "; ")
	switch {
	case !g.Active:
		return "pass", ""
	case g.Fail:
		return "fail", reason
	case g.Inconclusive:
		return "inconclusive", reason
	default:
		return "pass", reason
	}
}

// matchedBlockingFindings re-derives the exact set of issues evaluateGate's
// own threshold loop (policygate.go) would match, as structured
// render.AuditFinding values instead of evaluateGate's own printable lines.
// It deliberately mirrors that loop's matching rule (origin + the same
// rule/model severity floor, effectiveSeverityRank) rather than changing
// evaluateGate's signature to return them: policygate.go is also being
// edited concurrently for AUR-520's exceptions, and this function only
// READS the gate's already-public helpers, so it carries no risk of
// disagreeing with the gate's own decision.
func matchedBlockingFindings(gate config.GateConfig, acceptedOrigin string, dynamic map[string]review.Rule, issues []types.ReviewIssue) []render.AuditFinding {
	out := []render.AuditFinding{}
	if !gate.Declared() {
		return out
	}
	rank, _, ok, err := gate.Threshold()
	if err != nil || !ok {
		return out
	}
	for _, issue := range issues {
		rule, found := dynamic[issue.RuleID]
		if !found || rule.Origin != acceptedOrigin {
			continue
		}
		effective, comparable := effectiveSeverityRank(issue.Severity, rule.Severity)
		if !comparable || effective < rank {
			continue
		}
		out = append(out, render.AuditFinding{
			RuleID:   issue.RuleID,
			Path:     issue.File,
			Line:     issue.Line,
			Severity: issue.Severity,
		})
	}
	return out
}

// firstNonEmpty returns the first non-empty, trimmed value, or "" when all
// are empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
