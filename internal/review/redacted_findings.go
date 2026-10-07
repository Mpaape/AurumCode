package review

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// RedactionMarkerDiscardKey is the result metadata key counting the model
// findings discarded for citing the redaction marker. They are also part of
// issues_rejected_by_scope and named in scope_discard_warning.
const RedactionMarkerDiscardKey = "issues_rejected_by_redaction_marker"

// redactedMarkerDiscardReason names, in the operator-visible discard
// warning, the findings removed because they cite the redaction marker.
const redactedMarkerDiscardReason = "citando o marcador de redacao (o modelo nao viu o valor mascarado)"

// citesRedactionMarker reports whether a model finding builds its claim on
// redacted text. Every untrusted input is redacted before it reaches the
// model, so wherever a secret-shaped value stood the model saw only
// redaction.Marker. A finding that quotes the marker in its message,
// evidence, impact or suggested fix is therefore an allegation about a value
// the model never saw -- typically "this [REDACTED] is a hardcoded secret"
// about something that was an ordinary identifier. Secret detection belongs
// to the mandatory secret scanner, which reads the raw content and fails
// closed; the model cannot ground a finding on the mask.
func citesRedactionMarker(issue types.ReviewIssue) bool {
	for _, field := range []string{issue.Message, issue.Evidence, issue.Impact, issue.Suggestion} {
		if strings.Contains(field, redaction.Marker) {
			return true
		}
	}
	return false
}

// isModelFinding reports whether a finding was written by the model. The
// engine stamps Origin on every deterministic finding (secret scanner, SAST,
// vet, security pass) and the parser clears any Origin a model reply
// supplied, so only an empty Origin is the model's.
func isModelFinding(issue types.ReviewIssue) bool {
	return issue.Origin == ""
}

// discardRedactedModelFindings removes the model findings that cite the
// redaction marker and returns how many it removed. A deterministic
// finding is never touched here, even when its text carries the marker:
// a scanner saw the real value, so its finding keeps its authority.
func discardRedactedModelFindings(issues []types.ReviewIssue) ([]types.ReviewIssue, int) {
	kept := make([]types.ReviewIssue, 0, len(issues))
	discarded := 0
	for _, issue := range issues {
		if isModelFinding(issue) && citesRedactionMarker(issue) {
			discarded++
			continue
		}
		kept = append(kept, issue)
	}
	return kept, discarded
}
