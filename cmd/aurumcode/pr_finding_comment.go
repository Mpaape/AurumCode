// The line comment of a blocking finding: its text and, when the model
// proposed a concrete fix for exactly those changed lines, GitHub's
// suggestion block that applies it with one click. Only what blocks gets a
// line comment, so a fix for anything else stays in the parecer's details.
package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// fixFor is the applicable fix of issue: the first code suggestion on the
// same file whose changed-line range holds the finding's line. A finding
// on a removed line (LEFT) has no applicable fix: GitHub applies a
// suggestion to the new file only.
func fixFor(diff *types.Diff, suggestions []types.ReviewSuggestion, issue types.ReviewIssue) (types.ReviewSuggestion, bool) {
	if strings.EqualFold(issue.Side, "LEFT") {
		return types.ReviewSuggestion{}, false
	}
	for _, s := range suggestions {
		if s.File != issue.File || !isNativeSuggestion(diff, s) {
			continue
		}
		if start, end := suggestionRange(s); start <= issue.Line && issue.Line <= end {
			return s, true
		}
	}
	return types.ReviewSuggestion{}, false
}

// attachedToBlocking reports whether suggestion is the fix of a finding in
// blocking: it is published inside that finding's line comment.
func attachedToBlocking(diff *types.Diff, suggestion types.ReviewSuggestion, blocking []types.ReviewIssue) bool {
	for _, issue := range blocking {
		if fix, ok := fixFor(diff, []types.ReviewSuggestion{suggestion}, issue); ok && fix.File == suggestion.File {
			return true
		}
	}
	return false
}

// findingComment is the line comment of a blocking finding with body: on
// its line, or, with an applicable fix, on the fix's range with the
// suggestion block after the text.
func findingComment(diff *types.Diff, suggestions []types.ReviewSuggestion, issue types.ReviewIssue, side, body string) githubclient.ReviewLineComment {
	comment := githubclient.ReviewLineComment{Body: body, Path: issue.File, Line: issue.Line, Side: side}
	fix, ok := fixFor(diff, suggestions, issue)
	if !ok {
		return comment
	}
	start, end := suggestionRange(fix)
	comment.Body = body + "\n\n```suggestion\n" + strings.Trim(fix.ProposedCode, "\n") + "\n```"
	comment.Line = end
	comment.Side = "RIGHT"
	if start != end {
		comment.StartLine = start
		comment.StartSide = "RIGHT"
	}
	return comment
}
