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
func Dismissed(name, issueAuthor string, h IssueHistory) bool {
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
	if closed == nil || closed.Actor == "" || closed.Actor == issueAuthor {
		return false
	}
	for _, c := range h.Comments {
		first, _, _ := strings.Cut(c.Body, "\n")
		if strings.TrimRight(first, "\r") == ClosedMarker(name) && c.Author == closed.Actor && c.CreatedAt >= lastReopen {
			return false
		}
	}
	return true
}

// dismissedSet reads the history of each closed issue whose finding is
// back. An unreadable history is not a dismissal (fail closed): the issue
// is reopened.
func dismissedSet(ctx context.Context, api IssuesAPI, name string, findings []IssueFinding, issues []RemoteIssue) map[int]bool {
	byFP := issuesByFingerprint(name, issues)
	out := map[int]bool{}
	for _, f := range findings {
		is, ok := byFP[f.Fingerprint]
		if !ok || is.State != "closed" {
			continue
		}
		h, err := api.History(ctx, is.Number)
		out[is.Number] = err == nil && Dismissed(name, is.Author, h)
	}
	return out
}
