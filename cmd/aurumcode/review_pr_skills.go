package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

// remoteSkillSource reads .aurumcode/skills at the trusted ref through the
// contents API, as loadPullRequestContext does for the other context files:
// a pull request never supplies its own skills.
type remoteSkillSource struct {
	ctx    context.Context
	client *githubclient.Client
	owner  string
	repo   string
	ref    string
}

func (r remoteSkillSource) Dirs() ([]string, error) {
	base := os.Getenv("AURUMCODE_GITHUB_API_URL")
	if base == "" {
		base = githubclient.DefaultBaseURL
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s", base, r.owner, r.repo, skills.DefaultDirName)
	if r.ref != "" {
		endpoint += "?ref=" + url.QueryEscape(r.ref)
	}
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", githubclient.UserAgent)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("skills: listing %s: %w", skills.DefaultDirName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("skills: listing %s: HTTP %d: %s", skills.DefaultDirName, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("skills: listing %s: %w", skills.DefaultDirName, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.Type == "dir" {
			dirs = append(dirs, e.Name)
		}
	}
	return dirs, nil
}

func (r remoteSkillSource) Read(dir string) ([]byte, bool, error) {
	return r.client.GetRepositoryFile(r.ctx, r.owner, r.repo, skills.DefaultDirName+"/"+dir+"/"+skills.DocName, r.ref)
}

func (r remoteSkillSource) Label(dir string) string {
	return skills.DefaultDirName + "/" + dir
}
