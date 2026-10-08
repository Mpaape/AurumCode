// The local (--base) report: verdict, rendered summary and the suggestions
// with their applicable ranges.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// renderPass is the render pass: it renders the deterministic, localized
// summary (render.Summary) and the Mermaid flow diagram (render.Mermaid) from
// the already-redacted result and the diff. The --base path prints them to
// stdout; the PR path publishes a single code-review document instead.
//
// render.Summary prints result.Verdict verbatim, and that field is whatever
// the model itself claimed in its JSON response -- the same kind of
// untrusted, potentially stale self-report result.Summary is. localVerdict
// substitutes the pipeline's own decision before it reaches render.Summary,
// so --base can never show a different outcome than the PR comment or the
// formal review publish for the identical, already-filtered result (AUR-517
// AC-003).
func renderPass(result *types.ReviewResult, diff *types.Diff, language, verdict string) (tldr string, diagram string) {
	labeled := localVerdict(result)
	if labeled != nil {
		labeled.Verdict = verdict
	}
	tldr = render.Summary(labeled, language)
	if d, err := render.Mermaid(diff); err == nil {
		diagram = d
	}
	return tldr, diagram
}

// localVerdict returns a shallow copy of result with Verdict replaced by
// canonicalVerdict's decision. Every other field, Issues and Suggestions
// included, is result's own: only the three-way outcome label changes, and
// it changes to the exact decision formalReviewEvent already computes for
// the published PR paths from this same, already-filtered result -- one
// decision, three vocabularies (formalReviewEvent's GitHub event,
// reviewVerdictForLanguage's localized text, and this enum), never three
// independent readings of the issues that could drift apart.
func localVerdict(result *types.ReviewResult) *types.ReviewResult {
	if result == nil {
		return result
	}
	withVerdict := *result
	withVerdict.Verdict = canonicalVerdict(result)
	return &withVerdict
}

// canonicalVerdict maps formalReviewEvent's GitHub review event to the
// verdict enum result.Verdict and render.Summary already use
// ("approve"/"changes_requested"/"comment").
func canonicalVerdict(result *types.ReviewResult) string {
	return verdictForEvent(formalReviewEvent(result))
}

// verdictForEvent maps a GitHub review event to the verdict enum.
func verdictForEvent(event string) string {
	switch event {
	case blocking.EventRequestChanges:
		return verdictChangesRequested
	case blocking.EventComment:
		return verdictComment
	default:
		return verdictApprove
	}
}

// eventForVerdict is verdictForEvent's inverse.
func eventForVerdict(verdict string) string {
	switch verdict {
	case verdictChangesRequested:
		return blocking.EventRequestChanges
	case verdictComment:
		return blocking.EventComment
	default:
		return eventApprove
	}
}

// gateRuleVerdict is the terminal report's verdict under the blocking rule:
// the same alignment the published review event follows, so the local
// report and the pull request never disagree with the gate.
func gateRuleVerdict(rule blocking.Rule, verdict string) string {
	return verdictForEvent(rule.Event(eventForVerdict(verdict)))
}

// The verdict enum and the approving review event.
const (
	verdictChangesRequested = "changes_requested"
	verdictComment          = "comment"
	verdictApprove          = "approve"
	eventApprove            = "APPROVE"
)

// applicableSuggestionRange reports the 1-based start/end of a suggestion whose
// replacement can actually be applied, reusing the same coordinate rules the
// PR publication path already enforces (suggestionRange + isInlineEligible):
// every line of the range must be a line this diff added, so the replacement
// has an exact, reviewable boundary. A suggestion without a location, or one
// whose location falls outside the added lines, is not applicable and returns
// ok=false -- it is advice, not a one-click fix.
//
// It shares one definition with nativeSuggestionComment and
// filterSuggestionsToChangedLines rather than reimplementing the check, so the
// terminal view and the PR view can never disagree about what is eligible. The
// one extra bound -- end-start > 1000 -- mirrors filterSuggestionsToChangedLines
// so a pathological range is rejected before the line walk, never after.
func applicableSuggestionRange(diff *types.Diff, suggestion types.ReviewSuggestion) (start, end int, ok bool) {
	if strings.TrimSpace(suggestion.ProposedCode) == "" || strings.TrimSpace(suggestion.File) == "" {
		return 0, 0, false
	}
	start, end = suggestionRange(suggestion)
	if start <= 0 || end < start || end-start > 1000 {
		return 0, 0, false
	}
	for line := start; ; line++ {
		if !isInlineEligible(diff, types.ReviewIssue{File: suggestion.File, Line: line}) {
			return 0, 0, false
		}
		if line == end {
			break
		}
	}
	return start, end, true
}

// suggestionLocationLabel renders a suggestion's location in the same
// `<file>:<line>` / `<file>:<start>-<end>` shape the PR summary already uses,
// so the terminal and the PR describe a suggestion identically.
func suggestionLocationLabel(suggestion types.ReviewSuggestion, start, end int) string {
	if start == end {
		return fmt.Sprintf("%s:%d", suggestion.File, start)
	}
	return fmt.Sprintf("%s:%d-%d", suggestion.File, start, end)
}

// renderSuggestions renders the review's suggestions for the local --base
// terminal report. Every suggestion the model returned is shown, exactly once,
// with its title, description, location and proposed replacement, so a user who
// never opens the PR still receives the complete suggestion. A suggestion whose
// replacement is eligible for a one-click change is marked as such (and names
// its exact range); one that is not -- no location, a location outside the
// added lines, or no proposed code -- is shown with an explicit limitation and
// never presented as an applicable substitution, matching the PR path's
// fail-closed classification (filterSuggestionsToChangedLines /
// nativeSuggestionComment). Suggestions without a title and without a
// description carry nothing to render and are skipped, exactly as the PR
// summary skips them. The returned string is empty when there is nothing to
// show, so the zero-suggestion report is byte-identical to the published
// behavior.
func renderSuggestions(result *types.ReviewResult, diff *types.Diff, language string) string {
	if result == nil || len(result.Suggestions) == 0 {
		return ""
	}
	copy := reviewCopyFor(language)
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n", copy.suggestions)
	wrote := false
	for _, suggestion := range result.Suggestions {
		title := strings.TrimSpace(suggestion.Title)
		description := strings.TrimSpace(suggestion.Description)
		if title == "" && description == "" {
			continue
		}
		wrote = true
		fmt.Fprintf(&b, "- **%s**", title)
		if description != "" {
			fmt.Fprintf(&b, " — %s", description)
		}
		start, end, applicable := applicableSuggestionRange(diff, suggestion)
		if applicable {
			fmt.Fprintf(&b, " %s", fmt.Sprintf(copy.suggestionApplicable, suggestionLocationLabel(suggestion, start, end)))
		} else {
			fmt.Fprintf(&b, " %s", copy.suggestionNotApplicable)
		}
		b.WriteByte('\n')
		if proposed := strings.TrimSpace(suggestion.ProposedCode); proposed != "" {
			fmt.Fprintf(&b, "  - **%s:**\n\n    ```\n%s\n    ```\n", copy.proposedImplementation, proposed)
		}
		writeSummaryField(&b, copy.rationale, suggestion.Rationale)
		writeSummaryField(&b, copy.verify, suggestion.Verification)
	}
	if !wrote {
		return ""
	}
	return b.String()
}

// renderLocalReport renders the --base path's stdout report from the shared
// render pass: the summary block, then a fenced ```mermaid block when a
// diagram was produced. It is deterministic and derives only from result and
// diff, so the same input always prints the same bytes.
// verdict is canonicalVerdict(result), or the one aligned with the gate.
func renderLocalReport(result *types.ReviewResult, diff *types.Diff, language, verdict string) string {
	tldr, diagram := renderPass(result, diff, language, verdict)
	var b strings.Builder
	if strings.TrimSpace(tldr) != "" {
		b.WriteString(tldr)
		b.WriteString("\n")
	}
	if strings.TrimSpace(diagram) != "" {
		b.WriteString("\n```mermaid\n")
		b.WriteString(diagram)
		b.WriteString("\n```\n")
	}
	return b.String()
}
