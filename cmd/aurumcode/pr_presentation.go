// What a --pr review publishes: the same problem reported by two passes is
// published once, and the repository's review.presentation preferences
// condense non-blocking findings into one explained line. The gate, the
// review event and the commit statuses keep reading the run's own findings.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review/consolidate"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// presentation is the published shape of this run's findings.
type presentation struct {
	consolidate.Result
}

// presentFindings consolidates the sorted issues. A collapse preference
// that does not validate is ignored with a stderr note and every finding is
// published: a configuration error never hides a finding.
func (p *prReview) presentFindings() presentation {
	collapse, err := p.cfg.ReviewPresentationCollapse()
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: %v; publishing every finding\n", err)
		collapse = nil
	}
	rule := p.blockingRule()
	return presentation{consolidate.Apply(p.issues, consolidate.Options{Collapse: collapse, Blocking: rule.Blocks})}
}

// publishedResult is the run's result with the published findings, for the
// review body.
func (pr presentation) publishedResult(result *types.ReviewResult, language string) *types.ReviewResult {
	shown := *result
	shown.Issues = pr.withSources(language)
	return &shown
}

// withSources folds the merged occurrences' sources into each message, so a
// consolidated finding stays traceable to every pass that reported it.
func (pr presentation) withSources(language string) []types.ReviewIssue {
	out := make([]types.ReviewIssue, len(pr.Issues))
	for i, issue := range pr.Issues {
		if i < len(pr.AlsoFrom) && len(pr.AlsoFrom[i]) > 0 {
			sources := sourceList(append([]string{issue.Origin}, pr.AlsoFrom[i]...))
			issue.Message = fmt.Sprintf("%s [%s]", issue.Message, i18n.Format(language, "review.presentation_sources", sources))
		}
		out[i] = issue
	}
	return out
}

// sourceList names each source once; the model has no origin.
func sourceList(origins []string) string {
	seen := map[string]bool{}
	var names []string
	for _, o := range origins {
		name := o
		if name == "" {
			name = "model"
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// appendPresentationNotice explains, in the review body, what was merged
// and what the preference condensed, naming each condensed finding.
func appendPresentationNotice(body string, pr presentation, language string) string {
	if pr.Merged == 0 && len(pr.Collapsed) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	fmt.Fprintf(&b, "\n\n### %s\n\n", i18n.Text(language, "review.presentation_heading"))
	if pr.Merged > 0 {
		fmt.Fprintf(&b, "- %s\n", i18n.Format(language, "review.presentation_merged", pr.Merged))
	}
	if len(pr.Collapsed) > 0 {
		names := make([]string, 0, len(pr.Collapsed))
		for _, issue := range pr.Collapsed {
			names = append(names, fmt.Sprintf("[%s] `%s` %s:%d", issue.Severity, issue.RuleID, issue.File, issue.Line))
		}
		fmt.Fprintf(&b, "- %s\n", i18n.Format(language, "review.presentation_collapsed", len(pr.Collapsed), strings.Join(names, "; ")))
	}
	return b.String()
}
