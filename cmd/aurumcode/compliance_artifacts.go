// Compliance artifacts: the audit record and the SARIF document a
// policy-governed review writes for a workflow to publish, assembled once
// from the gate run for both review sources.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/deliberation"
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

	// deliberation is the tool conversation's transcript, nil when the
	// model was offered no tool.
	deliberation *deliberation.Transcript

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

	// analysisData (AUR-533) is non-nil only when a declared analysis_data
	// section resolved a usable artifact.
	analysisData *render.AnalysisDataAudit

	// diff is the exact diff this run reviewed. The SARIF fingerprint is
	// built from the code AT (issue.File, issue.Line, issue.Side) in this
	// diff (render.FindingIdentityFor) -- never from issue.Message/
	// Evidence, which are the model's own, rewordable account of the
	// finding, not the code itself.
	diff *types.Diff

	issues       []types.ReviewIssue
	dynamicRules map[string]review.Rule

	coverageComplete bool
	omittedFiles     []string

	// proposedExceptions is the text of the exceptions the model's disputes
	// suggest (never applied).
	proposedExceptions string
}

// writeComplianceArtifacts is a complete no-op when neither --auditoria nor
// --sarif was given: today's behavior stays byte-identical. Both review paths
// call it once their gate decision is final and before anything is published
// or any exit code is chosen (AUR-568). A requested artifact that cannot be
// written never ends as success: the failure is named on stderr with its
// path, folded into *res (gate.ApplyArtifactFailures: inconclusive by the
// policy's mode when a gate is declared), and returned so the caller exits
// non-zero. When something failed, the artifacts that were written are
// rewritten once with the final decision so every file agrees with it.
func writeComplianceArtifacts(in complianceArtifactInputs, run *gateRun, res *gateDecision, filter *redaction.Filter, stderr io.Writer) []artifactFailure {
	if in.auditoriaPath == "" && in.sarifPath == "" {
		return nil
	}
	in.gate, in.gateInconclusiveReason = *res, res.Reason
	failures := writeArtifactFiles(in, filter, nil)
	if len(failures) == 0 {
		return nil
	}
	for _, f := range failures {
		fmt.Fprintf(stderr, "aurumcode review: %s: %s: %v\n", f.Reason, f.Path, f.Err)
	}
	applyArtifactFailures(run, res, failures)
	in.gate, in.gateInconclusiveReason = *res, res.Reason
	skip := make(map[string]bool, len(failures))
	for _, f := range failures {
		skip[f.Path] = true
	}
	writeArtifactFiles(in, filter, skip)
	return failures
}

// writeArtifactFiles writes each requested artifact whose path is not in
// skip and returns the failures.
func writeArtifactFiles(in complianceArtifactInputs, filter *redaction.Filter, skip map[string]bool) []artifactFailure {
	var failures []artifactFailure
	if in.auditoriaPath != "" && !skip[in.auditoriaPath] {
		if err := writeAuditFile(in, filter); err != nil {
			failures = append(failures, artifactFailure{Reason: gateReasonAuditWriteFailed, Path: in.auditoriaPath, Err: err})
		}
	}
	if in.sarifPath != "" && !skip[in.sarifPath] {
		if err := writeSARIFFile(in, filter); err != nil {
			failures = append(failures, artifactFailure{Reason: gateReasonSARIFWriteFailed, Path: in.sarifPath, Err: err})
		}
	}
	return failures
}

// gateFacts is the gate's own structured output, defaulted to empty lists.
func gateFacts(g gateDecision) (blocking []render.AuditFinding, exceptions []render.AuditException) {
	// AUR-521/AUR-520: BlockingFindings and AppliedExceptions are
	// evaluateGate's OWN structured output (policygate.go) -- the single
	// place a finding is ever decided to block the gate or be excepted.
	// This file never re-derives that decision a second time.
	blocking, exceptions = g.BlockingFindings, g.AppliedExceptions
	if blocking == nil {
		blocking = []render.AuditFinding{}
	}
	if exceptions == nil {
		exceptions = []render.AuditException{}
	}
	return blocking, exceptions
}

func writeAuditFile(in complianceArtifactInputs, filter *redaction.Filter) error {
	// AUR-521's own workflow-SHA convention: AURUMCODE_WORKFLOW_SHA (the
	// reusable workflow's own version, forwarded under a name GitHub
	// Actions does not reserve). No fallback to GITHUB_SHA here on
	// purpose: a run with no AURUMCODE_WORKFLOW_SHA (a local invocation,
	// outside the reusable workflow) must record an EMPTY workflow_sha,
	// never silently duplicate reviewed_sha's value into it.
	workflowSHA := strings.TrimSpace(os.Getenv("AURUMCODE_WORKFLOW_SHA"))
	decision, reason := auditGateOutcome(in.gate, in.gateInconclusiveReason)
	blocking, exceptions := gateFacts(in.gate)
	rec := render.BuildAuditRecord(
		render.PolicyDigest(in.policyDir, in.centralCfg), workflowSHA, in.repo, in.reviewedSHA, in.model, in.verdict,
		render.AuditGate{Decision: decision, Reason: reason},
		blocking,
		exceptions,
		in.coverageComplete, in.omittedFiles,
	)
	rec.AnalysisData = in.analysisData
	rec.EvidenceAssessments = render.AssessedEvidence(in.issues)
	rec.ProposedExceptions = in.proposedExceptions
	rec.Deliberation = in.deliberation
	return render.WriteAuditRecord(in.auditoriaPath, rec, filter)
}

func writeSARIFFile(in complianceArtifactInputs, filter *redaction.Filter) error {
	blocking, exceptions := gateFacts(in.gate)
	// suppressed indexes AppliedExceptions by (ruleID, path) so the loop
	// below marks the exact same findings evaluateGate excepted as
	// suppressed, with the exact same justification.
	suppressed := make(map[[2]string]render.AuditException, len(exceptions))
	for _, exc := range exceptions {
		suppressed[[2]string{exc.RuleID, exc.Path}] = exc
	}
	findings := make([]render.SARIFFinding, 0, len(in.issues))
	for _, issue := range in.issues {
		title := ""
		if rule, ok := in.dynamicRules[issue.RuleID]; ok {
			title = rule.Title
		}
		identity := render.FindingIdentityFor(in.diff, issue, filter)
		finding := render.SARIFFinding{
			RuleID:    issue.RuleID,
			RuleTitle: title,
			Path:      issue.File,
			Line:      issue.Line,
			Severity:  issue.Severity,
			Message:   issue.Message,
			Context:   identity.Context,
		}
		if exc, ok := suppressed[[2]string{issue.RuleID, issue.File}]; ok {
			finding.Suppressed = true
			finding.Justification = exc.Justification
		}
		finding.Assessment = render.AssessmentOf(issue)
		findings = append(findings, finding)
	}
	// The gate's blocking findings (the audit's blocking_findings) give each
	// counted finding its origin and add those no issue carries.
	findings = render.GateSARIFFindings(findings, blocking)
	// The model's assessment of a deterministic finding travels beside the
	// engine's origin.
	for i := range findings {
		if findings[i].Assessment != nil && findings[i].Origin == "" && i < len(in.issues) {
			findings[i].Origin = in.issues[i].Origin
		}
	}
	executionSuccessful := in.gateInconclusiveReason == ""
	return render.WriteSARIF(in.sarifPath, version, findings, executionSuccessful, in.gateInconclusiveReason, filter)
}

// auditGateOutcome collapses a gateDecision (policygate.go) into the
// pass/fail/inconclusive decision AC-001's audit record names, mirroring
// publishPolicyGateStatus's own switch (policygate.go) without publishing
// anything. inconclusiveReason is checked independently of the gate's own
// Active/Inconclusive flags: a run can be genuinely inconclusive (provider
// failure, partial coverage, degraded parse) with NO gate declared at all
// (gate.Declared() == false, e.g. no `gate:` key in config), and the audit
// record must still say "inconclusive", never "pass" -- the SARIF document
// for the exact same run already marks executionSuccessful=false in that
// case, and the two files must agree.
func auditGateOutcome(g gateDecision, inconclusiveReason string) (decision, reason string) {
	reason = strings.Join(g.Lines, "; ")
	switch {
	case g.Breach, g.Fail:
		return "fail", reason
	case g.Inconclusive:
		return "inconclusive", reason
	case inconclusiveReason != "":
		return "inconclusive", "review inconclusive (" + inconclusiveReason + ")"
	default:
		return "pass", reason
	}
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
