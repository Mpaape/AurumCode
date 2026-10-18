package main

// #198 on the --pr path: the codebase context of a review in batches comes
// from one reading of the verified checkout, and a file the policy ignores
// never reaches the model as context.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// issue198Repo commits files (slash paths, nested trees) as the only commit
// of a loose-object repository whose origin is owner/repo, writes them to
// the working tree and chdirs into it: a clean checkout of the head.
func issue198Repo(t *testing.T, files map[string]string) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rootTree := issue198Tree(t, dir, "", files)
	commit := "tree " + rootTree + "\nauthor Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nhead\n"
	head = gitObject(t, dir, "commit", []byte(commit))
	for name, body := range map[string]string{
		".git/HEAD":            "ref: refs/heads/main\n",
		".git/refs/heads/main": head + "\n",
		".git/config":          "[core]\n\trepositoryformatversion = 0\n\tbare = false\n[remote \"origin\"]\n\turl = https://github.com/owner/repo.git\n",
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K", "AURUMCODE_MODEL_WINDOW_TOKENS"} {
		t.Setenv(key, "")
	}
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir, head
}

// issue198Tree writes the tree object of the files under prefix, entries in
// git's order (a directory sorts as its name with a trailing slash).
func issue198Tree(t *testing.T, dir, prefix string, files map[string]string) string {
	t.Helper()
	blobs, subdirs := map[string]string{}, map[string]bool{}
	for rel, body := range files {
		if !strings.HasPrefix(rel, prefix) {
			continue
		}
		rest := strings.TrimPrefix(rel, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			subdirs[rest[:i]] = true
			continue
		}
		blobs[rest] = gitObject(t, dir, "blob", []byte(body))
	}
	var names []string
	for name := range blobs {
		names = append(names, name)
	}
	for name := range subdirs {
		names = append(names, name+"/")
	}
	sort.Strings(names)
	var tree bytes.Buffer
	for _, name := range names {
		if strings.HasSuffix(name, "/") {
			sub := strings.TrimSuffix(name, "/")
			tree.Write(treeEntry(t, "40000", sub, issue198Tree(t, dir, path.Join(prefix, sub)+"/", files)))
			continue
		}
		tree.Write(treeEntry(t, "100644", name, blobs[name]))
	}
	return gitObject(t, dir, "tree", tree.Bytes())
}

// issue198Server serves pull request 48 of owner/repo: diff, head SHA and
// the repository config, the same on base and head.
func issue198Server(t *testing.T, head, diff, configYAML string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(diff))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprintf(w, `{"head":{"sha":%q}}`, head)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.HasSuffix(r.URL.Path, "config.yml") && configYAML != "":
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(configYAML)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func issue198Review(t *testing.T, head, diff, configYAML string, deps reviewDeps) (code int, stderr, prompt string) {
	t.Helper()
	capture := aur515Env(t, issue198Server(t, head, diff, configYAML).URL)
	var out, errOut strings.Builder
	code = runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: deps}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, publicationSet: true, publication: "review"})
	data, _ := os.ReadFile(capture)
	return code, out.String() + errOut.String(), string(data)
}

const issue198Symbol = "Issue198ChangedHandler"

// A file the policy's ignore covers is never read for the context sent to
// the model on the --pr path; a file it does not cover, referencing the
// same symbol, is (the control that the context was resolved at all).
func TestIssue198IgnoredFileNeverReachesThePRContext(t *testing.T) {
	use := "package demo\n\nvar _ = " + issue198Symbol + "\n"
	_, head := issue198Repo(t, map[string]string{
		"app.go":                      "package demo\n\nfunc " + issue198Symbol + "() {}\n",
		"kept_reference.go":           use,
		"vendor/ignored_reference.go": use,
		".aurumcode/config.yml":       "ignore:\n  - \"vendor/**\"\n",
	})
	diff := "diff --git a/app.go b/app.go\n@@ -1,2 +1,3 @@\n package demo\n+func " + issue198Symbol + "() {}\n"
	code, out, prompt := issue198Review(t, head, diff, "ignore:\n  - \"vendor/**\"\n", reviewDeps{})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !strings.Contains(prompt, "kept_reference.go") {
		t.Fatalf("the control reference is absent: the context was not resolved, the test proves nothing:\n%.3000s", prompt)
	}
	if strings.Contains(prompt, "ignored_reference.go") {
		t.Fatalf("a file under ignore reached the model as context:\n%.3000s", prompt)
	}
}

// A review the declared window splits in batches resolves each batch's
// context from one reading of the checkout: over every batch's resolution
// no repository file is parsed twice, counted by the grammar each resolver
// the command hands out parses with.
func TestIssue198BatchesShareOneReadingOfTheCheckout(t *testing.T) {
	files := map[string]string{"use/use.go": "package use\n\nvar _ = 1\n"}
	var diff strings.Builder
	for _, dir := range []string{"a", "b", "c"} {
		name := dir + "/" + dir + ".go"
		files[name] = "package " + dir + "\n\nfunc Handler" + strings.ToUpper(dir) + "() {}\n"
		fmt.Fprintf(&diff, "diff --git a/%s b/%s\n@@ -1,2 +1,%d @@\n package %s\n", name, name, issue198Lines+1, dir)
		for i := 0; i < issue198Lines; i++ {
			fmt.Fprintf(&diff, "+var value%s%d = \"alpha beta gamma delta epsilon zeta eta theta\"\n", dir, i)
		}
	}
	_, head := issue198Repo(t, files)
	counter := &issue198CountingGrammar{Provider: grammar.Default(), parsed: map[string]int{}}
	counted := map[*codebasectx.Resolver]bool{}
	calls := 0
	deps := reviewDeps{resolveFiles: func(resolver *codebasectx.Resolver, dir string, changed, scan []string) (*codebasectx.Pack, error) {
		calls++
		if !counted[resolver] {
			counted[resolver] = true
			resolver.WithGrammar(counter)
		}
		return resolver.ResolveWithFiles(dir, changed, scan)
	}}
	t.Setenv("AURUMCODE_MODEL_WINDOW_TOKENS", issue198Window)
	code, out, _ := issue198Review(t, head, diff.String(), "", deps)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if calls < 2 {
		t.Fatalf("the context was resolved %d time(s): the review did not go in batches, the test proves nothing:\n%s", calls, out)
	}
	if counter.parsed["use/use.go"] == 0 {
		t.Fatalf("parsed %v: the repository was not read, the test proves nothing", counter.parsed)
	}
	for name, n := range counter.parsed {
		if n != 1 {
			t.Fatalf("%s parsed %d times over %d resolutions (%d resolvers): each batch read the checkout again", name, n, calls, len(counted))
		}
	}
}

// issue198CountingGrammar is the runtime's grammar, counting each file it
// parses.
type issue198CountingGrammar struct {
	grammar.Provider
	mu     sync.Mutex
	parsed map[string]int
}

func (g *issue198CountingGrammar) Analyze(name string, content []byte) grammar.Structure {
	g.mu.Lock()
	g.parsed[name]++
	g.mu.Unlock()
	return g.Provider.Analyze(name, content)
}

// The diff's size and the declared window that splits it: each file fits a
// prompt, the three together do not.
const (
	issue198Lines  = 600
	issue198Window = "20000"
)
