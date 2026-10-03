// Package gate is the single pipeline every review path (--base and --pr)
// runs to reach a gate decision. Each source of gate evidence (policy
// skills, the embedded analysis catalog, SAST, Dependency-Track, analysis
// data, exceptions, verdict reuse) is a Contributor; the Pipeline applies
// them in declared order against one Result and decides, in one place,
// what a failing contributor means.
package gate

import (
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
	BlockingFindings  []types.AuditFinding
	AppliedExceptions []types.AuditException

	// AnalysisData is the audit fact of a declared analysis_data section
	// that resolved a usable artifact.
	AnalysisData *types.AnalysisDataAudit

	// Reason is the machine-readable motive of an inconclusive run
	// ("provider_failure", "partial_coverage", ...), comma-joined when
	// several contributors report one. Empty means conclusive.
	Reason string

	// Trail records, per applied contributor, what it added. It is never
	// published; it exists so each finding's origin is auditable.
	Trail []Contribution
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
}
