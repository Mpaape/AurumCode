package review

import (
	"context"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"strings"
	"testing"

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
	weighAssessments(result, []string{"E1", "E2"})
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

// Evidence above the section's ceiling is declared "N omitidos" and never
// shown: the model did not read it, so an assessment of an omitted id is
// discarded with the warning, exactly like an id never offered, and only
// the admitted ids keep their assessment (nothing omitted can be demoted).
func TestAssessmentOfEvidenceOmittedByTheCeilingIsDiscarded(t *testing.T) {
	evidence := sampleEvidence(40)
	reply := `{"verdict":"comment","issues":[],"summary":"ok","evidence_assessments":[` +
		`{"evidence_id":"ev-0","status":"confirmed","justification":"lido"},` +
		`{"evidence_id":"ev-39","status":"disputed","justification":"nunca lido"}]}`
	provider := &messagesCaptureProvider{FakeProvider: FakeProvider{Response: reply}}
	reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil), DefaultConfig())
	limits := prompt.DefaultLimits()
	limits.EvidenceMaxTokens = 400
	if err := reviewer.promptBuilder.SetSlotLimits(limits); err != nil {
		t.Fatal(err)
	}
	result, err := reviewer.GenerateReviewWithContext(context.Background(), goldenDiff(), ReviewContext{Evidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	user := provider.messages[1].Content
	if strings.Contains(user, "[ev-39]") || !strings.Contains(user, "[ev-0]") || !strings.Contains(user, "omitidos") {
		t.Fatalf("fixture invalid: ev-39 must be omitted and ev-0 shown")
	}
	if len(result.EvidenceAssessments) != 1 || result.EvidenceAssessments[0].EvidenceID != "ev-0" {
		t.Fatalf("kept %+v, want only the shown ev-0", result.EvidenceAssessments)
	}
	if w := result.Metadata[AssessmentDiscardWarningKey]; !strings.Contains(w, `"ev-39"`) || !strings.Contains(w, "omitted by the section ceiling") {
		t.Fatalf("the omitted id must be discarded loudly, warning = %q", w)
	}
}
