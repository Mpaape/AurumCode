package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// IssueEvent is one entry of an issue's timeline of events.
type IssueEvent struct {
	Event     string // closed, reopened, ...
	Actor     string
	CreatedAt string // RFC 3339, UTC: compares as text
}

// IssueComment is one comment of an issue.
type IssueComment struct {
	Body, Author, CreatedAt string
}

// IssueHistory is what decides who closed an issue.
type IssueHistory struct {
	Events   []IssueEvent
	Comments []IssueComment
}

// History reads every event and every comment of an issue, page by page to
// the end (no cap), oldest first.
func (g *GitHubIssues) History(ctx context.Context, number int) (IssueHistory, error) {
	var h IssueHistory
	type wire struct {
		Event     string `json:"event"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		Actor     struct {
			Login string `json:"login"`
		} `json:"actor"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	for _, kind := range []string{"events", "comments"} {
		for page := 1; ; page++ {
			q := url.Values{"per_page": {"100"}, "page": {fmt.Sprint(page)}}
			var batch []wire
			if err := g.do(ctx, http.MethodGet, fmt.Sprintf("/issues/%d/%s", number, kind), q, nil, &batch); err != nil {
				return IssueHistory{}, err
			}
			if len(batch) == 0 {
				break
			}
			for _, w := range batch {
				if kind == "events" {
					h.Events = append(h.Events, IssueEvent{Event: w.Event, Actor: w.Actor.Login, CreatedAt: w.CreatedAt})
				} else {
					h.Comments = append(h.Comments, IssueComment{Body: w.Body, Author: w.User.Login, CreatedAt: w.CreatedAt})
				}
			}
		}
	}
	return h, nil
}

// Dismissed decides, from the AUTHOR of the last close, whether a person
// dismissed a closed issue whose finding is still present. The plugin's
// identity is the issue's author (the plugin opened it; nobody can change
// that) or the author of a closed-marker comment written after the last
// reopen. Dismissed only when the last `closed` event's actor is neither:
// a person's close stands. Any doubt (no closed event, no actor) is NOT a
// dismissal: a present finding never disappears without an explicit human
// decision, so the issue is reopened. A forged marker can only make the
// plugin reopen, never hide a finding.
// CloseDecision is who closed an issue whose finding is still present.
type CloseDecision int

const (
	// ClosedByPlugin: the plugin closed it; the finding is back, so it reopens.
	ClosedByPlugin CloseDecision = iota
	// ClosedByPerson: a person closed it; the close stands (dismissed).
	ClosedByPerson
	// CloseUndetermined: the history does not say who closed it (no closed
	// event, no actor, unreadable). The issue reopens and the run reports it:
	// a present finding never disappears without an explicit human decision.
	CloseUndetermined
)

// Dismissed reports whether a person dismissed the issue (CloseOf).
func Dismissed(name, issueAuthor string, h IssueHistory) bool {
	return CloseOf(name, issueAuthor, h) == ClosedByPerson
}

// CloseOf decides who closed the issue, from the actor of its last close.
func CloseOf(name, issueAuthor string, h IssueHistory) CloseDecision {
	var closed *IssueEvent
	lastReopen := ""
	for i := range h.Events {
		switch h.Events[i].Event {
		case "closed":
			closed = &h.Events[i]
		case "reopened":
			lastReopen = h.Events[i].CreatedAt
		}
	}
	if closed == nil || closed.Actor == "" {
		return CloseUndetermined
	}
	if closed.Actor == issueAuthor {
		return ClosedByPlugin
	}
	for _, c := range h.Comments {
		first, _, _ := strings.Cut(c.Body, "\n")
		if strings.TrimRight(first, "\r") == ClosedMarker(name) && c.Author == closed.Actor && c.CreatedAt >= lastReopen {
			return ClosedByPlugin
		}
	}
	return ClosedByPerson
}

// dismissedSet reads the history of each closed issue whose finding is
// back and returns the ones a person dismissed, and how many closes were
// undetermined (unreadable or incomplete history): those reopen, and the
// run reports them, never a silent dismissal.
func dismissedSet(ctx context.Context, api IssuesAPI, name string, findings []IssueFinding, issues []RemoteIssue) (map[int]bool, int) {
	byFP := issuesByFingerprint(name, issues)
	out := map[int]bool{}
	undetermined := 0
	for _, f := range findings {
		is, ok := byFP[f.Fingerprint]
		if !ok || is.State != "closed" {
			continue
		}
		h, err := api.History(ctx, is.Number)
		decision := CloseUndetermined
		if err == nil {
			decision = CloseOf(name, is.Author, h)
		}
		if decision == CloseUndetermined {
			undetermined++
		}
		out[is.Number] = decision == ClosedByPerson
	}
	return out, undetermined
}
