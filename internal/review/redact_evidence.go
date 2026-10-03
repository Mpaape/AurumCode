package review

import (
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// redactEvidence returns a copy of evidence with every field passed
// through the AUR-009 filter; the snippet keeps its diff markers.
func redactEvidence(f *redaction.Filter, evidence []prompt.EvidenceItem) []prompt.EvidenceItem {
	if len(evidence) == 0 {
		return nil
	}
	out := make([]prompt.EvidenceItem, len(evidence))
	for i, e := range evidence {
		out[i] = prompt.EvidenceItem{
			ID:       f.Redact(e.ID),
			Origin:   f.Redact(e.Origin),
			RuleID:   f.Redact(e.RuleID),
			File:     f.Redact(e.File),
			Line:     e.Line,
			Side:     f.Redact(e.Side),
			Severity: f.Redact(e.Severity),
			Snippet:  redactLinesKeepingMarkers(f, e.Snippet),
		}
	}
	return out
}

// redactTools returns a copy of tools with every field redacted.
func redactTools(f *redaction.Filter, tools []prompt.ToolOffer) []prompt.ToolOffer {
	if len(tools) == 0 {
		return nil
	}
	out := make([]prompt.ToolOffer, len(tools))
	for i, t := range tools {
		out[i] = prompt.ToolOffer{Name: f.Redact(t.Name), Description: f.Redact(t.Description), Cost: f.Redact(t.Cost)}
	}
	return out
}
