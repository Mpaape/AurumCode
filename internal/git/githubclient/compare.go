package githubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Comparison is what changed between two commits: GitHub's status of head
// relative to base ("ahead", "identical", "behind", "diverged") and the
// changed files with their unified patches (empty for a binary or too
// large file).
type Comparison struct {
	Status string
	Files  []ComparedFile
}

// ComparedFile is one file of a comparison.
type ComparedFile struct {
	Filename string
	Status   string
	Patch    string
}

// CompareCommits reads base...head. Any failure (unknown commit, HTTP
// error, undecodable body) is an error, never a partial comparison.
func (c *Client) CompareCommits(ctx context.Context, owner, repo, base, head string) (Comparison, error) {
	if err := ctx.Err(); err != nil {
		return Comparison{}, err
	}
	if !validPullRequestCoordinates(owner, repo, 1) || !validCommitRef(base) || !validCommitRef(head) {
		return Comparison{}, fmt.Errorf("invalid comparison coordinates")
	}
	endpoint := strings.TrimRight(c.baseURL, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
		"/compare/" + url.PathEscape(base) + "..." + url.PathEscape(head)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Comparison{}, fmt.Errorf("failed to create comparison request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return Comparison{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Comparison{}, fmt.Errorf("comparing commits: HTTP status %d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Files  []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
			Patch    string `json:"patch"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&payload); err != nil {
		return Comparison{}, fmt.Errorf("decoding commit comparison")
	}
	out := Comparison{Status: payload.Status, Files: make([]ComparedFile, 0, len(payload.Files))}
	for _, f := range payload.Files {
		out.Files = append(out.Files, ComparedFile{Filename: f.Filename, Status: f.Status, Patch: f.Patch})
	}
	return out, nil
}

// validCommitRef accepts a hex commit SHA only.
func validCommitRef(ref string) bool {
	if len(ref) < 7 || len(ref) > 64 {
		return false
	}
	for _, r := range ref {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
