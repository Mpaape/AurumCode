package render

import (
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// AuditAssessment is the model's conclusion about one piece of
// deterministic evidence. It sits beside the evidence's Origin (what the
// engine measured), never in place of it.
type AuditAssessment struct {
	EvidenceID    string   `json:"evidence_id"`
	Status        string   `json:"status"`
	Justification string   `json:"justification"`
	Priority      string   `json:"priority,omitempty"`
	Correlates    []string `json:"correlates_with,omitempty"`
}

// AuditEvidence is one deterministic finding the model assessed: where the
// engine found it and under which origin, and what the model concluded.
type AuditEvidence struct {
	RuleID     string          `json:"rule_id"`
	Path       string          `json:"path"`
	Line       int             `json:"line"`
	Severity   string          `json:"severity"`
	Origin     string          `json:"origin"`
	Assessment AuditAssessment `json:"assessment"`
}

// AssessedEvidence lists the issues that carry both an engine origin and a
// model assessment, in input order.
func AssessedEvidence(issues []types.ReviewIssue) []AuditEvidence {
	var out []AuditEvidence
	for _, issue := range issues {
		if issue.Origin == "" || issue.Assessment == nil {
			continue
		}
		out = append(out, AuditEvidence{
			RuleID: issue.RuleID, Path: issue.File, Line: issue.Line,
			Severity: issue.Severity, Origin: issue.Origin,
			Assessment: auditAssessmentOf(issue.Assessment),
		})
	}
	return out
}

func auditAssessmentOf(a *types.EvidenceAssessment) AuditAssessment {
	return AuditAssessment{
		EvidenceID: a.EvidenceID, Status: a.Status, Justification: a.Justification,
		Priority: a.Priority, Correlates: append([]string(nil), a.Correlates...),
	}
}

// redactAssessment returns a redacted copy of a.
func redactAssessment(filter *redaction.Filter, a AuditAssessment) AuditAssessment {
	out := AuditAssessment{
		EvidenceID: filter.Redact(a.EvidenceID), Status: filter.Redact(a.Status),
		Justification: filter.Redact(a.Justification), Priority: filter.Redact(a.Priority),
	}
	for _, id := range a.Correlates {
		out.Correlates = append(out.Correlates, filter.Redact(id))
	}
	return out
}

// redactEvidence returns a redacted copy of evidence.
func redactAuditEvidence(filter *redaction.Filter, evidence []AuditEvidence) []AuditEvidence {
	if evidence == nil {
		return nil
	}
	out := make([]AuditEvidence, len(evidence))
	for i, e := range evidence {
		out[i] = AuditEvidence{
			RuleID: filter.Redact(e.RuleID), Path: filter.Redact(e.Path), Line: e.Line,
			Severity: filter.Redact(e.Severity), Origin: filter.Redact(e.Origin),
			Assessment: redactAssessment(filter, e.Assessment),
		}
	}
	return out
}

// sarifAssessmentOf is the SARIF property form of an assessment; nil when
// the finding carries none.
func sarifAssessmentOf(a *AuditAssessment) *AuditAssessment {
	if a == nil {
		return nil
	}
	copied := *a
	return &copied
}

// AssessmentOf is the SARIF/audit form of issue's assessment, nil when the
// issue carries none.
func AssessmentOf(issue types.ReviewIssue) *AuditAssessment {
	if issue.Assessment == nil {
		return nil
	}
	a := auditAssessmentOf(issue.Assessment)
	return &a
}
