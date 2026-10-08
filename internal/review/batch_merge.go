package review

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Consolidation of a review in batches into one result: one parecer, one set
// of findings for one gate.

// summedMetaKeys are the engine counts that add up across batches.
var summedMetaKeys = map[string]bool{
	"issues_rejected_without_rule": true,
	"issues_rejected_by_scope":     true,
	RedactionMarkerDiscardKey:      true,
	"summary_discarded_findings":   true,
	"total_files":                  true,
	"lines_added":                  true,
	"lines_deleted":                true,
	"segments_used":                true,
	"estimated_tokens":             true,
	"code_files_total":             true,
	"code_files_complete":          true,
	"code_files_partial":           true,
	"code_files_omitted":           true,
}

// joinedMetaKeys are the warnings that keep every batch's text.
var joinedMetaKeys = map[string]bool{
	"discard_warning":             true,
	"scope_discard_warning":       true,
	AssessmentDiscardWarningKey:   true,
	"optional_sections_discarded": true,
}

// verdictRank orders the model's verdicts from the least to the most severe;
// the consolidated verdict is the most severe a batch gave.
var verdictRank = map[string]int{"approve": 1, "comment": 2, "request_changes": 3}

// metaTrue is the value of a metadata flag that is set.
const metaTrue = "true"

// mergeResults consolidates the batches' results. Lists are concatenated
// in batch order, counts are summed, a set flag (quality degraded and the
// like) stays set, and the summaries are kept side by side.
func mergeResults(results []*types.ReviewResult) *types.ReviewResult {
	merged := &types.ReviewResult{Metadata: map[string]string{}}
	var summaries []string
	for _, res := range results {
		if res == nil {
			continue
		}
		if verdictRank[res.Verdict] > verdictRank[merged.Verdict] {
			merged.Verdict = res.Verdict
		}
		merged.Strengths = appendUnlike(merged.Strengths, res.Strengths...)
		merged.Issues = append(merged.Issues, res.Issues...)
		merged.Suggestions = append(merged.Suggestions, res.Suggestions...)
		merged.CIAnalysis = append(merged.CIAnalysis, res.CIAnalysis...)
		merged.TestPlan = appendUnlike(merged.TestPlan, res.TestPlan...)
		merged.Limitations = appendUnlike(merged.Limitations, res.Limitations...)
		merged.LineComments = append(merged.LineComments, res.LineComments...)
		merged.FileComments = append(merged.FileComments, res.FileComments...)
		merged.EvidenceAssessments = append(merged.EvidenceAssessments, res.EvidenceAssessments...)
		if merged.ISOScores == nil {
			merged.ISOScores = res.ISOScores
		}
		if merged.CommitComment == "" {
			merged.CommitComment = res.CommitComment
		}
		if merged.OverallScore == 0 || (res.OverallScore != 0 && res.OverallScore < merged.OverallScore) {
			merged.OverallScore = res.OverallScore
		}
		if s := strings.TrimSpace(res.Summary); s != "" {
			summaries = appendUnlike(summaries, s)
		}
		mergeMetadata(merged.Metadata, res.Metadata)
	}
	merged.Summary = strings.Join(summaries, "\n\n")
	return merged
}

// mergeMetadata folds one batch's metadata into the consolidated map.
func mergeMetadata(into, from map[string]string) {
	for key, value := range from {
		current, seen := into[key]
		switch {
		case key == OutsideDiffFindingsKey:
			mergeOutsideDiff(into, value)
		case summedMetaKeys[key]:
			into[key] = strconv.Itoa(metaCount(into, key) + metaCount(from, key))
		case joinedMetaKeys[key]:
			if value != "" {
				into[key] = strings.TrimPrefix(current+"; "+value, "; ")
			}
		case !seen || current == "" || value == metaTrue:
			into[key] = value
		}
	}
}

// declareOutside counts and names the files a ceiling left out: omitted
// code files, so the coverage is partial and the approval withheld.
func declareOutside(result *types.ReviewResult, outside []string) {
	if len(outside) == 0 {
		return
	}
	for _, key := range []string{"code_files_total", "code_files_omitted"} {
		result.Metadata[key] = strconv.Itoa(metaCount(result.Metadata, key) + len(outside))
	}
	result.Metadata[MetaOmittedPaths] = strings.Join(outside, "\n")
}

// mergeTranscripts joins the deliberations of the batches; nil when no
// batch deliberated.
func mergeTranscripts(transcripts []*deliberation.Transcript) *deliberation.Transcript {
	var merged *deliberation.Transcript
	for _, t := range transcripts {
		if t == nil {
			continue
		}
		if merged == nil {
			merged = &deliberation.Transcript{}
		}
		merged.Offered = appendUnique(merged.Offered, t.Offered...)
		merged.Requested = append(merged.Requested, t.Requested...)
		merged.NotRequested = appendUnique(merged.NotRequested, t.NotRequested...)
		merged.Rounds += t.Rounds
		merged.TokensIn += t.TokensIn
		merged.TokensOut += t.TokensOut
		merged.Calls = append(merged.Calls, t.Calls...)
		merged.Outcome = t.Outcome
		if merged.Limit == "" {
			merged.Limit = t.Limit
		}
		if merged.Undecided == "" {
			merged.Undecided = t.Undecided
		}
	}
	return merged
}

// appendUnique appends the values not already in list.
func appendUnique(list []string, values ...string) []string {
	seen := make(map[string]bool, len(list))
	for _, v := range list {
		seen[v] = true
	}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			list = append(list, v)
		}
	}
	return list
}

// appendUnlike appends the values whose opening words are not already in
// list. Each batch writes its own prose, and two batches often say the
// same thing in slightly different words ("the CI is green; no logs" twice):
// the parecer keeps one.
func appendUnlike(list []string, values ...string) []string {
	seen := make(map[string]bool, len(list))
	for _, v := range list {
		seen[proseKey(v)] = true
	}
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if key := proseKey(v); !seen[key] {
			seen[key] = true
			list = append(list, v)
		}
	}
	return list
}

// proseKey is the first five words of a text, lowercased and stripped of
// punctuation.
func proseKey(text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) > 5 {
		words = words[:5]
	}
	return strings.Join(words, " ")
}
