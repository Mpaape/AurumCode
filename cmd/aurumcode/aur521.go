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
	// AUR-521's own workflow-SHA convention: AURUMCODE_WORKFLOW_SHA (the
	// reusable workflow's own version, forwarded under a name GitHub
	// Actions does not reserve). No fallback to GITHUB_SHA here on
	// purpose: GITHUB_SHA already has its own field (ReviewedSHA, right
	// below) -- a run with no AURUMCODE_WORKFLOW_SHA (a local invocation,
	// outside the reusable workflow) must record an EMPTY workflow_sha,
	// never silently duplicate reviewed_sha's value into it. A reviewer
	// reading the record must be able to tell "this workflow's own
	// version is unknown" apart from "this workflow's version happens to
	// equal the reviewed commit".
	workflowSHA := strings.TrimSpace(os.Getenv("AURUMCODE_WORKFLOW_SHA"))

	decision, reason := auditGateOutcome(in.gate, in.gateInconclusiveReason)
	// AUR-521/AUR-520: BlockingFindings and AppliedExceptions are
	// evaluateGate's OWN structured output (policygate.go) -- the single
	// place a finding is ever decided to block the gate or be excepted.
	// This function never re-derives that decision a second time.
	blocking := in.gate.BlockingFindings
	if blocking == nil {
		blocking = []render.AuditFinding{}
	}
	exceptionsApplied := in.gate.AppliedExceptions
	if exceptionsApplied == nil {
		exceptionsApplied = []render.AuditException{}
	}
	// suppressed indexes AppliedExceptions by (ruleID, path) so the SARIF
	// loop below can mark the exact same findings evaluateGate excepted
	// as suppressed, with the exact same justification -- never a second,
	// possibly-disagreeing exception match.
	suppressed := make(map[[2]string]render.AuditException, len(exceptionsApplied))
	for _, exc := range exceptionsApplied {
		suppressed[[2]string{exc.RuleID, exc.Path}] = exc
	}

	if in.auditoriaPath != "" {
		rec := render.BuildAuditRecord(
			policyDigest, workflowSHA, in.repo, in.reviewedSHA, in.model, in.verdict,
			render.AuditGate{Decision: decision, Reason: reason},
			blocking,
			exceptionsApplied,
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
			findings = append(findings, finding)
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
