package feedback

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// OrgRepos lists the non-archived repositories of an organization.
func (g *GitHub) OrgRepos(org string) ([]string, error) {
	type repo struct {
		FullName string `json:"full_name"`
		Archived bool   `json:"archived"`
	}
	items, err := list[repo](g, "/orgs/"+url.PathEscape(org)+"/repos?per_page="+perPage)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range items {
		if !r.Archived && r.FullName != "" {
			out = append(out, r.FullName)
		}
	}
	return out, nil
}

// DismissedAlerts lists the dismissed code scanning alerts of repo. A repo
// without code scanning (404) has none; ok is false so the run can say so.
func (g *GitHub) DismissedAlerts(repo string) (alerts []Alert, ok bool, err error) {
	alerts, err = list[Alert](g, "/repos/"+repo+"/code-scanning/alerts?state=dismissed&per_page="+perPage)
	var nf errNotFound
	if errors.As(err, &nf) {
		return nil, false, nil
	}
	return alerts, err == nil, err
}

// AuditRuns downloads the audit records the review workflow uploaded as
// artifacts aurumcode-audit-<pr>. An expired artifact is skipped; an
// artifact that cannot be read is an error, never a silent gap.
func (g *GitHub) AuditRuns(repo string) ([]AuditRun, error) {
	type artifact struct {
		Name        string `json:"name"`
		Expired     bool   `json:"expired"`
		CreatedAt   string `json:"created_at"`
		DownloadURL string `json:"archive_download_url"`
	}
	type page struct {
		Artifacts []artifact `json:"artifacts"`
	}
	var runs []AuditRun
	next := "/repos/" + repo + "/actions/artifacts?per_page=" + perPage
	seen := 0
	for p := 0; next != "" && p < maxPages && seen < maxArtifacts; p++ {
		data, link, err := g.do("GET", next, nil)
		if err != nil {
			return nil, err
		}
		var pg page
		if err := json.Unmarshal(data, &pg); err != nil {
			return nil, fmt.Errorf("artefatos de %s: %w", repo, err)
		}
		for _, a := range pg.Artifacts {
			pr, ok := auditPR(a.Name)
			if !ok || a.Expired || seen >= maxArtifacts {
				continue
			}
			seen++
			raw, err := g.download(a.DownloadURL, auditRecordFile)
			if err != nil {
				return nil, fmt.Errorf("%s de %s: %w", a.Name, repo, err)
			}
			var run AuditRun
			if err := json.Unmarshal(raw, &run); err != nil {
				return nil, fmt.Errorf("%s de %s: %w", a.Name, repo, err)
			}
			run.PR, run.Created = pr, a.CreatedAt
			runs = append(runs, run)
		}
		next = link
	}
	return runs, nil
}

func auditPR(name string) (int, bool) {
	if !strings.HasPrefix(name, auditArtifact) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(name, auditArtifact))
	return n, err == nil && n > 0
}

// ChangedLines returns the old-side lines from..to rewrote, per path, from
// the compare endpoint. A file without a patch (binary, too large) cannot
// prove a fix and contributes nothing.
func (g *GitHub) ChangedLines(repo, from, to string) (map[string]map[int]bool, error) {
	var cmp struct {
		Files []struct {
			Filename         string `json:"filename"`
			PreviousFilename string `json:"previous_filename"`
			Patch            string `json:"patch"`
		} `json:"files"`
	}
	if err := g.getJSON("/repos/"+repo+"/compare/"+url.PathEscape(from)+"..."+url.PathEscape(to), &cmp); err != nil {
		return nil, err
	}
	out := map[string]map[int]bool{}
	for _, f := range cmp.Files {
		name := f.Filename
		if f.PreviousFilename != "" {
			name = f.PreviousFilename
		}
		out[name] = RemovedOldLines(f.Patch)
	}
	return out, nil
}

// Comments lists the issue and pull request comments of repo since an
// RFC 3339 instant (empty: every comment the bounded pages reach).
func (g *GitHub) Comments(repo, since string) ([]Comment, error) {
	// Newest first: with the bounded pages, a busy repository is read from
	// its recent comments, never only from its oldest ones.
	path := "/repos/" + repo + "/issues/comments?sort=created&direction=desc&per_page=" + perPage
	if since != "" {
		path += "&since=" + url.QueryEscape(since)
	}
	return list[Comment](g, path)
}

// PullHead resolves the head commit of a pull request; an issue is not one.
func (g *GitHub) PullHead(repo string, number int) (string, bool, error) {
	var pr struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	err := g.getJSON("/repos/"+repo+"/pulls/"+strconv.Itoa(number), &pr)
	var nf errNotFound
	if errors.As(err, &nf) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return pr.Head.SHA, true, nil
}
