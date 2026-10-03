// Filters that keep the published pull request review on the changed lines:
// inline eligibility, limitation and suggestion filtering, issue ordering.
package main

import (
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// addedLineNumbers returns the set of new-side line numbers hunk h adds.
// The restored client's parser (internal/git/githubclient/client.go,
// parseDiff) keeps every hunk content line's leading +/-/space marker: an
// added ('+') line is recorded at the current new-file counter and
// advances it; a context (' ') line only advances it; a removed ('-') line
// does neither, because it has no line on the new side to advance past or
// to anchor a comment to.
func addedLineNumbers(h types.DiffHunk) map[int]bool {
	added := make(map[int]bool)
	n := h.NewStart
	for _, l := range h.Lines {
		if l == "" {
			continue
		}
		switch l[0] {
		case '+':
			added[n] = true
			n++
		case ' ':
			n++
		}
	}
	return added
}

// isInlineEligible requires a changed line in the declared coordinate space.
// Additions use RIGHT and deletions LEFT; context cannot anchor a finding.
func isInlineEligible(diff *types.Diff, issue types.ReviewIssue) bool {
	return review.IsChangedLine(diff, issue)
}

// sortedIssues returns a copy of issues ordered by (file, line) -- the same
// order printFindings already uses for the --base contract -- so the
// sequence of PostReviewComment/PostIssueComment calls this card makes is
// deterministic. AC-001 requires that repeating the same input produces the
// same output; for the PR path that output includes the publish
// transcript (what got posted, in what order), not only stdout.
func sortedIssues(issues []types.ReviewIssue) []types.ReviewIssue {
	out := make([]types.ReviewIssue, len(issues))
	copy(out, issues)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// suppressOperationalStrengths prevents a model from presenting repository
// wiring as a code-quality achievement. A PR that changes only configuration,
// workflows, documentation or comments may still have real findings, but its
// published review should not praise the integration setup as product code.
func suppressOperationalStrengths(diff *types.Diff, result *types.ReviewResult) {
	if result != nil && !prompt.HasSubstantiveCodeChange(diff) {
		result.Strengths = nil
	}
}

// filterLimitationsAgainstDiff removes a model limitation that contradicts
// the review input itself. A file printed in the Code changes block was
// available to the model, so publishing that file as "unavailable" makes an
// otherwise valid review factually misleading. Limitations about evidence
// outside the diff, such as missing CI logs, remain untouched.
func filterLimitationsAgainstDiff(diff *types.Diff, limitations []string) []string {
	if len(limitations) == 0 {
		return limitations
	}
	paths := make([]string, 0, len(diff.Files))
	for _, file := range diff.Files {
		if path := strings.TrimSpace(file.Path); path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return limitations
	}

	filtered := make([]string, 0, len(limitations))
	for _, limitation := range limitations {
		lower := strings.ToLower(limitation)
		claimsUnavailable := strings.Contains(lower, "not available") ||
			strings.Contains(lower, "unavailable") ||
			strings.Contains(lower, "não disponível") ||
			strings.Contains(lower, "nao disponivel") ||
			strings.Contains(lower, "não estavam disponíveis") ||
			strings.Contains(lower, "nao estavam disponiveis")
		if claimsUnavailable && limitationMentionsChangedPath(lower, paths) {
			continue
		}
		filtered = append(filtered, limitation)
	}
	return filtered
}

func limitationMentionsChangedPath(lowerLimitation string, paths []string) bool {
	for _, path := range paths {
		if strings.Contains(lowerLimitation, strings.ToLower(path)) {
			return true
		}
	}
	return false
}

// filterSuggestionsToChangedLines keeps the published review actionable. A
// non-blocking suggestion may be general, but once it claims a file and line
// it must point at actually added lines in this pull request. A code proposal
// may cover a range, but every line in that range must be added; this gives a
// future apply operation an exact, reviewable replacement boundary. Models
// often emit line zero or cite nearby context files; publishing those
// locations makes the review look authoritative while giving the author
// nowhere useful to act. Suggestions without a location remain valid general
// advice.
func filterSuggestionsToChangedLines(diff *types.Diff, suggestions []types.ReviewSuggestion) []types.ReviewSuggestion {
	filtered := make([]types.ReviewSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		start, end := suggestionRange(suggestion)
		if strings.TrimSpace(suggestion.File) == "" && start <= 0 && end <= 0 {
			filtered = append(filtered, suggestion)
			continue
		}
		if strings.TrimSpace(suggestion.File) == "" || start <= 0 || end < start || end-start > 1000 {
			continue
		}
		valid := true
		for line := start; ; line++ {
			if !isInlineEligible(diff, types.ReviewIssue{File: suggestion.File, Line: line}) {
				valid = false
				break
			}
			if line == end {
				break
			}
		}
		if !valid {
			continue
		}
		filtered = append(filtered, suggestion)
	}
	return filtered
}

// suggestionRange keeps the old single-line field compatible while allowing
// newer model responses to describe a complete replacement range.
func suggestionRange(suggestion types.ReviewSuggestion) (start, end int) {
	start = suggestion.StartLine
	if start <= 0 {
		start = suggestion.Line
	}
	end = suggestion.EndLine
	if end <= 0 {
		end = start
	}
	return start, end
}
