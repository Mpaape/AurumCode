// What a --pr review publishes: the same problem reported by two passes is
// published once, and the repository's review.presentation preferences
// condense non-blocking findings into one explained line. The gate, the
// review event and the commit statuses keep reading the run's own findings.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
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
	if !collapseAllowed(rule, p.gateRes != nil && p.gateRes.Inconclusive) {
		collapse = nil
	}
	return presentation{consolidate.Apply(p.withoutDismissed(p.issues), consolidate.Options{Collapse: collapse, Blocking: rule.Blocks})}
}

// withoutDismissed is issues without the deterministic findings the
// model's triage demoted: a refuted alarm leaves the parecer (no
// observation line); the SARIF and the audit record keep it.
func (s *reviewState) withoutDismissed(issues []types.ReviewIssue) []types.ReviewIssue {
	if len(s.triageDemotions) == 0 {
		return issues
	}
	dismissed := make(map[string]bool, len(s.triageDemotions))
	for _, d := range s.triageDemotions {
		dismissed[findingOriginKey(d.RuleID, d.File, d.Line)] = true
	}
	out := make([]types.ReviewIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Origin != "" && dismissed[findingOriginKey(issue.RuleID, issue.File, issue.Line)] {
			continue
		}
		out = append(out, issue)
	}
	return out
}

// collapseAllowed reports whether the run's blocking rule can tell what
// blocks. A declared gate that fails without naming a blocking finding (an
// inconclusive run under gate.inconclusive: block) or any inconclusive run
// leaves every finding possibly blocking: nothing is condensed.
func collapseAllowed(rule blocking.Rule, inconclusive bool) bool {
	if inconclusive {
		return false
	}
	return !(rule.Gated() && rule.Fails() && rule.Count(nil) == 0)
}

// publishedResult is the run's result with the published findings, for the
// review body. The occurrences merged into another that block under rule
// are recorded, so the headline counts each problem once.
func (pr presentation) publishedResult(result *types.ReviewResult, language string, rule blocking.Rule) *types.ReviewResult {
	shown := *result
	shown.Issues = pr.withSources(language)
	shown.Metadata = make(map[string]string, len(result.Metadata)+1)
	for k, v := range result.Metadata {
		shown.Metadata[k] = v
	}
	absorbed := 0
	for _, issue := range pr.Absorbed {
		if rule.Blocks(issue) {
			absorbed++
		}
	}
	if absorbed > 0 {
		shown.Metadata[metaAbsorbedBlocking] = strconv.Itoa(absorbed)
	}
	return &shown
}

// metaAbsorbedBlocking is the metadata key with how many blocking
// occurrences were published inside another one (the same problem from a
// second source).
const metaAbsorbedBlocking = "published_absorbed_blocking"

// withSources writes each published finding as a person reads it: its
// short title and every pass that reported it (publishedMessage), so a
// consolidated finding stays traceable to each source.
func (pr presentation) withSources(language string) []types.ReviewIssue {
	out := make([]types.ReviewIssue, len(pr.Issues))
	for i, issue := range pr.Issues {
		var also []string
		if i < len(pr.AlsoFrom) {
			also = pr.AlsoFrom[i]
		}
		issue.Message = publishedMessage(issue, also, language)
		out[i] = issue
	}
	return out
}

// sourceList names each source once, in a sentence of the review's
// language; the model has no origin.
func sourceList(origins []string, language string) string {
	seen := map[string]bool{}
	var names []string
	for _, o := range origins {
		name := sourceName(o, language)
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return joinNames(names, language)
}

// presentationNotice is the details note of what was merged and what the
// preference condensed, naming each condensed finding.
func presentationNotice(pr presentation, language string) string {
	if pr.Merged == 0 && len(pr.Collapsed) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#### %s\n\n", i18n.Text(language, "review.presentation_heading"))
	if pr.Merged > 0 {
		fmt.Fprintf(&b, "- %s\n", countText(pr.Merged, i18n.Text(language, "review.presentation_merged_one"), i18n.Text(language, "review.presentation_merged")))
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
