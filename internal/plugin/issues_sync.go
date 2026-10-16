package plugin

import (
	"context"
	"fmt"
	"sort"
)

// IssueCounts is what a sync reports: numbers only, never a finding's text.
type IssueCounts struct {
	Opened, Kept, Closed, Reopened int
	// Dismissed are closed issues whose finding is still there but that a
	// person closed: the human decision stands, nothing is written on them.
	Dismissed int
	// Undetermined closes reopened because the history did not say who
	// closed them (they are also counted as reopened).
	Undetermined int
}

// issueAction is one planned write.
type issueAction struct {
	kind    string // open, update, reopen, close
	number  int
	finding IssueFinding
}

// planIssues compares the findings of a CONCLUSIVE scan with the plugin's
// issues. Issues whose first line is not this plugin's marker are never
// touched; two issues with the same marker keep the first (lowest number).
// A closed issue whose finding is back is reopened unless dismissed says a
// person closed it (see Dismissed): then it stays closed, counted, with no
// write.
func planIssues(name string, findings []IssueFinding, issues []RemoteIssue, dismissed map[int]bool) ([]issueAction, IssueCounts) {
	byFP := issuesByFingerprint(name, issues)
	var plan []issueAction
	var c IssueCounts
	present := map[string]bool{}
	for _, f := range findings {
		present[f.Fingerprint] = true
		is, ok := byFP[f.Fingerprint]
		switch {
		case !ok:
			plan = append(plan, issueAction{kind: "open", finding: f})
			c.Opened++
		case is.State == "closed" && dismissed[is.Number]:
			c.Dismissed++
		case is.State == "closed":
			plan = append(plan, issueAction{kind: "reopen", number: is.Number, finding: f})
			c.Reopened++
		default:
			if is.Body != IssueBody(name, f) {
				plan = append(plan, issueAction{kind: "update", number: is.Number, finding: f})
			}
			c.Kept++
		}
	}
	var closes []int
	for fp, is := range byFP {
		if !present[fp] && is.State == "open" {
			closes = append(closes, is.Number)
		}
	}
	sort.Ints(closes)
	for _, n := range closes {
		plan = append(plan, issueAction{kind: "close", number: n})
	}
	c.Closed = len(closes)
	return plan, c
}

// issuesByFingerprint maps each fingerprint to its issue.
func issuesByFingerprint(name string, issues []RemoteIssue) map[string]RemoteIssue {
	byFP := map[string]RemoteIssue{}
	for _, is := range issues {
		fp := markerOf(name, is.Body)
		if fp == "" {
			continue
		}
		if prev, ok := byFP[fp]; !ok || is.Number < prev.Number {
			byFP[fp] = is
		}
	}
	return byFP
}

// applyIssues performs the plan. It stops at the first failed write: the
// next run converges from whatever state the repository is in.
func applyIssues(ctx context.Context, api IssuesAPI, name string, plan []issueAction) error {
	label := IssueLabel(name)
	for _, a := range plan {
		title, body := IssueTitle(name, a.finding), IssueBody(name, a.finding)
		var err error
		switch a.kind {
		case "open":
			err = createLabeled(ctx, api, title, body, label)
		case "update":
			err = api.UpdateIssue(ctx, a.number, title, body, "open")
		case "reopen":
			if err = api.UpdateIssue(ctx, a.number, title, body, "open"); err == nil {
				err = api.Comment(ctx, a.number, "O achado voltou numa varredura conclusiva do plugin; a issue foi reaberta pelo AurumCode.")
			}
		case "close":
			if err = api.Comment(ctx, a.number, closeComment(name)); err == nil {
				err = api.CloseIssue(ctx, a.number)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// createLabeled opens the issue and refuses one the API returned without
// the label: it would never be listed again and would be duplicated.
func createLabeled(ctx context.Context, api IssuesAPI, title, body, label string) error {
	is, err := api.CreateIssue(ctx, title, body, label)
	if err != nil {
		return err
	}
	for _, l := range is.Labels {
		if l == label {
			return nil
		}
	}
	return fmt.Errorf("a issue #%d foi criada sem o rótulo %s (o token precisa de issues: write)", is.Number, label)
}
