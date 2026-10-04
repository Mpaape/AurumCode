package main

import "github.com/Mpaape/AurumCode/internal/i18n"

// reviewCopy is the published review's interface text in one language. The
// texts live in the internal/i18n catalog; this struct only names them for
// the formatters.
type reviewCopy struct {
	title, verdict, summary, strengths, findings, suggestions, ciStatus, tests, limits string
	impact, evidence, suggestedFix, verify, rationale, proposedImplementation          string
	cause, fix, nextVerification                                                       string
	changesRequested, comment, approve, inconclusive                                   string
	blockingFindings, nonBlockingFindings, optionalSuggestions, noBlockingFindings     string
	qualityIncomplete                                                                  string
	suggestionApplicable, suggestionNotApplicable                                      string
	// coverageHeading and the coverage* templates render AUR-476's
	// deterministic "this review was partial" notice. Each reason a file was
	// not covered gets its own sentence; coverageSummary names the count and
	// the denominator so the reader sees how much of the diff actually ran.
	coverageHeading, coverageSummary, coveragePartial, coverageBudget, coverageIgnored, coverageFiltered, coverageNoStructure string
	// summaryWithheld is AUR-517's one-line notice (%d is the discard
	// count) printed in place of the "### Summary" block whenever
	// internal/review withheld the model's free-text summary because the
	// scope/evidence or rule gate discarded one of its proposed findings
	// (AC-001/N3a): the omission must be visible, never silent.
	summaryWithheld string
}

// reviewCopyFor reads the review's texts for language from the catalog.
func reviewCopyFor(language string) reviewCopy {
	return reviewCopy{
		title:                   i18n.Text(language, "review.title"),
		verdict:                 i18n.Text(language, "review.verdict"),
		summary:                 i18n.Text(language, "review.summary"),
		strengths:               i18n.Text(language, "review.strengths"),
		findings:                i18n.Text(language, "review.findings"),
		suggestions:             i18n.Text(language, "review.suggestions"),
		ciStatus:                i18n.Text(language, "review.ci_status"),
		tests:                   i18n.Text(language, "review.tests"),
		limits:                  i18n.Text(language, "review.limits"),
		impact:                  i18n.Text(language, "review.impact"),
		evidence:                i18n.Text(language, "review.evidence"),
		suggestedFix:            i18n.Text(language, "review.suggested_fix"),
		verify:                  i18n.Text(language, "review.verify"),
		rationale:               i18n.Text(language, "review.rationale"),
		proposedImplementation:  i18n.Text(language, "review.proposed_implementation"),
		cause:                   i18n.Text(language, "review.cause"),
		fix:                     i18n.Text(language, "review.fix"),
		nextVerification:        i18n.Text(language, "review.next_verification"),
		changesRequested:        i18n.Text(language, "review.changes_requested"),
		comment:                 i18n.Text(language, "review.comment"),
		approve:                 i18n.Text(language, "review.approve"),
		inconclusive:            i18n.Text(language, "review.inconclusive"),
		blockingFindings:        i18n.Text(language, "review.blocking_findings"),
		nonBlockingFindings:     i18n.Text(language, "review.non_blocking_findings"),
		optionalSuggestions:     i18n.Text(language, "review.optional_suggestions"),
		noBlockingFindings:      i18n.Text(language, "review.no_blocking_findings"),
		qualityIncomplete:       i18n.Text(language, "review.quality_incomplete"),
		suggestionApplicable:    i18n.Text(language, "review.suggestion_applicable"),
		suggestionNotApplicable: i18n.Text(language, "review.suggestion_not_applicable"),
		coverageHeading:         i18n.Text(language, "review.coverage_heading"),
		coverageSummary:         i18n.Text(language, "review.coverage_summary"),
		coveragePartial:         i18n.Text(language, "review.coverage_partial"),
		coverageBudget:          i18n.Text(language, "review.coverage_budget"),
		coverageIgnored:         i18n.Text(language, "review.coverage_ignored"),
		coverageFiltered:        i18n.Text(language, "review.coverage_filtered"),
		coverageNoStructure:     i18n.Text(language, "review.coverage_no_structure"),
		summaryWithheld:         i18n.Text(language, "review.summary_withheld"),
	}
}
