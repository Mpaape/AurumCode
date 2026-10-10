// The verdict word and the one-line conclusion of a review, read from the
// findings and the blocking rule: what the terminal report, the audit and
// the formal review event share with the parecer's headline.
package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func reviewVerdict(result *types.ReviewResult) string {
	return reviewVerdictForLanguage(result, reviewCopyFor("en-US"))
}

func reviewVerdictForLanguage(result *types.ReviewResult, copy reviewCopy) string {
	return gatedVerdictText(result, copy, blocking.Ungated())
}

// gatedVerdictText is the verdict word under rule, the one the terminal
// report and the audit keep: a declared gate decides it the way the formal
// review event follows the gate. A run whose quality review did not
// complete keeps reading inconclusive.
func gatedVerdictText(result *types.ReviewResult, copy reviewCopy, rule blocking.Rule) string {
	verdict := ungatedVerdictText(result, copy)
	if !rule.Gated() {
		return verdict
	}
	if rule.Fails() && verdict != copy.inconclusive {
		return copy.changesRequested
	}
	if !rule.Fails() && verdict == copy.changesRequested {
		return copy.comment
	}
	return verdict
}

// ungatedVerdictText is the historical verdict, read from the findings
// alone: any error or warning requests changes.
func ungatedVerdictText(result *types.ReviewResult, copy reviewCopy) string {
	for _, issue := range result.Issues {
		switch strings.ToLower(issue.Severity) {
		case "error", "warning":
			return copy.changesRequested
		}
	}
	if len(result.Issues) > 0 {
		return copy.comment
	}
	if result.Metadata["quality_degraded"] == metaTrue {
		return copy.inconclusive
	}
	// AUR-519 (B-V): this function otherwise re-derives the verdict from
	// Issues/Suggestions alone, never from the model's own Verdict field
	// -- intentionally: a model is never trusted to self-report "comment"
	// or "changes_requested" into an outcome these structural checks did
	// not already reach on their own. The engine's OWN withholding
	// (gate.Fail/Inconclusive, cmd/aurumcode's gate section) is a
	// different, trusted signal and gets its own reserved,
	// forge-safe key instead of overloading result.Verdict's value --
	// see prompt.PolicyGateWithheldKey's own doc for why a model cannot
	// set or erase it.
	if result.Metadata[prompt.PolicyGateWithheldKey] == metaTrue {
		return copy.comment
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.comment
		}
	}
	return copy.approve
}

// formalReviewEvent maps AurumCode's review result to GitHub's formal review
// events. Blocking findings request changes; non-blocking observations stay
// a neutral review comment; a clean review can approve the pull request.
func formalReviewEvent(result *types.ReviewResult) string {
	for _, issue := range result.Issues {
		switch strings.ToLower(strings.TrimSpace(issue.Severity)) {
		case "error", "warning":
			return "REQUEST_CHANGES"
		}
	}
	if len(result.Issues) > 0 {
		return "COMMENT"
	}
	if result.Metadata["quality_degraded"] == metaTrue {
		return "COMMENT"
	}
	// AUR-519 (B-V): same engine-owned marker as reviewVerdictForLanguage
	// above, never the model's own Verdict text.
	if result.Metadata[prompt.PolicyGateWithheldKey] == metaTrue {
		return "COMMENT"
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return "COMMENT"
		}
	}
	return "APPROVE"
}

// summaryWithheldNotice renders AUR-517/N3a's visible notice when
// internal/review withheld result.Summary (withholdSummaryWhenFiltered):
// result.Metadata["summary_discarded_findings"] names how many of the
// model's proposed findings the scope/evidence or rule gate discarded, and
// an empty result.Summary with a nonzero count is exactly that withholding.
// Returns "" in every other case, so a clean review's body is unchanged.
func summaryWithheldNotice(result *types.ReviewResult, copy reviewCopy) string {
	if result == nil || strings.TrimSpace(result.Summary) != "" {
		return ""
	}
	discarded := atoiOrZero(result.Metadata["summary_discarded_findings"])
	if discarded <= 0 {
		return ""
	}
	return countText(discarded, copy.summaryWithheldOne, copy.summaryWithheld)
}

// reviewSummaryText is deliberately derived from the filtered result rather
// than copied from result.Summary. The model summary can become stale when a
// source-aware gate removes a false positive; publishing it would produce a
// contradictory verdict and review comment.
func reviewSummaryText(result *types.ReviewResult) string {
	return reviewSummaryTextForLanguage(result, reviewCopyFor("en-US"))
}

func reviewSummaryTextForLanguage(result *types.ReviewResult, copy reviewCopy) string {
	return gatedSummaryText(result, copy, blocking.Ungated())
}

// gatedSummaryText is the one-line conclusion under rule, kept for the
// terminal report: the blocking count is the rule's, and with a declared
// gate that passed every finding is named a non-blocking observation.
func gatedSummaryText(result *types.ReviewResult, copy reviewCopy, rule blocking.Rule) string {
	if count := rule.Count(result.Issues); count > 0 {
		return countText(count, copy.blockingFindingsOne, copy.blockingFindings)
	}
	if rule.Fails() {
		if result.Metadata["quality_degraded"] == metaTrue {
			return copy.qualityIncomplete
		}
		return copy.gateFailed
	}
	if rule.Gated() && len(result.Issues) > 0 {
		return countText(len(result.Issues), copy.belowGateThresholdOne, copy.belowGateThreshold)
	}
	if len(result.Issues) > 0 {
		return copy.nonBlockingFindings
	}
	if result.Metadata["quality_degraded"] == metaTrue {
		return copy.qualityIncomplete
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.optionalSuggestions
		}
	}
	return copy.noBlockingFindings
}
