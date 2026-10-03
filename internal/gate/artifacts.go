package gate

import (
	"fmt"
)

// Reasons an audit record or SARIF document the run was asked to write could
// not be written (AUR-568). They are stable, machine-readable tokens joined
// to Result.Reason like every other inconclusive motive.
const (
	ReasonAuditWriteFailed = "audit_write_failed"
	ReasonSARIFWriteFailed = "sarif_write_failed"
)

// ArtifactFailure is one compliance artifact (--auditoria, --sarif) that was
// requested and could not be written.
type ArtifactFailure struct {
	Reason string
	Path   string
	Err    error
}

// Line names the failed artifact for the published summary and stderr.
func (f ArtifactFailure) Line() string {
	return fmt.Sprintf("artefato de conformidade nao gravado em %s (%s): %v", f.Path, f.Reason, f.Err)
}

// ApplyArtifactFailures folds the failed compliance artifacts into the gate
// decision before anything is published or any exit code is chosen. The
// motive is always recorded in Reason. When a gate is declared the review
// becomes inconclusive by the policy's mode, exactly like a failing
// contributor: block fails the check, warn marks it inconclusive, and
// approval is withheld either way. Without a declared gate only the reason
// is recorded (the caller still must not exit 0). A no-op for no failures.
func ApplyArtifactFailures(run *Run, res *Result, failures []ArtifactFailure) {
	if len(failures) == 0 {
		return
	}
	lines := make([]string, 0, len(failures))
	for _, f := range failures {
		res.AddReason(f.Reason)
		lines = append(lines, f.Line())
	}
	if run == nil || run.Cfg == nil || !run.Cfg.Gate.Declared() {
		return
	}
	res.Active = true
	res.Inconclusive = true
	if mode, err := run.Cfg.Gate.InconclusiveMode(); err == nil && mode == "block" {
		res.Fail = true
	}
	res.Lines = append(res.Lines, lines...)
	publishGateLines(run, lines)
	withholdApproval(run.Review)
}
