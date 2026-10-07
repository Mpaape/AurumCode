// A new review round of the same pull request: findings an earlier round
// already commented on are not commented again, and the earlier findings
// this round no longer reports are named in the review body.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review/rounds"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// roundRepeatedMarker ends the stdout line of a finding an earlier round
// already commented on.
const roundRepeatedMarker = "-- ja comentado em rodada anterior"

// roundPlan is the publication plan of this round, parallel to p.issues.
type roundPlan struct {
	fingerprints []string
	plan         rounds.Plan
}

// roundFingerprint is the finding's identity across rounds: the canonical
// render.FindingFingerprint over the rule, the path and the reviewed code
// at the finding's line, with the line number left out so moving the same
// code keeps the identity. A finding whose line is not in the diff keeps
// its line number, since there is no code to recognize it by.
func roundFingerprint(diff *types.Diff, issue types.ReviewIssue, filter *redaction.Filter) string {
	id := render.FindingIdentityFor(diff, issue, filter)
	if strings.TrimSpace(id.Context) != "" {
		id.Line = 0
	}
	return render.FindingFingerprint(id)
}

// earlierComments maps the conversation onto the comments rounds reads.
// Review bodies are left out: only finding comments carry markers.
func earlierComments(entries []githubclient.ReviewHistoryEntry) []rounds.Comment {
	out := make([]rounds.Comment, 0, len(entries))
	for _, e := range entries {
		if e.Kind == "review" {
			continue
		}
		line := 0
		if e.Line != nil {
			line = *e.Line
		} else if e.OriginalLine != nil {
			line = *e.OriginalLine
		}
		out = append(out, rounds.Comment{Body: e.Body, Path: e.Path, Line: line, Reply: e.InReplyToID != nil})
	}
	return out
}

// planRound computes this round's plan over the sorted issues. Without a
// readable conversation every finding is posted: a repeated comment is
// better than a missing one.
func (p *prReview) planRound() roundPlan {
	fps := make([]string, len(p.issues))
	for i, issue := range p.issues {
		fps[i] = roundFingerprint(p.diff, issue, p.filter)
	}
	var previous []rounds.Previous
	if p.historyErr == nil {
		previous = rounds.Published(earlierComments(p.historyEntries))
	}
	return roundPlan{fingerprints: fps, plan: rounds.PlanRound(fps, previous)}
}

// posts reports whether the i-th sorted issue gets a comment this round.
func (r roundPlan) posts(i int) bool {
	return i >= len(r.plan.Post) || r.plan.Post[i]
}

// findingBody is the finding's comment with its marker for later rounds.
func (r roundPlan) findingBody(i int, issue types.ReviewIssue, language string) string {
	body := formatInlineIssueForLanguage(issue, language)
	if i < len(r.fingerprints) {
		if marker := rounds.Marker(r.fingerprints[i], issue.RuleID); marker != "" {
			body += "\n\n" + marker
		}
	}
	return body
}

// appendRoundNotice adds to the review body what this round did not
// repeat and which earlier findings it no longer reports. Nothing is added
// on a first round.
func appendRoundNotice(body string, r roundPlan, language string) string {
	if r.plan.Repeated == 0 && len(r.plan.Resolved) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	fmt.Fprintf(&b, "\n\n### %s\n\n", i18n.Text(language, "review.round_heading"))
	if r.plan.Repeated > 0 {
		fmt.Fprintf(&b, "- %s\n", i18n.Format(language, "review.round_repeated", r.plan.Repeated))
	}
	if len(r.plan.Resolved) > 0 {
		names := make([]string, 0, len(r.plan.Resolved))
		for _, prev := range r.plan.Resolved {
			names = append(names, fmt.Sprintf("`%s` %s:%d", prev.RuleID, prev.Path, prev.Line))
		}
		fmt.Fprintf(&b, "- %s\n", i18n.Format(language, "review.round_resolved", strings.Join(names, ", ")))
	}
	return b.String()
}
