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
	// belowGateThreshold, gateFailed and nonBlockingLabel are the texts of a
	// run with a declared gate, where blocking means what the gate fails on.
	belowGateThreshold, gateFailed, nonBlockingLabel string
	// ciNothingFailed is the one line of a CI status section whose every
	// item was discarded as not a fact of this run (%d is the count).
	ciNothingFailed                               string
	suggestionApplicable, suggestionNotApplicable string
	// The ci* texts separate what the CI context observed from what the
	// model inferred (cmd/aurumcode/ci_status_render.go).
	ciVerified, ciUnverified, ciUnverifiedNote, ciCauseUnknown, ciHypothesis string
	ciDiagnose, ciDiagnoseNoLink, ciObserved, ciInferredCause, ciInferredFix string
	// coverageHeading and the coverage* templates render AUR-476's
	// deterministic "this review was partial" notice. Each reason a file was
	// not covered gets its own sentence; coverageSummary names the count and
	// the denominator so the reader sees how much of the diff actually ran.
	coverageHeading, coverageSummary, coveragePartial, coverageBudget, coverageIgnored, coverageFiltered, coverageNoStructure, coverageMore string
	// summaryWithheld is AUR-517's one-line notice (%d is the discard
	// count) printed in place of the "### Summary" block whenever
	// internal/review withheld the model's free-text summary because the
	// scope/evidence or rule gate discarded one of its proposed findings
	// (AC-001/N3a): the omission must be visible, never silent.
	summaryWithheld string
	// The headline* texts are the one-line decision of the parecer, read
	// from the blocking rule; the facts* texts are the line under it; the
	// section names below head what to fix, what only to note, and the
	// collapsed details.
	headlineBlocked, headlineInconclusive, headlineObservations, headlineApproved string
	factsGatePassed, factsGateFailed, factsNoGate, factsFiles, factsParts         string
	fixBeforeMerge, observations, outsideDiffLabel, details, affectedTests        string
	// The *One texts are the singular forms of the counted headline and
	// facts texts.
	headlineBlockedOne, headlineObservationsOne, factsFilesOne string
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
		belowGateThreshold:      i18n.Text(language, "review.below_gate_threshold"),
		gateFailed:              i18n.Text(language, "review.gate_failed"),
		nonBlockingLabel:        i18n.Text(language, "review.non_blocking_label"),
		ciNothingFailed:         i18n.Text(language, "review.ci_status_nothing_failed"),
		ciVerified:              i18n.Text(language, "review.ci_verified"),
		ciUnverified:            i18n.Text(language, "review.ci_unverified"),
		ciUnverifiedNote:        i18n.Text(language, "review.ci_unverified_note"),
		ciCauseUnknown:          i18n.Text(language, "review.ci_cause_unknown"),
		ciHypothesis:            i18n.Text(language, "review.ci_hypothesis"),
		ciDiagnose:              i18n.Text(language, "review.ci_diagnose"),
		ciDiagnoseNoLink:        i18n.Text(language, "review.ci_diagnose_no_link"),
		ciObserved:              i18n.Text(language, "review.ci_observed"),
		ciInferredCause:         i18n.Text(language, "review.ci_inferred_cause"),
		ciInferredFix:           i18n.Text(language, "review.ci_inferred_fix"),
		suggestionApplicable:    i18n.Text(language, "review.suggestion_applicable"),
		suggestionNotApplicable: i18n.Text(language, "review.suggestion_not_applicable"),
		coverageHeading:         i18n.Text(language, "review.coverage_heading"),
		coverageSummary:         i18n.Text(language, "review.coverage_summary"),
		coveragePartial:         i18n.Text(language, "review.coverage_partial"),
		coverageBudget:          i18n.Text(language, "review.coverage_budget"),
		coverageIgnored:         i18n.Text(language, "review.coverage_ignored"),
		coverageFiltered:        i18n.Text(language, "review.coverage_filtered"),
		coverageNoStructure:     i18n.Text(language, "review.coverage_no_structure"),
		coverageMore:            i18n.Text(language, "review.coverage_more"),
		summaryWithheld:         i18n.Text(language, "review.summary_withheld"),
		headlineBlocked:         i18n.Text(language, "review.headline_blocked"),
		headlineBlockedOne:      i18n.Text(language, "review.headline_blocked_one"),
		headlineInconclusive:    i18n.Text(language, "review.headline_inconclusive"),
		headlineObservations:    i18n.Text(language, "review.headline_observations"),
		headlineObservationsOne: i18n.Text(language, "review.headline_observations_one"),
		headlineApproved:        i18n.Text(language, "review.headline_approved"),
		factsGatePassed:         i18n.Text(language, "review.facts_gate_passed"),
		factsGateFailed:         i18n.Text(language, "review.facts_gate_failed"),
		factsNoGate:             i18n.Text(language, "review.facts_no_gate"),
		factsFiles:              i18n.Text(language, "review.facts_files"),
		factsFilesOne:           i18n.Text(language, "review.facts_files_one"),
		factsParts:              i18n.Text(language, "review.facts_parts"),
		fixBeforeMerge:          i18n.Text(language, "review.fix_before_merge"),
		observations:            i18n.Text(language, "review.observations"),
		outsideDiffLabel:        i18n.Text(language, "review.outside_diff_label"),
		details:                 i18n.Text(language, "review.details"),
		affectedTests:           i18n.Text(language, "review.affected_tests"),
	}
}
