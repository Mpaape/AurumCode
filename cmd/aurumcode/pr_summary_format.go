// The published review body: summary, verdict, formal review event and the
// notices, in the review's language.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// formatFormalReviewSummary keeps an actionable native suggestion from being
// duplicated as a second code block in the review summary. The title,
// description and location remain in the summary while the replacement itself
// is attached to the exact changed lines by nativeSuggestionComment.
func formatFormalReviewSummary(result *types.ReviewResult, diff *types.Diff, language string, rule blocking.Rule) string {
	copy := *result
	copy.Suggestions = append([]types.ReviewSuggestion(nil), result.Suggestions...)
	for i := range copy.Suggestions {
		if isNativeSuggestion(diff, copy.Suggestions[i]) {
			copy.Suggestions[i].ProposedCode = ""
		}
	}
	return formatReviewDocument(&copy, diff, language, rule)
}

// A published review is one code-review document. The deterministic TL;DR
// and Mermaid diagram remain available in local CLI output, but prepending
// them here duplicated the verdict and mislabeled files without findings as
// "none". A diagram inferred from imports is not evidence about runtime flow.
func formatPublishedReviewBody(result *types.ReviewResult, diff *types.Diff, language string, formalWithInline bool, changelogText string) string {
	return formatGatedReviewBody(result, diff, language, formalWithInline, changelogText, blocking.Ungated())
}

// formatGatedReviewBody is formatPublishedReviewBody under rule: with a
// declared gate, the verdict, the blocking count and the finding labels
// follow the gate decision, so the document never claims a block the gate
// did not apply.
func formatGatedReviewBody(result *types.ReviewResult, diff *types.Diff, language string, formalWithInline bool, changelogText string, rule blocking.Rule) string {
	body := formatReviewDocument(result, diff, language, rule)
	if formalWithInline {
		body = formatFormalReviewSummary(result, diff, language, rule)
	}
	return appendChangelogSection(body, changelogText)
}

func formatReviewSummary(result *types.ReviewResult) string {
	return formatReviewSummaryForLanguage(result, "en-US")
}

func formatReviewSummaryForLanguage(result *types.ReviewResult, language string) string {
	return formatReviewSummaryForLanguageAndDiff(result, nil, language)
}

func formatReviewSummaryForLanguageAndDiff(result *types.ReviewResult, diff *types.Diff, language string) string {
	return formatReviewDocument(result, diff, language, blocking.Ungated())
}

// formatReviewDocument renders the review document under rule.
func formatReviewDocument(result *types.ReviewResult, diff *types.Diff, language string, rule blocking.Rule) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	b.WriteString("<!-- aurumcode-review -->\n")
	fmt.Fprintf(&b, "## AurumCode %s\n\n", copy.title)
	fmt.Fprintf(&b, "**%s:** %s\n\n", copy.verdict, gatedVerdictText(result, copy, rule))
	if diff != nil && prompt.HasSubstantiveCodeChange(diff) && strings.TrimSpace(result.Summary) != "" {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", copy.summary, strings.TrimSpace(result.Summary))
	} else if note := summaryWithheldNotice(result, copy); note != "" {
		fmt.Fprintf(&b, "%s\n\n", note)
	}
	b.WriteString(gatedSummaryText(result, copy, rule))
	b.WriteString("\n\n")

	if len(result.Strengths) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.strengths)
		writeReviewBullets(&b, result.Strengths)
		b.WriteString("\n")
	}

	writeFindingsSection(&b, result.Issues, copy, rule)

	if len(result.Suggestions) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.suggestions)
		for _, suggestion := range result.Suggestions {
			if strings.TrimSpace(suggestion.Title) == "" && strings.TrimSpace(suggestion.Description) == "" {
				continue
			}
			fmt.Fprintf(&b, "- **%s**", strings.TrimSpace(suggestion.Title))
			if suggestion.Description != "" {
				fmt.Fprintf(&b, " — %s", strings.TrimSpace(suggestion.Description))
			}
			if suggestion.File != "" {
				start, end := suggestionRange(suggestion)
				if start > 0 && end > 0 {
					if start == end {
						fmt.Fprintf(&b, " (`%s:%d`)", suggestion.File, start)
					} else {
						fmt.Fprintf(&b, " (`%s:%d-%d`)", suggestion.File, start, end)
					}
				}
			}
			b.WriteByte('\n')
			if strings.TrimSpace(suggestion.ProposedCode) != "" {
				fmt.Fprintf(&b, "  - **%s:**\n\n    ```\n%s\n    ```\n", copy.proposedImplementation, strings.TrimSpace(suggestion.ProposedCode))
			}
			writeSummaryField(&b, copy.rationale, suggestion.Rationale)
			writeSummaryField(&b, copy.verify, suggestion.Verification)
		}
		b.WriteString("\n")
	}

	writeCIStatusSection(&b, result, copy)

	if len(result.TestPlan) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.tests)
		writeReviewBullets(&b, result.TestPlan)
		b.WriteString("\n")
	}

	if len(result.Limitations) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.limits)
		writeReviewBullets(&b, result.Limitations)
	}

	return strings.TrimSpace(b.String()) + "\n"
}

func reviewVerdict(result *types.ReviewResult) string {
	return reviewVerdictForLanguage(result, reviewCopyFor("en-US"))
}

func reviewVerdictForLanguage(result *types.ReviewResult, copy reviewCopy) string {
	return gatedVerdictText(result, copy, blocking.Ungated())
}

// gatedVerdictText is the document's verdict under rule. A declared gate
// decides it the way the formal review event follows the gate: a failing
// gate requests changes, a passing one never does. A run whose quality
// review did not complete keeps reading inconclusive: the gate fails it for
// that reason, and inconclusive already never approves.
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
	if result.Metadata["quality_degraded"] == "true" {
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
	// set or erase it. A prior version of this check matched
	// result.Verdict == "comment" directly, which regressed a model that
	// legitimately self-reports "comment" with no gate active at all: it
	// started publishing COMMENT instead of this function's pre-AUR-519
	// APPROVE default, an outcome this function never produced before
	// and the model's self-report alone must not be able to cause.
	if result.Metadata[prompt.PolicyGateWithheldKey] == "true" {
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
	if result.Metadata["quality_degraded"] == "true" {
		return "COMMENT"
	}
	// AUR-519 (B-V): same engine-owned marker as reviewVerdictForLanguage
	// above, never the model's own Verdict text -- see that function's
	// comment and prompt.PolicyGateWithheldKey's own doc.
	if result.Metadata[prompt.PolicyGateWithheldKey] == "true" {
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
// an empty result.Summary with a nonzero count is exactly that withholding
// (never a model that happened to return no summary at all with nothing
// discarded). Returns "" in every other case, so a clean review's body is
// unchanged.
func summaryWithheldNotice(result *types.ReviewResult, copy reviewCopy) string {
	if result == nil || strings.TrimSpace(result.Summary) != "" {
		return ""
	}
	discarded := atoiOrZero(result.Metadata["summary_discarded_findings"])
	if discarded <= 0 {
		return ""
	}
	return fmt.Sprintf(copy.summaryWithheld, discarded)
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

// gatedSummaryText is the document's one-line conclusion under rule: the
// blocking count is the rule's, and with a declared gate that passed every
// finding is named a non-blocking observation below the threshold.
func gatedSummaryText(result *types.ReviewResult, copy reviewCopy, rule blocking.Rule) string {
	if count := rule.Count(result.Issues); count > 0 {
		return fmt.Sprintf(copy.blockingFindings, count)
	}
	if rule.Fails() {
		if result.Metadata["quality_degraded"] == "true" {
			return copy.qualityIncomplete
		}
		return copy.gateFailed
	}
	if rule.Gated() && len(result.Issues) > 0 {
		return fmt.Sprintf(copy.belowGateThreshold, len(result.Issues))
	}
	if len(result.Issues) > 0 {
		return copy.nonBlockingFindings
	}
	if result.Metadata["quality_degraded"] == "true" {
		return copy.qualityIncomplete
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.optionalSuggestions
		}
	}
	return copy.noBlockingFindings
}

func writeReviewBullets(b *strings.Builder, values []string) {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			fmt.Fprintf(b, "- %s\n", text)
		}
	}
}

func writeSummaryField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) != "" {
		fmt.Fprintf(b, "  - **%s:** %s\n", label, strings.TrimSpace(value))
	}
}
