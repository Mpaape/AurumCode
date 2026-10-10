// The published review document (the parecer): the decision first, what to
// fix, what only to note, the summary, and everything else behind a
// collapsed details block, in the review's language.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// reviewBodyMarker opens every parecer, so a later round finds the one it
// edits in place.
const reviewBodyMarker = "<!-- aurumcode-review -->"

// metaReviewParts is the metadata key with how many parts (batches) the
// diff was reviewed in, when it did not fit one prompt.
const metaReviewParts = "review_parts"

// Caps of the collapsed details: the model's prose is kept short there.
// Its strengths are not published at all: praise is not information a
// reader of the parecer acts on.
const (
	maxTestPlan         = 3
	maxModelLimitations = 3
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

// formatPublishedReviewBody is the parecer of a run without a declared gate.
func formatPublishedReviewBody(result *types.ReviewResult, diff *types.Diff, language string, formalWithInline bool, changelogText string) string {
	return formatGatedReviewBody(result, diff, language, formalWithInline, changelogText, blocking.Ungated())
}

// formatGatedReviewBody is the parecer under rule: with a declared gate, the
// decision, the blocking count and the finding labels follow the gate, so the
// document never claims a block the gate did not apply.
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

// formatReviewDocument renders the parecer under rule.
func formatReviewDocument(result *types.ReviewResult, diff *types.Diff, language string, rule blocking.Rule) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	b.WriteString(reviewBodyMarker + "\n")
	writeParecerHead(&b, result, diff, copy, rule)
	b.WriteString("\n")

	blockingIssues, observations := splitFindings(result.Issues, rule)
	if len(blockingIssues) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.fixBeforeMerge)
		for _, issue := range blockingIssues {
			writeBlockingFinding(&b, issue, copy)
		}
		b.WriteString("\n")
	}
	outside := review.OutsideDiffFindings(result)
	if len(observations)+len(outside) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.observations)
		for _, issue := range observations {
			writeObservation(&b, issue, "")
		}
		for _, issue := range outside {
			writeObservation(&b, issue, copy.outsideDiffLabel)
		}
		b.WriteString("\n")
	}

	if diff != nil && prompt.HasSubstantiveCodeChange(diff) && strings.TrimSpace(result.Summary) != "" {
		writeModelSummary(&b, result, diff, copy)
	} else if note := summaryWithheldNotice(result, copy); note != "" {
		fmt.Fprintf(&b, "%s\n\n", note)
	}

	if details := detailsSections(result, copy); details != "" {
		b.WriteString(detailsBlock(copy.details, details))
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// writeParecerHead writes what the parecer and the terminal report open
// with: the title, the decision headline and the facts line.
func writeParecerHead(b *strings.Builder, result *types.ReviewResult, diff *types.Diff, copy reviewCopy, rule blocking.Rule) {
	fmt.Fprintf(b, "## AurumCode %s\n\n", copy.title)
	b.WriteString(formatHeadline(result, copy, rule))
	b.WriteString("\n\n")
	b.WriteString(factsLine(result, diff, copy, rule))
	b.WriteString("\n")
}

// writeModelSummary writes the model's summary when the change is
// substantive and the model wrote one.
func writeModelSummary(b *strings.Builder, result *types.ReviewResult, diff *types.Diff, copy reviewCopy) {
	if diff != nil && prompt.HasSubstantiveCodeChange(diff) && strings.TrimSpace(result.Summary) != "" {
		fmt.Fprintf(b, "### %s\n\n%s\n\n", copy.summary, strings.TrimSpace(result.Summary))
	}
}

// reviewDecision is the one decision the headline, the facts line and the
// terminal report read from the blocking rule.
type reviewDecision int

const (
	decisionApproved reviewDecision = iota
	decisionObservations
	decisionInconclusive
	decisionBlocked
)

// decide reads the run's decision: blocking findings block; a declared
// gate that failed without one, a model review that did not complete and
// an approval the gate withheld are inconclusive (never approved); findings
// below the threshold are observations; nothing is approval.
func decide(result *types.ReviewResult, rule blocking.Rule) (reviewDecision, int) {
	if n := rule.Count(result.Issues); n > 0 {
		return decisionBlocked, n
	}
	if rule.Fails() || result.Metadata["quality_degraded"] == metaTrue || result.Metadata[prompt.PolicyGateWithheldKey] == metaTrue {
		return decisionInconclusive, 0
	}
	if n := len(result.Issues) + len(review.OutsideDiffFindings(result)); n > 0 {
		return decisionObservations, n
	}
	return decisionApproved, 0
}

// metaTrue is the value of a set metadata flag.
const metaTrue = "true"

// formatHeadline is the decision as a GitHub alert block: the host draws
// the colored band, the text says what the reader must do.
func formatHeadline(result *types.ReviewResult, copy reviewCopy, rule blocking.Rule) string {
	decision, n := decide(result, rule)
	switch decision {
	case decisionBlocked:
		return alertBlock("CAUTION", countText(n, copy.headlineBlockedOne, copy.headlineBlocked), "")
	case decisionInconclusive:
		reason := copy.gateFailed
		if result.Metadata["quality_degraded"] == metaTrue {
			reason = copy.qualityIncomplete
		}
		return alertBlock("WARNING", copy.headlineInconclusive, reason)
	case decisionObservations:
		return alertBlock("NOTE", countText(n, copy.headlineObservationsOne, copy.headlineObservations), "")
	}
	if atoiOrZero(result.Metadata["summary_discarded_findings"]) > 0 {
		// The model proposed findings the gates discarded for lack of
		// proof: approved, but never "no problem found".
		return alertBlock("TIP", copy.headlineApprovedUnproven, "")
	}
	return alertBlock("TIP", copy.headlineApproved, "")
}

// countText is the singular text for one, the plural template otherwise.
func countText(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}

// alertBlock renders GitHub's alert syntax: the kind on the first line, the
// bold headline, then an optional second line.
func alertBlock(kind, headline, second string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "> [!%s]\n> **%s**", kind, headline)
	if strings.TrimSpace(second) != "" {
		fmt.Fprintf(&b, "\n> %s", strings.TrimSpace(second))
	}
	return b.String()
}

// factsLine is the line under the headline: the gate, how many files were
// reviewed and in how many parts.
func factsLine(result *types.ReviewResult, diff *types.Diff, copy reviewCopy, rule blocking.Rule) string {
	gate := copy.factsNoGate
	if rule.Gated() {
		gate = copy.factsGatePassed
		if rule.Fails() {
			gate = copy.factsGateFailed
		}
	}
	parts := []string{gate}
	files := 0
	if diff != nil {
		files = len(diff.Files)
	} else if n, err := strconv.Atoi(result.Metadata["total_files"]); err == nil {
		files = n
	}
	if files > 0 {
		filesText := countText(files, copy.factsFilesOne, copy.factsFiles)
		if n, err := strconv.Atoi(result.Metadata[metaReviewParts]); err == nil && n > 1 {
			filesText += " " + fmt.Sprintf(copy.factsParts, n)
		}
		parts = append(parts, filesText)
	}
	return strings.Join(parts, " · ")
}

// splitFindings separates what blocks from what is only noted, both sorted.
func splitFindings(issues []types.ReviewIssue, rule blocking.Rule) (blockingIssues, observations []types.ReviewIssue) {
	for _, issue := range sortedIssues(issues) {
		if rule.Blocks(issue) {
			blockingIssues = append(blockingIssues, issue)
		} else {
			observations = append(observations, issue)
		}
	}
	return blockingIssues, observations
}

// writeBlockingFinding is one problem to fix: where, what, the rule, and
// the fields that help fix it.
func writeBlockingFinding(b *strings.Builder, issue types.ReviewIssue, copy reviewCopy) {
	fmt.Fprintf(b, "- **`%s:%d`** — %s", issue.File, issue.Line, strings.TrimSpace(issue.Message))
	if issue.Side == "LEFT" {
		b.WriteString(" (`LEFT`)")
	}
	writeRuleSuffix(b, issue)
	b.WriteString("\n")
	writeFindingFields(b, issue, copy)
	printAssessment(b, issue)
}

// writeObservation is one non-blocking finding, in one line.
func writeObservation(b *strings.Builder, issue types.ReviewIssue, label string) {
	fmt.Fprintf(b, "- `%s:%d` — %s", issue.File, issue.Line, strings.TrimSpace(issue.Message))
	writeRuleSuffix(b, issue)
	if label != "" {
		fmt.Fprintf(b, " — %s", label)
	}
	b.WriteString("\n")
}

// writeRuleSuffix names the rule after a finding whose message does not
// already cite it (the engine appends the catalog citation to the model's
// findings; a scanner's message may not carry the rule id).
func writeRuleSuffix(b *strings.Builder, issue types.ReviewIssue) {
	rule := strings.TrimSpace(issue.RuleID)
	if rule == "" || strings.Contains(issue.Message, rule) {
		return
	}
	fmt.Fprintf(b, " (`%s`)", rule)
}

// detailsSections is the content of the collapsed block: the model's
// suggestions, the CI status, the tests, the limitations and, for a build
// with a stamped version, the AurumCode that reviewed (AUR-611).
func detailsSections(result *types.ReviewResult, copy reviewCopy) string {
	var b strings.Builder
	writeSuggestionsSection(&b, result, copy)
	writeCIStatusSection(&b, result, copy)
	if len(result.TestPlan) > 0 || result.Metadata[metaAffectedTests] != "" {
		fmt.Fprintf(&b, "#### %s\n\n", copy.tests)
		writeReviewBullets(&b, capped(result.TestPlan, maxTestPlan))
		writeAffectedTests(&b, result, copy)
		b.WriteString("\n")
	}
	if len(result.Limitations) > 0 {
		fmt.Fprintf(&b, "#### %s\n\n", copy.limits)
		writeReviewBullets(&b, result.Limitations)
	}
	return joinDetails(strings.TrimSpace(b.String()), toolVersionLine(toolVersion(), copy))
}

// writeSuggestionsSection lists the model's optional suggestions.
func writeSuggestionsSection(b *strings.Builder, result *types.ReviewResult, copy reviewCopy) {
	shown := 0
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			shown++
		}
	}
	if shown == 0 {
		return
	}
	fmt.Fprintf(b, "#### %s\n\n", copy.suggestions)
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) == "" && strings.TrimSpace(suggestion.Description) == "" {
			continue
		}
		fmt.Fprintf(b, "- **%s**", strings.TrimSpace(suggestion.Title))
		if suggestion.Description != "" {
			fmt.Fprintf(b, " — %s", strings.TrimSpace(suggestion.Description))
		}
		if suggestion.File != "" {
			start, end := suggestionRange(suggestion)
			if start > 0 && end > 0 {
				if start == end {
					fmt.Fprintf(b, " (`%s:%d`)", suggestion.File, start)
				} else {
					fmt.Fprintf(b, " (`%s:%d-%d`)", suggestion.File, start, end)
				}
			}
		}
		b.WriteByte('\n')
		if strings.TrimSpace(suggestion.ProposedCode) != "" {
			fmt.Fprintf(b, "  - **%s:**\n\n    ```\n%s\n    ```\n", copy.proposedImplementation, strings.TrimSpace(suggestion.ProposedCode))
		}
		writeSummaryField(b, copy.rationale, suggestion.Rationale)
		writeSummaryField(b, copy.verify, suggestion.Verification)
	}
	b.WriteString("\n")
}

// detailsBlock wraps content in GitHub's collapsed block.
func detailsBlock(title, content string) string {
	return "<details>\n<summary>" + title + "</summary>\n\n" + strings.TrimSpace(content) + "\n\n</details>\n"
}

// appendBodySection adds a section to the parecer before the collapsed
// details, so what is actionable stays in view.
func appendBodySection(body, section string) string {
	section = strings.TrimSpace(section)
	if section == "" {
		return body
	}
	if i := strings.Index(body, "<details>"); i >= 0 {
		return strings.TrimRight(body[:i], "\n") + "\n\n" + section + "\n\n" + body[i:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + section + "\n"
}

// appendDetailSection adds a section inside the collapsed details, creating
// the block when the parecer has none.
func appendDetailSection(body, title, section string) string {
	section = strings.TrimSpace(section)
	if section == "" {
		return body
	}
	if i := strings.LastIndex(body, "\n</details>"); i >= 0 {
		return body[:i] + "\n\n" + section + body[i:]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + detailsBlock(title, section)
}

// capped is the first n values.
func capped(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func writeReviewBullets(b *strings.Builder, values []string) {
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if title, rest, ok := strings.Cut(text, "\n"); ok {
			// A multi-line note (the coverage block) is its own list: the
			// first line titles it and the rest are already bullets.
			fmt.Fprintf(b, "\n**%s**\n\n%s\n\n", title, rest)
			continue
		}
		fmt.Fprintf(b, "- %s\n", text)
	}
}

func writeSummaryField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) != "" {
		fmt.Fprintf(b, "  - **%s:** %s\n", label, strings.TrimSpace(value))
	}
}
