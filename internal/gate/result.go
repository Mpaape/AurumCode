// Package gate is the single pipeline every review path (--base and --pr)
// runs to reach a gate decision. Each source of gate evidence (exceptions,
// verdict reuse, policy skills, the registered scanners, the embedded
// analysis catalog, the deterministic security pass, analysis data,
// Dependency-Track, dependencies) is a Contributor; the Pipeline applies
// them in declared order against one Result and decides, in one place,
// what a failing contributor means.
package gate

import (
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Result is one run's gate outcome. Active is false when nothing declared
// a gate: every other field is then meaningless and the caller changes
// nothing about today's behavior. Fail means the check must report failure
// (a severity breach, or an inconclusive run under gate.inconclusive:
// block). Breach is true only when a real severity-threshold breach was
// found, as opposed to Fail set purely by the inconclusive mode. Lines
// names why, for the published summary and stderr.
type Result struct {
	Active       bool
	Fail         bool
	Inconclusive bool
	Breach       bool
	Lines        []string

	// BlockingFindings and AppliedExceptions are the same decisions as
	// Breach and the exception match, as structured data for the audit
	// record and the SARIF document.
	BlockingFindings  []facts.AuditFinding
	AppliedExceptions []facts.AuditException

	// AnalysisData is the audit fact of a declared analysis_data section
	// that resolved a usable artifact.
	AnalysisData *facts.AnalysisDataAudit

	// Reason is the machine-readable motive of an inconclusive run
	// ("provider_failure", "partial_coverage", ...), comma-joined when
	// several contributors report one. Empty means conclusive.
	Reason string

	// Trail records, per applied contributor, what it added. It is never
	// published; it exists so each finding's origin is auditable.
	Trail []Contribution

	// FirstBreach is the first severity breach found, for the one-line
	// summary of the policy-gate status. It is never part of the audit
	// record or the SARIF document.
	FirstBreach *Breach

	// auditText maps a line shown in the review's language to the text the
	// audit record keeps for it: the audit is machine data and does not
	// change language. Lines shown as written are not in it.
	auditText map[string]string
}

// addLine appends a line shown as shown and audited as audit.
func (r *Result) addLine(shown, audit string) {
	r.Lines = append(r.Lines, shown)
	if shown == audit {
		return
	}
	if r.auditText == nil {
		r.auditText = map[string]string{}
	}
	r.auditText[shown] = audit
}

// AuditLines is Lines as the audit record writes them, whatever the review
// language.
func (r Result) AuditLines() []string {
	out := make([]string, len(r.Lines))
	for i, line := range r.Lines {
		if audit, ok := r.auditText[line]; ok {
			line = audit
		}
		out[i] = line
	}
	return out
}

// Breach is one finding that failed the gate, as the status names it.
type Breach struct {
	Title    string
	Severity string
	Path     string
	Line     int
}

// noteBreach records issue as FirstBreach unless one is already recorded.
func (r *Result) noteBreach(title string, issue types.ReviewIssue) {
	if r.FirstBreach != nil {
		return
	}
	r.FirstBreach = &Breach{Title: title, Severity: issue.Severity, Path: issue.File, Line: issue.Line}
}

// Contribution is one contributor's footprint on a Result.
type Contribution struct {
	Name     string
	Origin   string
	Lines    int
	Findings int
	Err      string
}

// AddReason appends reason to Reason, comma-joined, never replacing one
// already set.
func (r *Result) AddReason(reason string) {
	if reason == "" {
		return
	}
	if r.Reason != "" {
		r.Reason += "," + reason
		return
	}
	r.Reason = reason
}

// Merge folds other into r the way a secondary gate source joins the
// primary decision: flags are OR-ed, lines and structured findings are
// appended, and other's Reason is joined to r's. An analysis-data audit
// fact other carries is kept even when other is inactive. Otherwise a zero
// (inactive) other leaves r untouched.
func (r *Result) Merge(other Result) {
	if other.AnalysisData != nil {
		r.AnalysisData = other.AnalysisData
	}
	if !other.Active {
		return
	}
	r.Active = true
	r.Fail = r.Fail || other.Fail
	r.Breach = r.Breach || other.Breach
	r.Inconclusive = r.Inconclusive || other.Inconclusive
	r.Lines = append(r.Lines, other.Lines...)
	r.BlockingFindings = append(r.BlockingFindings, other.BlockingFindings...)
	r.AppliedExceptions = append(r.AppliedExceptions, other.AppliedExceptions...)
	r.AddReason(other.Reason)
	for shown, audit := range other.auditText {
		if r.auditText == nil {
			r.auditText = map[string]string{}
		}
		r.auditText[shown] = audit
	}
	if r.FirstBreach == nil {
		r.FirstBreach = other.FirstBreach
	}
}
