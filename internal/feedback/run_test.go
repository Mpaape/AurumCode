package feedback

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub is a local GitHub with one application repository and one
// policy repository whose files live per branch.
type fakeGitHub struct {
	mu       sync.Mutex
	alerts   []Alert
	files    map[string]map[string]string
	open     []int
	created  int
	patched  int
	puts     int
	requests []string
}

func newFakeGitHub() *fakeGitHub {
	return &fakeGitHub{files: map[string]map[string]string{"main": {
		".aurumcode/config.yml": "review:\n  context:\n    skills:\n      - skills/seguranca.md\n",
		"skills/seguranca.md":   "# Segurança\n\n## sql\n\nNão concatene SQL.\n",
	}}}
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	p := r.URL.Path
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case p == "/repos/o/app/code-scanning/alerts":
		write(f.alerts)
	case p == "/repos/o/app/actions/artifacts":
		write(map[string]any{"artifacts": []any{}})
	case p == "/repos/o/app/issues/comments":
		write([]any{})
	case p == "/repos/o/politica":
		write(map[string]string{"default_branch": "main"})
	case p == "/repos/o/politica/pulls" && r.Method == http.MethodGet:
		var out []map[string]int
		for _, n := range f.open {
			out = append(out, map[string]int{"number": n})
		}
		write(out)
	case p == "/repos/o/politica/pulls" && r.Method == http.MethodPost:
		f.created++
		f.open = append(f.open, 100+f.created)
		write(map[string]int{"number": 100 + f.created})
	case strings.HasPrefix(p, "/repos/o/politica/pulls/") && r.Method == http.MethodPatch:
		f.patched++
		write(map[string]int{})
	case strings.HasPrefix(p, "/repos/o/politica/git/ref/heads/"):
		branch := strings.TrimPrefix(p, "/repos/o/politica/git/ref/heads/")
		if _, ok := f.files[branch]; !ok {
			http.NotFound(w, r)
			return
		}
		write(map[string]any{"object": map[string]string{"sha": "sha-" + branch}})
	case p == "/repos/o/politica/git/refs" && r.Method == http.MethodPost:
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		branch := strings.TrimPrefix(body["ref"], "refs/heads/")
		copied := map[string]string{}
		for k, v := range f.files["main"] {
			copied[k] = v
		}
		f.files[branch] = copied
		write(map[string]string{})
	case strings.HasPrefix(p, "/repos/o/politica/contents/"):
		f.contents(w, r, strings.TrimPrefix(p, "/repos/o/politica/contents/"))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGitHub) contents(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method == http.MethodPut {
		var body map[string]string
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		content, _ := base64.StdEncoding.DecodeString(body["content"])
		f.files[body["branch"]][path] = string(content)
		f.puts++
		_ = json.NewEncoder(w).Encode(map[string]string{})
		return
	}
	content, ok := f.files[r.URL.Query().Get("ref")][path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"content": base64.StdEncoding.EncodeToString([]byte(content)), "encoding": "base64", "sha": "blob"})
}

func runOnce(t *testing.T, srv *httptest.Server, model *fakeModel) Result {
	t.Helper()
	res, err := Run(Options{
		GitHub: &GitHub{BaseURL: srv.URL, Token: "t"}, Repos: []string{"o/app"}, Policy: "o/politica",
		Provider: model, Publish: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// AC-004 and AC-006 end to end against a local GitHub: the first run opens
// one pull request with the cited proposal; a rerun without new signals
// opens nothing and writes nothing; a new signal updates the same PR.
func TestAUR532AC004AC006SinglePullRequestAndIdempotentRerun(t *testing.T) {
	gh := newFakeGitHub()
	gh.alerts = []Alert{alert(1, "dismissed", "false positive", "seguranca#sql")}
	srv := httptest.NewServer(gh)
	defer srv.Close()
	id := FalsePositives(nil, "o/app", gh.alerts)[0].ID
	model := &fakeModel{text: `{"propostas":[{"titulo":"Query builder","skill":"skills/seguranca.md","secao":"sql","texto":"Parâmetros vinculados são seguros.","sinais":["` + id + `"]}]}`}

	first := runOnce(t, srv, model)
	if first.PullRequest != 101 || gh.created != 1 {
		t.Fatalf("first run: result %+v, created %d", first, gh.created)
	}
	branch := gh.files[Branch]
	if !strings.Contains(branch["skills/seguranca.md"], "Parâmetros vinculados") || !strings.Contains(branch[LedgerPath], id) {
		t.Fatalf("branch files: %v", branch)
	}
	if gh.files["main"]["skills/seguranca.md"] != "# Segurança\n\n## sql\n\nNão concatene SQL.\n" {
		t.Fatal("the policy's default branch was changed without review")
	}

	puts := gh.puts
	second := runOnce(t, srv, model)
	if second.Fresh != 0 || second.Planned || gh.created != 1 || gh.patched != 0 || gh.puts != puts {
		t.Fatalf("rerun without new signals: result %+v created %d patched %d puts %d->%d", second, gh.created, gh.patched, puts, gh.puts)
	}

	gh.alerts = append(gh.alerts, alert(2, "dismissed", "false positive", "seguranca#sql"))
	third := runOnce(t, srv, &fakeModel{text: `{"propostas":[]}`})
	if third.Fresh != 1 || gh.created != 1 || gh.patched != 1 || third.PullRequest != 101 {
		t.Fatalf("new signal with an open PR: result %+v created %d patched %d", third, gh.created, gh.patched)
	}
}
