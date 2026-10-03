// Package facts holds the gate's structured facts. The gate produces them
// while it decides; the audit record and the SARIF document only present
// them. They live in their own package, imported by both internal/gate and
// internal/render and importing neither, so the domain never depends on
// its presentation.
package facts

// AnalysisDataAudit is the audit fact of a declared analysis_data section
// that resolved a usable artifact.
type AnalysisDataAudit struct {
	Digest      string `json:"digest"`
	GeneratedAt string `json:"generated_at"`
	Tag         string `json:"tag"`
	// Source is "remote" (release listing answered) or "cache" (listing
	// failed; a locally cached copy, re-verified, was used).
	Source string `json:"source,omitempty"`
}

// AuditFinding is one finding that actually contributed to the gate's
// decision -- not every finding the review raised, only the ones the gate
// matched against its threshold.
type AuditFinding struct {
	RuleID   string `json:"rule_id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	// Origin is where the finding came from: skills, analysis, security,
	// or a scanner engine's typed origin (sast for semgrep).
	Origin string `json:"origin,omitempty"`
}

// AuditException documents one exception the gate applied to a finding.
type AuditException struct {
	RuleID        string `json:"rule_id"`
	Path          string `json:"path"`
	Justification string `json:"justification"`
}
