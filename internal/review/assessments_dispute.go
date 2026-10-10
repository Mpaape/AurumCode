package review

import (
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// DisputeCounts reports whether the model's assessment is a dispute the gate
// may act on: status disputed with a justification that is not blank. The
// model decides with reasoning; a dispute without it weighs as
// needs_context, so the finding keeps counting.
func DisputeCounts(a *types.EvidenceAssessment) bool {
	if a == nil || a.Status != types.AssessmentDisputed {
		return false
	}
	return strings.TrimSpace(a.Justification) != ""
}
