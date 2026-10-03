package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// AssessmentDiscardWarningKey is the result metadata key naming the
// evidence assessments the engine refused: a status outside the closed set,
// no evidence id, or an id the engine never offered. A refusal is never
// silent.
const AssessmentDiscardWarningKey = "assessment_discard_warning"

// The priorities an assessment may carry; anything else is cleared.
var knownAssessmentPriorities = map[string]bool{"high": true, "medium": true, "low": true}

// weighAssessments keeps only the model's assessments of evidence the
// engine actually offered in this prompt. The model answers about evidence;
// it never creates it: an assessment naming an id that was not offered is
// dropped (and named in the warning), so a reply cannot attach a verdict to
// a finding it invented. Issue-level assessments of offered evidence are
// lifted into the result's list when the list has none for that id.
func weighAssessments(result *types.ReviewResult, offered []prompt.EvidenceItem) {
	ids := make(map[string]bool, len(offered))
	for _, e := range offered {
		ids[e.ID] = true
	}
	var refused []string
	kept := make([]types.EvidenceAssessment, 0, len(result.EvidenceAssessments))
	seen := map[string]bool{}
	keep := func(a types.EvidenceAssessment) {
		a = normalizeAssessment(a, ids)
		switch {
		case !types.IsKnownAssessmentStatus(a.Status) || !ids[a.EvidenceID]:
			refused = append(refused, refusedLabel(a.EvidenceID))
		case !seen[a.EvidenceID]:
			seen[a.EvidenceID] = true
			kept = append(kept, a)
		}
	}
	for _, a := range result.EvidenceAssessments {
		keep(a)
	}
	for i := range result.Issues {
		a := result.Issues[i].Assessment
		if a == nil {
			continue
		}
		if !ids[strings.TrimSpace(a.EvidenceID)] {
			refused = append(refused, refusedLabel(a.EvidenceID))
			result.Issues[i].Assessment = nil
			continue
		}
		keep(*a)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].EvidenceID < kept[j].EvidenceID })
	result.EvidenceAssessments = kept
	if len(result.EvidenceAssessments) == 0 {
		result.EvidenceAssessments = nil
	}
	if result.Metadata == nil {
		result.Metadata = map[string]string{}
	}
	result.Metadata[AssessmentDiscardWarningKey] = ""
	if len(refused) > 0 {
		result.Metadata[AssessmentDiscardWarningKey] = fmt.Sprintf(
			"discarded %d evidence assessment(s) that named no offered evidence or carried an unknown status: %s",
			len(refused), strings.Join(refused, ", "))
	}
}

// normalizeAssessment trims and lower-cases the closed fields, clears an
// unknown priority and keeps only correlations to other offered evidence.
func normalizeAssessment(a types.EvidenceAssessment, offered map[string]bool) types.EvidenceAssessment {
	a.EvidenceID = strings.TrimSpace(a.EvidenceID)
	a.Status = strings.ToLower(strings.TrimSpace(a.Status))
	a.Priority = strings.ToLower(strings.TrimSpace(a.Priority))
	if !knownAssessmentPriorities[a.Priority] {
		a.Priority = ""
	}
	var correlates []string
	for _, id := range a.Correlates {
		id = strings.TrimSpace(id)
		if id != a.EvidenceID && offered[id] {
			correlates = append(correlates, id)
		}
	}
	a.Correlates = correlates
	return a
}

// refusedLabel names a refused assessment in the warning.
func refusedLabel(id string) string {
	if id = strings.TrimSpace(id); id == "" {
		return "(no evidence id)"
	}
	return fmt.Sprintf("%q", id)
}
