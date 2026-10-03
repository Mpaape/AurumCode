// The --base publisher: write the compliance artifacts, print the terminal
// report and decide the exit code.
package main

import (
	"fmt"
	"strings"
)

// publish writes AUR-521's audit record and SARIF once the gate decision is
// final, prints the report and returns the session's exit decision.
func (b *baseReview) publish() (int, bool) {
	artifactFailures := b.writeArtifacts()
	b.printReport()
	return b.decideExit(publishOutcome{artifactsMissing: len(artifactFailures) > 0}), true
}

// printReport prints the --base report (AUR-490): diff notices, the report
// with its summary, the coverage notice (AUR-476), suggestions, the
// changelog and the findings. A skipped or failed quality review is never
// reported as "No issues found." (AUR-449/458): the report says it covers
// deterministic analysis only and the verdict is "comment".
func (b *baseReview) printReport() {
	result := b.result
	printNotices(b.stdout, b.filter, b.notices)
	if b.qualityDidNotRun() {
		result.Verdict = "comment"
		// canonicalVerdict reads this same flag, so a quality-degraded
		// result with no deterministic findings cannot canonicalize to
		// "approve" (AUR-449/458).
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["quality_degraded"] = "true"
		fmt.Fprintln(b.stdout, "LLM quality review did not run. The following report covers deterministic analysis only.")
	}
	fmt.Fprint(b.stdout, renderLocalReport(result, b.diff, b.reviewLanguage))
	if b.coverageText != "" {
		fmt.Fprint(b.stdout, "\n"+b.coverageText+"\n")
	}
	if len(b.skillNotices) > 0 {
		var notes strings.Builder
		fmt.Fprintf(&notes, "\n### %s\n\n", reviewCopyFor(b.reviewLanguage).limits)
		writeReviewBullets(&notes, b.skillNotices)
		fmt.Fprint(b.stdout, notes.String())
	}
	if suggestions := renderSuggestions(result, b.diff, b.reviewLanguage); suggestions != "" {
		fmt.Fprint(b.stdout, "\n"+suggestions)
	}
	if b.changelogText != "" {
		fmt.Fprint(b.stdout, "\n"+b.changelogText)
	}
	if !b.qualityDidNotRun() || len(result.Issues) > 0 {
		printFindings(b.stdout, result, b.gateRes.Reason)
	}
	if b.model != modelProviderFailed {
		persistReviewMemory(b.memoryStore, b.cfg.Review.Memory, b.memoryNotes, result.Issues, b.stderr, b.filter)
	}
	if b.f.seguranca {
		printSecurityFindings(b.stdout, b.filter, b.securityFindings)
	}
	if b.proposedExceptions != "" {
		fmt.Fprint(b.stdout, "\n"+b.filter.Redact(b.proposedExceptions))
	}
}
