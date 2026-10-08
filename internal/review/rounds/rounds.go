// Package rounds lets a new review round of the same pull request recognize
// what an earlier round already commented on. Every finding comment carries
// a hidden marker with the finding's location-independent fingerprint; the
// next round reads the markers back from the conversation, posts only the
// findings no earlier comment covers, and names the earlier findings this
// round no longer reports.
//
// A marker is never authority over a finding: it decides only whether one
// more comment is posted. The finding itself still counts in the review
// body, the gate and the commit statuses, so a forged or stale marker can at
// most suppress a repeated comment, never a rule.
package rounds

import (
	"regexp"
	"strings"
)

// markerPattern is the one shape a marker may have: a sha256 hex
// fingerprint and a rule id of the catalog's characters.
var markerPattern = regexp.MustCompile(`<!-- aurumcode:finding ([0-9a-f]{64}) ([A-Za-z0-9_./#:-]+) -->`)

// rulePattern is the rule id a marker can carry.
var rulePattern = regexp.MustCompile(`^[A-Za-z0-9_./#:-]+$`)

// fingerprintPattern validates a fingerprint before it is written.
var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Marker is the hidden line appended to a finding comment. It is empty when
// the fingerprint or the rule id cannot be written safely, so a malformed
// value never produces a marker the next round would misread.
func Marker(fingerprint, ruleID string) string {
	ruleID = strings.TrimSpace(ruleID)
	if !fingerprintPattern.MatchString(fingerprint) || !rulePattern.MatchString(ruleID) {
		return ""
	}
	return "<!-- aurumcode:finding " + fingerprint + " " + ruleID + " -->"
}

// ResolvedMarker is the hidden line a later round adds to a finding
// comment it no longer reports. A comment carrying it was already told
// resolved: its finding markers are not read again, so a resolved finding
// is named once, not in every round after it.
const ResolvedMarker = "<!-- aurumcode:resolved -->"

// Comment is one earlier comment of the conversation, as the caller read it.
// The caller passes only comments authored by the identity this product
// publishes as: a marker from anyone else is never read.
type Comment struct {
	// ID and Kind identify the comment to the host ("inline" for a review
	// line comment, "comment" for a general one), so a later round can edit
	// it.
	ID   int64
	Kind string
	Body string
	Path string
	Line int
	// Reply marks an answer inside a thread: a quoted marker there is not a
	// finding comment of an earlier round.
	Reply bool
}

// Previous is one finding an earlier round commented on, with the comment
// that carries it.
type Previous struct {
	Fingerprint string
	RuleID      string
	Path        string
	Line        int
	CommentID   int64
	CommentKind string
	CommentBody string
}

// Published lists the findings earlier rounds commented on, one per marker
// found in a comment that is not a reply and was not already told
// resolved, in conversation order.
func Published(comments []Comment) []Previous {
	var out []Previous
	for _, c := range comments {
		if c.Reply || strings.Contains(c.Body, ResolvedMarker) {
			continue
		}
		for _, m := range markerPattern.FindAllStringSubmatch(c.Body, -1) {
			out = append(out, Previous{Fingerprint: m[1], RuleID: m[2], Path: c.Path, Line: c.Line, CommentID: c.ID, CommentKind: c.Kind, CommentBody: c.Body})
		}
	}
	return out
}

// Resolved is the edited body of a finding comment a later round no longer
// reports: the note first, the original body, then the resolved marker.
func Resolved(note, body string) string {
	return note + "\n\n" + strings.TrimSpace(body) + "\n\n" + ResolvedMarker
}

// Plan is what a round publishes given what earlier rounds published.
type Plan struct {
	// Post is parallel to the round's findings: true when no earlier
	// comment covers that finding.
	Post []bool
	// Repeated counts the findings an earlier comment already covers.
	Repeated int
	// Resolved lists, once per fingerprint, the earlier findings this round
	// no longer reports.
	Resolved []Previous
}

// PlanRound matches the round's fingerprints (one per published finding,
// in publication order) against the earlier ones. Matching is by count: two
// findings with the same fingerprint need two earlier comments, so a second
// occurrence of the same defect is still posted. An empty fingerprint is
// always posted. stillReported are findings this round reports without a
// comment of their own (condensed by a preference): never posted, never
// resolved.
func PlanRound(current, stillReported []string, previous []Previous) Plan {
	available := map[string]int{}
	for _, p := range previous {
		available[p.Fingerprint]++
	}
	plan := Plan{Post: make([]bool, len(current))}
	reported := map[string]bool{}
	for _, fp := range stillReported {
		reported[fp] = true
	}
	for i, fp := range current {
		reported[fp] = true
		if fp != "" && available[fp] > 0 {
			available[fp]--
			plan.Repeated++
			continue
		}
		plan.Post[i] = true
	}
	listed := map[string]bool{}
	for _, p := range previous {
		if reported[p.Fingerprint] || listed[p.Fingerprint] {
			continue
		}
		listed[p.Fingerprint] = true
		plan.Resolved = append(plan.Resolved, p)
	}
	return plan
}
