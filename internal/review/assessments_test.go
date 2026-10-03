package review

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The model answers about evidence; it never creates it: an assessment of an
// id that was not offered, or with a status outside the closed set, is
// dropped and named in the warning; correlations keep only offered ids.
func TestWeighAssessmentsKeepsOnlyOfferedEvidence(t *testing.T) {
	result := &types.ReviewResult{
		EvidenceAssessments: []types.EvidenceAssessment{
			{EvidenceID: "E1", Status: " Disputed ", Justification: "x", Correlates: []string{"E2", "E7", "E1"}, Priority: "HIGH"},
			{EvidenceID: "E1", Status: "confirmed"},
			{EvidenceID: "E2", Status: "maybe"},
			{EvidenceID: "E7", Status: "confirmed"},
		},
		Issues: []types.ReviewIssue{{RuleID: "r", Assessment: &types.EvidenceAssessment{EvidenceID: "E8", Status: "confirmed"}}},
	}
	weighAssessments(result, []prompt.EvidenceItem{{ID: "E1"}, {ID: "E2"}})
	if len(result.EvidenceAssessments) != 1 {
		t.Fatalf("kept %+v, want only E1", result.EvidenceAssessments)
	}
	a := result.EvidenceAssessments[0]
	if a.Status != types.AssessmentDisputed || a.Priority != "high" || strings.Join(a.Correlates, ",") != "E2" {
		t.Fatalf("E1 not normalized: %+v", a)
	}
	if result.Issues[0].Assessment != nil {
		t.Fatal("an issue-level assessment of evidence never offered survived")
	}
	warning := result.Metadata[AssessmentDiscardWarningKey]
	for _, want := range []string{"discarded 3", `"E2"`, `"E7"`, `"E8"`} {
		if !strings.Contains(warning, want) {
			t.Errorf("warning %q lacks %q", warning, want)
		}
	}
}

// Nothing offered, nothing kept: a reply cannot attach a verdict to
// evidence the engine never showed it.
func TestWeighAssessmentsWithoutEvidenceKeepsNothing(t *testing.T) {
	result := &types.ReviewResult{EvidenceAssessments: []types.EvidenceAssessment{{EvidenceID: "E1", Status: "confirmed"}}}
	weighAssessments(result, nil)
	if result.EvidenceAssessments != nil || result.Metadata[AssessmentDiscardWarningKey] == "" {
		t.Fatalf("got %+v / %q", result.EvidenceAssessments, result.Metadata[AssessmentDiscardWarningKey])
	}
}
