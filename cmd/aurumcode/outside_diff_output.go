// Output of the model findings that point outside the changed lines but
// carry full proof: a general comment, never inline, never counted by the
// policy gate (internal/review.OutsideDiffFindings).
package main

import (
	"fmt"
	"io"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// outsideDiffLocalMarker tells a --base reader where the finding stands: it
// is information about code the change did not touch, not a gate input.
const outsideDiffLocalMarker = "-- fora das linhas alteradas (comentario geral; nao conta para o gate)"

// outsideDiffPublishedMarker is the --pr stdout marker of a general comment,
// the same one an unanchorable inline finding has always used.
const outsideDiffPublishedMarker = "-- publicado como comentario geral"

// outsideDiffLine is the one-line location and message of a finding.
func outsideDiffLine(issue types.ReviewIssue) string {
	return fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
}

// printOutsideDiffFindings lists, after the gate-counted findings of a
// --base report, the general-comment findings of result.
func printOutsideDiffFindings(stdout io.Writer, result *types.ReviewResult) {
	for _, issue := range review.OutsideDiffFindings(result) {
		fmt.Fprintf(stdout, "%s %s\n", outsideDiffLine(issue), outsideDiffLocalMarker)
	}
}

// outsideDiffCommentBody is the finding's usual body headed by its location:
// a general comment has no anchor, so the reader needs the file and line.
func outsideDiffCommentBody(issue types.ReviewIssue, language string) string {
	return fmt.Sprintf("`%s:%d`\n\n%s", issue.File, issue.Line, formatInlineIssueForLanguage(issue, language))
}

// postOutsideDiffFindings publishes each general-comment finding as a pull
// request (issue) comment, whatever the publication mode: there is no line
// to anchor it to. A failed POST is recorded and the loop continues.
func (p *prReview) postOutsideDiffFindings() (published int, failures []string) {
	for _, issue := range review.OutsideDiffFindings(p.result) {
		if err := p.client.PostIssueComment(p.ctx, p.owner, p.repoName, p.prNumber, outsideDiffCommentBody(issue, p.reviewLanguage)); err != nil {
			fmt.Fprintf(p.stderr, "aurumcode review: publishing general comment for %s:%d: %v\n", issue.File, issue.Line, err)
			failures = append(failures, fmt.Sprintf("%s:%d (geral): %v", issue.File, issue.Line, err))
			continue
		}
		fmt.Fprintf(p.stdout, "%s %s\n", outsideDiffLine(issue), outsideDiffPublishedMarker)
		published++
	}
	return published, failures
}
