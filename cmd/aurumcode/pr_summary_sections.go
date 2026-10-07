// Sections of the published review document that depend on the blocking
// rule or on the facts of this run: the findings and the CI status.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// writeFindingsSection lists the findings. Under a declared gate a finding
// the gate did not fail on carries the non-blocking label, so the reader
// sees which observations stay below the threshold.
func writeFindingsSection(b *strings.Builder, issues []types.ReviewIssue, copy reviewCopy, rule blocking.Rule) {
	sorted := sortedIssues(issues)
	if len(sorted) == 0 {
		return
	}
	fmt.Fprintf(b, "### %s\n\n", copy.findings)
	for _, issue := range sorted {
		label := ""
		if rule.Gated() && !rule.Blocks(issue) {
			label = " (" + copy.nonBlockingLabel + ")"
		}
		fmt.Fprintf(b, "- **[%s] %s:%d**%s — %s\n", issue.Severity, issue.File, issue.Line, label, issue.Message)
		if issue.Side == "LEFT" {
			fmt.Fprintln(b, "  - `LEFT`: base / −")
		}
		writeFindingFields(b, issue, copy)
		printAssessment(b, issue)
	}
	b.WriteString("\n")
}

// writeCIStatusSection renders the CI status. When every analysis item was
// discarded as not a fact of this run, the section says so in one line
// instead of disappearing.
func writeCIStatusSection(b *strings.Builder, result *types.ReviewResult, copy reviewCopy) {
	if len(result.CIAnalysis) == 0 {
		if discarded := atoiOrZero(result.Metadata[ciStatusDiscardedKey]); discarded > 0 {
			fmt.Fprintf(b, "### %s\n\n%s\n\n", copy.ciStatus, fmt.Sprintf(copy.ciNothingFailed, discarded))
		}
		return
	}
	fmt.Fprintf(b, "### %s\n\n", copy.ciStatus)
	for _, analysis := range result.CIAnalysis {
		fmt.Fprintf(b, "- **%s — %s**\n", analysis.Check, analysis.Status)
		writeSummaryField(b, copy.cause, analysis.Cause)
		writeSummaryField(b, copy.evidence, analysis.Evidence)
		writeSummaryField(b, copy.fix, analysis.Fix)
		writeSummaryField(b, copy.nextVerification, analysis.NextVerification)
	}
	b.WriteString("\n")
}
