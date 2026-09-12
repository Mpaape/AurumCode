package githubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ReviewHistoryEntry is an unsanitized GitHub history item. IDs are scoped by
// Kind. Empty Author means GitHub supplied no login (for example a deleted
// account); null line/reply fields retain the distinction from a current line.
// LEFT denotes the deletion side of a diff. Deleted comments themselves are
// not returned by these APIs, so this is not a deletion audit log.
type ReviewHistoryEntry struct {
	Kind             string     `json:"kind"`
	ID               int64      `json:"id"`
	Body             string     `json:"body"`
	Author           string     `json:"author"`
	State            string     `json:"state,omitempty"`
	CommitID         string     `json:"commit_id"`
	OriginalCommitID string     `json:"original_commit_id"`
	Path             string     `json:"path"`
	Line             *int       `json:"line"`
	OriginalLine     *int       `json:"original_line"`
	Side             string     `json:"side"`
	InReplyToID      *int64     `json:"in_reply_to_id"`
	CreatedAt        *time.Time `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at"`
}

type historySourceEntry struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User *struct {
		Login string `json:"login"`
	} `json:"user"`
	State            string     `json:"state"`
	CommitID         string     `json:"commit_id"`
	OriginalCommitID string     `json:"original_commit_id"`
	Path             string     `json:"path"`
	Line             *int       `json:"line"`
	OriginalLine     *int       `json:"original_line"`
	Side             string     `json:"side"`
	InReplyToID      *int64     `json:"in_reply_to_id"`
	SubmittedAt      *time.Time `json:"submitted_at"`
	CreatedAt        *time.Time `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at"`
}

// GetPullRequestHistory reads every page of reviews, inline review comments,
// and issue comments. A failed source returns nil and an error, never a partial
// success. Bodies are preserved verbatim for the caller's sanitization boundary.
// Review CreatedAt is submitted_at; pending reviews retain a null timestamp
// and sort before dated entries. UpdatedAt is null when not supplied by GitHub.
func (c *Client) GetPullRequestHistory(ctx context.Context, owner, repo string, number int) ([]ReviewHistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if owner == "" || repo == "" || number <= 0 || owner == "." || owner == ".." || repo == "." || repo == ".." || strings.ContainsAny(owner+repo, "/\\") {
		return nil, fmt.Errorf("invalid pull request coordinates")
	}
	root := strings.TrimRight(c.baseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	sources := []struct{ kind, endpoint string }{
		{"review", fmt.Sprintf("%s/pulls/%d/reviews", root, number)},
		{"inline", fmt.Sprintf("%s/pulls/%d/comments", root, number)},
		{"comment", fmt.Sprintf("%s/issues/%d/comments", root, number)},
	}
	// Do not mutate the shared client. Redirects could forward authorization to
	// an unrelated path even on the same host. Return their status as a failure.
	httpClient := *c.httpClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	history := make([]ReviewHistoryEntry, 0)
	for _, source := range sources {
		for page := 1; ; page++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			entries, next, err := c.readHistoryPage(ctx, &httpClient, source.endpoint, page)
			if err != nil {
				return nil, fmt.Errorf("pull request history %s page %d: %w", source.kind, page, err)
			}
			for _, entry := range entries {
				item := ReviewHistoryEntry{
					Kind: source.kind, ID: entry.ID, Body: entry.Body,
					CommitID: entry.CommitID, OriginalCommitID: entry.OriginalCommitID,
					Path: entry.Path, Line: entry.Line, OriginalLine: entry.OriginalLine,
					Side: entry.Side, InReplyToID: entry.InReplyToID,
					CreatedAt: entry.CreatedAt, UpdatedAt: entry.UpdatedAt,
				}
				if entry.User != nil {
					item.Author = entry.User.Login
				}
				if source.kind == "review" {
					item.State = entry.State
					item.CreatedAt = entry.SubmittedAt
				}
				history = append(history, item)
			}
			if !next {
				break
			}
			if page == int(^uint(0)>>1) {
				return nil, fmt.Errorf("pull request history page overflow")
			}
		}
	}
	sort.SliceStable(history, func(i, j int) bool {
		a, b := history[i], history[j]
		if a.CreatedAt == nil && b.CreatedAt != nil {
			return true
		}
		if a.CreatedAt != nil && b.CreatedAt == nil {
			return false
		}
		if a.CreatedAt != nil && !a.CreatedAt.Equal(*b.CreatedAt) {
			return a.CreatedAt.Before(*b.CreatedAt)
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func (c *Client) readHistoryPage(ctx context.Context, client *http.Client, endpoint string, page int) ([]*historySourceEntry, bool, error) {
	// GitHub's REST list endpoints document 100 as the maximum per_page:
	// https://docs.github.com/en/rest/pulls/reviews#list-reviews-for-a-pull-request
	// This is only a page size, not a total item or text limit.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?per_page=100&page="+strconv.Itoa(page), nil)
	if err != nil {
		return nil, false, fmt.Errorf("invalid history endpoint")
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	// Unlike doRequest, this read returns every non-200 status directly, including
	// rate limits. No error body, redirect location, or transport URL is exposed.
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("history request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	var entries []*historySourceEntry
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&entries)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if err != nil || entries == nil {
		return nil, false, fmt.Errorf("invalid history JSON array")
	}
	var trailing json.RawMessage
	err = decoder.Decode(&trailing)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if err != io.EOF {
		return nil, false, fmt.Errorf("invalid trailing history JSON")
	}
	for _, entry := range entries {
		if entry == nil || entry.ID <= 0 {
			return nil, false, fmt.Errorf("invalid history entry")
		}
	}
	return entries, historyHasNext(resp.Header.Values("Link")), nil
}

// Link is only a continuation signal. Never follow its untrusted URL: advance
// the numeric page on the original endpoint locally, even for short pages.
func historyHasNext(headers []string) bool {
	for _, header := range headers {
		for _, link := range strings.Split(header, ",") {
			end := strings.IndexByte(link, '>')
			if !strings.HasPrefix(strings.TrimSpace(link), "<") || end < 0 {
				continue
			}
			for _, parameter := range strings.Split(link[end+1:], ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(key), "rel") {
					continue
				}
				for _, relation := range strings.Fields(strings.Trim(strings.TrimSpace(value), `"`)) {
					if relation == "next" {
						return true
					}
				}
			}
		}
	}
	return false
}
