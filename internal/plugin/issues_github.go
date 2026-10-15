package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
)

// RemoteIssue is an issue of the repository as the sync sees it.
type RemoteIssue struct {
	Number int
	State  string // open or closed
	Body   string
	Labels []string
}

// IssuesAPI is the part of the GitHub REST API the issues destination uses.
// Only the methods that write change the repository.
type IssuesAPI interface {
	Visibility(ctx context.Context) (visibility string, private bool, err error)
	EnsureLabel(ctx context.Context, label string) error
	ListIssues(ctx context.Context, label string) ([]RemoteIssue, error)
	CreateIssue(ctx context.Context, title, body, label string) (RemoteIssue, error)
	UpdateIssue(ctx context.Context, number int, title, body, state string) error
	CloseIssue(ctx context.Context, number int) error
	Comment(ctx context.Context, number int, body string) error
}

// GitHubIssues is IssuesAPI over HTTP. Base is the API root (validated by
// config.GitHubAPIURL), Repo is OWNER/REPO. Errors carry only the status
// code, never a response body: the log may be public.
type GitHubIssues struct {
	Base, Token, Repo string
	HTTP              *http.Client
}

// NewGitHubIssues builds the client with a bounded per-request timeout.
func NewGitHubIssues(base, token, repo string) *GitHubIssues {
	return &GitHubIssues{Base: strings.TrimRight(base, "/"), Token: token, Repo: repo, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// NewGitHubIssuesFromEnv is the production client: the API root from
// AURUMCODE_GITHUB_API_URL (validated; unset is api.github.com) and the
// token from GITHUB_TOKEN, as the pull-request client does.
func NewGitHubIssuesFromEnv(getenv func(string) string, repo string) (*GitHubIssues, error) {
	base, err := config.GitHubAPIURL(getenv)
	if err != nil {
		return nil, err
	}
	return NewGitHubIssues(base, getenv("GITHUB_TOKEN"), repo), nil
}

// apiStatusError is a non-2xx answer: the status only.
type apiStatusError struct {
	Method, Route string
	Status        int
}

func (e *apiStatusError) Error() string {
	return fmt.Sprintf("GitHub respondeu %d em %s %s", e.Status, e.Method, e.Route)
}

func (g *GitHubIssues) do(ctx context.Context, method, route string, query url.Values, in, out any) error {
	u := g.Base + "/repos/" + g.Repo + route
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var data []byte
	if in != nil {
		var err error
		if data, err = json.Marshal(in); err != nil {
			return err
		}
	}
	resp, err := g.sendWithRetry(ctx, method, u, data)
	if err != nil {
		return fmt.Errorf("GitHub inacessível em %s %s", method, route)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		io.Copy(io.Discard, resp.Body)
		return &apiStatusError{Method: method, Route: route, Status: resp.StatusCode}
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("resposta ilegível do GitHub em %s %s", method, route)
	}
	return nil
}

// Visibility reads the repository's visibility and private flag.
func (g *GitHubIssues) Visibility(ctx context.Context) (string, bool, error) {
	var repo struct {
		Visibility string `json:"visibility"`
		Private    bool   `json:"private"`
	}
	if err := g.do(ctx, http.MethodGet, "", nil, nil, &repo); err != nil {
		return "", false, err
	}
	return repo.Visibility, repo.Private, nil
}

// EnsureLabel creates the plugin's label when it does not exist: an issue
// created with a missing label may lose it, and an issue without the label
// is never listed again (it would be duplicated every run).
func (g *GitHubIssues) EnsureLabel(ctx context.Context, label string) error {
	err := g.do(ctx, http.MethodGet, "/labels/"+url.PathEscape(label), nil, nil, nil)
	var st *apiStatusError
	if err == nil || !asStatus(err, &st) || st.Status != http.StatusNotFound {
		return err
	}
	in := map[string]string{"name": label, "color": "b60205", "description": "Achado de plugin agendado do AurumCode"}
	return g.do(ctx, http.MethodPost, "/labels", nil, in, nil)
}

func asStatus(err error, target **apiStatusError) bool {
	e, ok := err.(*apiStatusError)
	if ok {
		*target = e
	}
	return ok
}

type wireIssue struct {
	Number      int             `json:"number"`
	State       string          `json:"state"`
	Body        *string         `json:"body"`
	PullRequest json.RawMessage `json:"pull_request"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (w wireIssue) remote() RemoteIssue {
	r := RemoteIssue{Number: w.Number, State: w.State}
	if w.Body != nil {
		r.Body = *w.Body
	}
	for _, l := range w.Labels {
		r.Labels = append(r.Labels, l.Name)
	}
	return r
}

// ListIssues lists every issue (open and closed) with the label, page by
// page until an empty page (a server that caps the page size below
// per_page is still read to the end): there is no cap on how many issues
// are read. Pull requests the endpoint also returns are skipped.
func (g *GitHubIssues) ListIssues(ctx context.Context, label string) ([]RemoteIssue, error) {
	const perPage = 100
	var out []RemoteIssue
	for page := 1; ; page++ {
		q := url.Values{"labels": {label}, "state": {"all"}, "per_page": {fmt.Sprint(perPage)}, "page": {fmt.Sprint(page)}, "sort": {"created"}, "direction": {"asc"}}
		var batch []wireIssue
		if err := g.do(ctx, http.MethodGet, "/issues", q, nil, &batch); err != nil {
			return nil, err
		}
		for _, w := range batch {
			if len(w.PullRequest) == 0 || string(w.PullRequest) == "null" {
				out = append(out, w.remote())
			}
		}
		if len(batch) == 0 {
			return out, nil
		}
	}
}

// CreateIssue opens an issue with the label.
func (g *GitHubIssues) CreateIssue(ctx context.Context, title, body, label string) (RemoteIssue, error) {
	var w wireIssue
	in := map[string]any{"title": title, "body": body, "labels": []string{label}}
	if err := g.do(ctx, http.MethodPost, "/issues", nil, in, &w); err != nil {
		return RemoteIssue{}, err
	}
	return w.remote(), nil
}

// UpdateIssue changes title, body and state.
func (g *GitHubIssues) UpdateIssue(ctx context.Context, number int, title, body, state string) error {
	in := map[string]string{"title": title, "body": body, "state": state}
	return g.do(ctx, http.MethodPatch, fmt.Sprintf("/issues/%d", number), nil, in, nil)
}

// CloseIssue closes an issue, keeping its title and body.
func (g *GitHubIssues) CloseIssue(ctx context.Context, number int) error {
	return g.do(ctx, http.MethodPatch, fmt.Sprintf("/issues/%d", number), nil, map[string]string{"state": "closed"}, nil)
}

// Comment adds a comment to an issue.
func (g *GitHubIssues) Comment(ctx context.Context, number int, body string) error {
	return g.do(ctx, http.MethodPost, fmt.Sprintf("/issues/%d/comments", number), nil, map[string]string{"body": body}, nil)
}
