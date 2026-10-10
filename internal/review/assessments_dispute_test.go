package review

import (
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// AUR-608 AC-003: only a justified dispute may demote; a blank justification
// weighs as needs_context and every other status never demotes.
func TestAUR608DisputeCountsRequiresJustification(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    *types.EvidenceAssessment
		want bool
	}{
		{"nil", nil, false},
		{"justified dispute", &types.EvidenceAssessment{Status: types.AssessmentDisputed, Justification: "valor de exemplo"}, true},
		{"empty justification", &types.EvidenceAssessment{Status: types.AssessmentDisputed}, false},
		{"blank justification", &types.EvidenceAssessment{Status: types.AssessmentDisputed, Justification: " \n\t "}, false},
		{"confirmed", &types.EvidenceAssessment{Status: types.AssessmentConfirmed, Justification: "real"}, false},
		{"needs context", &types.EvidenceAssessment{Status: types.AssessmentNeedsContext, Justification: "depende"}, false},
	} {
		if got := DisputeCounts(tc.a); got != tc.want {
			t.Errorf("%s: DisputeCounts = %v, want %v", tc.name, got, tc.want)
		}
	}
}
