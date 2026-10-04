// Inline publication: a finding or a native GitHub suggestion rendered as a
// line comment of the pull request review.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func formatInlineIssue(issue types.ReviewIssue) string {
	return formatInlineIssueForLanguage(issue, "en-US")
}

func formatInlineIssueForLanguage(issue types.ReviewIssue, language string) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	fmt.Fprintf(&b, "**[%s] %s**", issue.Severity, issue.Message)
	if issue.Side == "LEFT" {
		fmt.Fprintf(&b, "\n\n%s: `%s:%d`", i18n.Text(language, "inline.removed_line"), issue.File, issue.Line)
	}
	writeReviewField(&b, copy.impact, issue.Impact)
	writeReviewField(&b, copy.evidence, issue.Evidence)
	writeReviewField(&b, copy.suggestedFix, issue.Suggestion)
	writeReviewField(&b, copy.verify, issue.Verification)
	return b.String()
}

func writeReviewField(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "\n\n**%s:** %s", label, strings.TrimSpace(value))
}

// nativeSuggestionComment converts an implementation-ready model suggestion
// into GitHub's inline suggestion format. The range has already passed the
// changed-line filter, but this helper repeats the location check at the
// publication boundary so a malformed response can never create an
// actionable suggestion against an unchanged line.
func nativeSuggestionComment(diff *types.Diff, suggestion types.ReviewSuggestion, language string) (githubclient.ReviewLineComment, bool) {
	if strings.TrimSpace(suggestion.ProposedCode) == "" || !isNativeSuggestion(diff, suggestion) {
		return githubclient.ReviewLineComment{}, false
	}
	start, end := suggestionRange(suggestion)
	comment := githubclient.ReviewLineComment{
		Body: formatNativeSuggestionBody(suggestion, language),
		Path: suggestion.File,
		Line: end,
		Side: "RIGHT",
	}
	if start != end {
		comment.StartLine = start
		comment.StartSide = "RIGHT"
	}
	return comment, true
}

func isNativeSuggestion(diff *types.Diff, suggestion types.ReviewSuggestion) bool {
	if strings.TrimSpace(suggestion.ProposedCode) == "" || strings.TrimSpace(suggestion.File) == "" {
		return false
	}
	start, end := suggestionRange(suggestion)
	if start <= 0 || end < start || end-start > 1000 {
		return false
	}
	for line := start; ; line++ {
		if !isInlineEligible(diff, types.ReviewIssue{File: suggestion.File, Line: line}) {
			return false
		}
		if line == end {
			return true
		}
	}
}

func formatNativeSuggestionBody(suggestion types.ReviewSuggestion, language string) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	if title := strings.TrimSpace(suggestion.Title); title != "" {
		fmt.Fprintf(&b, "**%s**", title)
	}
	if description := strings.TrimSpace(suggestion.Description); description != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(description)
	}
	writeReviewField(&b, copy.rationale, suggestion.Rationale)
	writeReviewField(&b, copy.verify, suggestion.Verification)
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("```suggestion\n")
	b.WriteString(strings.Trim(suggestion.ProposedCode, "\n"))
	b.WriteString("\n```")
	return b.String()
}
