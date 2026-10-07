package feedback

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// PolicyRepo reads and writes the policy repository through the contents
// API. Every write uses the token the workflow was given; no git identity
// is configured or invented.
type PolicyRepo struct {
	GitHub *GitHub
	Repo   string
}

// DefaultBranch returns the repository's default branch.
func (p PolicyRepo) DefaultBranch() (string, error) {
	var r struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := p.GitHub.getJSON("/repos/"+p.Repo, &r); err != nil {
		return "", err
	}
	if r.DefaultBranch == "" {
		return "", fmt.Errorf("%s sem branch padrão", p.Repo)
	}
	return r.DefaultBranch, nil
}

// File reads path at ref. found is false for a missing file.
func (p PolicyRepo) File(path, ref string) (content string, sha string, found bool, err error) {
	var r struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	err = p.GitHub.getJSON("/repos/"+p.Repo+"/contents/"+escapePath(path)+"?ref="+url.QueryEscape(ref), &r)
	var nf errNotFound
	if errors.As(err, &nf) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if r.Encoding != "base64" {
		return "", "", false, fmt.Errorf("%s: codificação %q inesperada", path, r.Encoding)
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(r.Content, "\n", ""))
	if err != nil {
		return "", "", false, fmt.Errorf("%s: %w", path, err)
	}
	return string(data), r.SHA, true, nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// Pull is one feedback pull request (any state) and its head commit.
type Pull struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Head   struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

// Pulls lists every feedback pull request whose head is Branch, open or
// closed: a closed one (rejected or merged) still says which signals were
// already proposed.
func (p PolicyRepo) Pulls() ([]Pull, error) {
	owner := p.Repo[:strings.Index(p.Repo, "/")]
	return list[Pull](p.GitHub, "/repos/"+p.Repo+"/pulls?state=all&head="+url.QueryEscape(owner+":"+Branch)+"&per_page="+perPage)
}

// ensureBranch makes Branch ready: with an open pull request it is kept;
// without one, a leftover Branch (from a closed pull request) is deleted
// and recreated from base, so a rejected proposal never rides along.
func (p PolicyRepo) ensureBranch(base string, open bool) error {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	err := p.GitHub.getJSON("/repos/"+p.Repo+"/git/ref/heads/"+escapePath(Branch), &ref)
	var nf errNotFound
	switch {
	case err == nil && open:
		return nil
	case err == nil:
		if _, _, err := p.GitHub.do("DELETE", "/repos/"+p.Repo+"/git/refs/heads/"+escapePath(Branch), nil); err != nil {
			return err
		}
	case !errors.As(err, &nf):
		return err
	}
	if err := p.GitHub.getJSON("/repos/"+p.Repo+"/git/ref/heads/"+escapePath(base), &ref); err != nil {
		return err
	}
	_, _, err = p.GitHub.do("POST", "/repos/"+p.Repo+"/git/refs", map[string]string{"ref": "refs/heads/" + Branch, "sha": ref.Object.SHA})
	return err
}

// put writes one file on Branch, skipping it when the content is unchanged.
func (p PolicyRepo) put(path, content, message string) error {
	current, sha, found, err := p.File(path, Branch)
	if err != nil {
		return err
	}
	if found && current == content {
		return nil
	}
	payload := map[string]string{"message": message, "content": base64.StdEncoding.EncodeToString([]byte(content)), "branch": Branch}
	if found {
		payload["sha"] = sha
	}
	_, _, err = p.GitHub.do("PUT", "/repos/"+p.Repo+"/contents/"+escapePath(path), payload)
	return err
}

// Publish writes the plan to Branch and opens the pull request, or updates
// the one already open: a run never opens a second feedback pull request.
func (p PolicyRepo) Publish(plan Plan, base string, openNumber int, open bool) (int, error) {
	if err := p.ensureBranch(base, open); err != nil {
		return 0, err
	}
	for _, path := range plan.Paths() {
		if err := p.put(path, plan.Files[path], "realimentacao: "+path); err != nil {
			return 0, fmt.Errorf("gravando %s: %w", path, err)
		}
	}
	payload := map[string]string{"title": plan.Title, "body": plan.Body}
	if open {
		_, _, err := p.GitHub.do("PATCH", "/repos/"+p.Repo+"/pulls/"+strconv.Itoa(openNumber), payload)
		return openNumber, err
	}
	payload["head"], payload["base"] = Branch, base
	data, _, err := p.GitHub.do("POST", "/repos/"+p.Repo+"/pulls", payload)
	if err != nil {
		return 0, err
	}
	var created struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		return 0, err
	}
	return created.Number, nil
}
