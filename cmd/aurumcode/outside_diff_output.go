// Output of the model findings that point outside the changed lines but
// carry full proof: listed among the observations of the parecer, never
// inline, never counted by the policy gate
// (internal/review.OutsideDiffFindings).
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
