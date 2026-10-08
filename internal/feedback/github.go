package feedback

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHub is the minimal REST client of the loop. BaseURL is validated by
// the caller (config.GitHubAPIURL); tests point it at a local fake server.
type GitHub struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// Bounds of what one run reads, so a large organization cannot make the
// loop unbounded.
const (
	maxPages        = 10
	maxBodyBytes    = 16 << 20
	maxArtifacts    = 200
	maxAuditBytes   = 4 << 20
	perPage         = "100"
	auditArtifact   = "aurumcode-audit-"
	auditRecordFile = "aurumcode-audit.json"
)

// errNotFound marks a 404, which some endpoints use for "feature disabled".
type errNotFound struct{ path string }

func (e errNotFound) Error() string { return "não encontrado: " + e.path }

func (g *GitHub) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// endpoint resolves a path against BaseURL. An absolute URL (a next-page
// link, an artifact download) must be on the same scheme and host as
// BaseURL, so the token is never sent anywhere else.
func (g *GitHub) endpoint(path string) (string, error) {
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		return strings.TrimRight(g.BaseURL, "/") + path, nil
	}
	base, err := url.Parse(g.BaseURL)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(path)
	if err != nil || u.Scheme != base.Scheme || u.Host != base.Host {
		return "", fmt.Errorf("URL fora da API configurada: %s", safePath(path))
	}
	return path, nil
}

// do sends one request and returns the body and the next-page URL.
func (g *GitHub) do(method, path string, payload any) ([]byte, string, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, "", err
		}
		body = bytes.NewReader(data)
	}
	target, err := g.endpoint(path)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s %s: %w", method, safePath(path), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%s %s: %w", method, safePath(path), err)
	}
	if len(data) > maxBodyBytes {
		return nil, "", fmt.Errorf("%s %s: resposta maior que o limite", method, safePath(path))
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", errNotFound{path: safePath(path)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("%s %s: HTTP %d", method, safePath(path), resp.StatusCode)
	}
	return data, nextLink(resp.Header.Get("Link")), nil
}

// safePath drops the query of a path before it reaches an error message.
func safePath(p string) string {
	if u, err := url.Parse(p); err == nil {
		return u.Path
	}
	return "?"
}

func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		segs := strings.Split(part, ";")
		if len(segs) < 2 || !strings.Contains(segs[1], `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(segs[0]), "<>")
	}
	return ""
}

// list reads every page of a JSON array endpoint into out (bounded).
func list[T any](g *GitHub, path string) ([]T, error) {
	var all []T
	next := path
	for page := 0; next != "" && page < maxPages; page++ {
		data, link, err := g.do(http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("GET %s: %w", safePath(next), err)
		}
		all = append(all, items...)
		next = link
	}
	return all, nil
}

func (g *GitHub) getJSON(path string, out any) error {
	data, _, err := g.do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("GET %s: %w", safePath(path), err)
	}
	return nil
}

// download reads an artifact zip and returns the named file inside it.
func (g *GitHub) download(rawURL, name string) ([]byte, error) {
	data, _, err := g.do(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("artefato não é zip: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxAuditBytes+1))
		rc.Close()
		if err != nil {
			return nil, err
		}
		if len(content) > maxAuditBytes {
			return nil, fmt.Errorf("%s maior que o limite", name)
		}
		return content, nil
	}
	return nil, fmt.Errorf("artefato sem %s", name)
}
