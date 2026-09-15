package githubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PullRequestMetadata is the untrusted title and body of a pull request, plus
// the head commit the pull request currently proposes. The caller redacts the
// untrusted text before it reaches any prompt, report or published body;
// HeadSHA is a commit identity, not free text, and is used to anchor a review
// or a commit status to the exact revision that was reviewed.
type PullRequestMetadata struct {
	Title   string
	Body    string
	HeadSHA string
}

// PullRequestCommit is one commit message attached to a pull request. SHA and
// Message are untrusted remote text.
type PullRequestCommit struct {
	SHA     string
	Message string
}

// validPullRequestCoordinates rejects coordinates that could escape the
// repo-scoped endpoint. It mirrors the history reader's fail-closed check.
func validPullRequestCoordinates(owner, repo string, number int) bool {
	if owner == "" || repo == "" || number <= 0 {
		return false
	}
	if owner == "." || owner == ".." || repo == "." || repo == ".." {
		return false
	}
	return !strings.ContainsAny(owner+repo, "/\\")
}

// GetPullRequestMetadata reads the pull request's title and body. It is
// read-only and returns the raw remote text for the caller's sanitization.
func (c *Client) GetPullRequestMetadata(ctx context.Context, owner, repo string, number int) (PullRequestMetadata, error) {
	if err := ctx.Err(); err != nil {
		return PullRequestMetadata{}, err
	}
	if !validPullRequestCoordinates(owner, repo, number) {
		return PullRequestMetadata{}, fmt.Errorf("invalid pull request coordinates")
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls/" + strconv.Itoa(number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PullRequestMetadata{}, fmt.Errorf("failed to create pull request request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return PullRequestMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PullRequestMetadata{}, fmt.Errorf("pull request metadata: HTTP status %d", resp.StatusCode)
	}
	var payload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := decoder.Decode(&payload); err != nil {
		return PullRequestMetadata{}, fmt.Errorf("decoding pull request metadata")
	}
	return PullRequestMetadata{Title: payload.Title, Body: payload.Body, HeadSHA: payload.Head.SHA}, nil
}

// GetPullRequestCommits reads every page of the pull request's commit list.
// Each commit's full message is preserved for the caller's redaction.
// A failed page returns nil and an error, never a partial success.
func (c *Client) GetPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]PullRequestCommit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validPullRequestCoordinates(owner, repo, number) {
		return nil, fmt.Errorf("invalid pull request coordinates")
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls/" + strconv.Itoa(number) + "/commits?per_page=100"
	commits := make([]PullRequestCommit, 0)
	for page := 1; endpoint != ""; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create commits request")
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := c.doRequest(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("pull request commits page %d: HTTP status %d", page, resp.StatusCode)
		}
		var entries []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
		}
		decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
		err = decoder.Decode(&entries)
		next := resp.Header.Get("Link")
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding pull request commits page %d", page)
		}
		for _, entry := range entries {
			if strings.TrimSpace(entry.SHA) == "" {
				continue
			}
			commits = append(commits, PullRequestCommit{SHA: entry.SHA, Message: entry.Commit.Message})
		}
		endpoint = c.parseNextLink(next)
	}
	return commits, nil
}
