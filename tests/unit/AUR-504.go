package unit

// Unit program for card AUR-504, selector TestAUR504.
//
// Proves the `aurumcode review --pr --check` commit status anchors on the pull
// request HEAD returned by the API, never on GITHUB_SHA -- which GitHub Actions
// reserves to the synthetic merge commit on a pull_request event. A branch
// protection rule that requires the `aurumcode/review` context would otherwise
// never see the status on the head and leave the PR "expected".
//
//   - AC-001: with --pr --check and GITHUB_SHA pointing at the merge commit,
//     the status is published on /statuses/{head} where {head} is the SHA
//     returned by GetPullRequestMetadata.
//   - AC-002: the local --base path still publishes nothing: it never contacts
//     the GitHub API at all and keeps its pre-existing output.
//   - AC-003: when the head cannot be determined (API error, or a response
//     with no head SHA) the command fails closed naming the reason and
//     publishes no status -- it never falls back to GITHUB_SHA.
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-504.sh), the same pattern the other cards in this wave
// use, so the file itself is a plain package file. It builds and runs the real
// cmd/aurumcode binary against a loopback httptest fake GitHub, so every
// assertion executes the real entrypoint rather than a metadata read.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func aur504Root(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

// aur504BuildBinary builds the real cmd/aurumcode binary once per test run.
func aur504BuildBinary(t *testing.T) string {
	t.Helper()
	root := aur504Root(t)
	bin := filepath.Join(t.TempDir(), "aurumcode-aur504")
	build := exec.Command("go", "build", "-o", bin, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return bin
}

// aur504ScrubEnv drops every variable that could accidentally configure a
// provider, a GitHub endpoint or an anchor SHA, so each subtest states its own
// configuration explicitly.
func aur504ScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":        true,
		"LLM_API_KEY":                  true,
		"LLM_BASE_URL":                 true,
		"LLM_MODEL":                    true,
		"AURUMCODE_GITHUB_API_URL":     true,
		"GITHUB_TOKEN":                 true,
		"GITHUB_SHA":                   true,
		"AURUMCODE_BASE_SHA":           true,
		"AURUMCODE_PR_PERMISSION_MODE": true,
		"AURUMCODE_OUTPUT_FILE":        true,
	}
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !drop[name] {
			env = append(env, kv)
		}
	}
	return env
}

func aur504Run(t *testing.T, bin, dir string, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(aur504ScrubEnv(), extraEnv...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// aur504Fixture writes a one-issue-free review response and returns its path.
func aur504Fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aur504-response.json")
	body := `{"issues":[],"summary":"Nenhum problema encontrado."}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing review fixture: %v", err)
	}
	return path
}

const aur504Diff = "diff --git a/main.go b/main.go\n@@ -1,1 +1,3 @@\n package main\n+// add returns the sum of two integers.\n+func add(a, b int) int { return a + b }\n"

// aur504FakeGitHub serves the minimum surface the --pr --check path needs and
// records every status POST by target SHA.
type aur504StatusPost struct {
	path string
	body map[string]interface{}
}

type aur504FakeGitHub struct {
	mu          sync.Mutex
	headSHA     string
	metadataErr bool
	requests    int
	statusPosts []aur504StatusPost
}

func (f *aur504FakeGitHub) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests++
		f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42":
			if strings.Contains(r.Header.Get("Accept"), "diff") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(aur504Diff))
				return
			}
			if f.metadataErr {
				// A non-2xx from the metadata endpoint: a real API
				// failure the caller must fail closed on (404 avoids the
				// client's 5xx retry backoff, keeping the sealed
				// acceptance fast without weakening the assertion).
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"title":"t","body":"b","head":{"sha":%q}}`, f.headSHA)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") ||
			strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/repos/owner/repo/statuses/"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.statusPosts = append(f.statusPosts, aur504StatusPost{path: r.URL.Path, body: body})
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Logf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (f *aur504FakeGitHub) snapshot() (int, []aur504StatusPost) {
	f.mu.Lock()
	defer f.mu.Unlock()
	posts := append([]aur504StatusPost(nil), f.statusPosts...)
	return f.requests, posts
}

func TestAUR504(t *testing.T) {
	bin := aur504BuildBinary(t)
	workDir := t.TempDir() // --pr mode reads nothing from the local git repo

	prCheckEnv := func(serverURL, fixture, sha string) []string {
		return []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + serverURL,
			"AURUMCODE_PR_PERMISSION_MODE=endpoint",
			"GITHUB_TOKEN=synthetic-aur504-token",
			"GITHUB_SHA=" + sha,
		}
	}

	t.Run("AC-001 status anchors on the API head not GITHUB_SHA", func(t *testing.T) {
		fake := &aur504FakeGitHub{headSHA: "head-504-deadbeef"}
		server := httptest.NewServer(fake.handler(t))
		defer server.Close()

		const mergeSHA = "merge-504-cafebabe"
		code, stdout, stderr := aur504Run(t, bin, workDir,
			prCheckEnv(server.URL, aur504Fixture(t), mergeSHA),
			"review", "--pr", "42", "--repo", "owner/repo", "--publicar", "--check")
		if code != 0 {
			t.Fatalf("runPRReview exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		_, posts := fake.snapshot()
		if len(posts) != 1 {
			t.Fatalf("expected exactly 1 status POST, got %d: %+v", len(posts), posts)
		}
		wantPath := "/repos/owner/repo/statuses/head-504-deadbeef"
		if posts[0].path != wantPath {
			t.Fatalf("status published on %q, want %q (the API head, never GITHUB_SHA=%s)", posts[0].path, wantPath, mergeSHA)
		}
		if !strings.Contains(stdout, "head-504-deadbeef") {
			t.Fatalf("stdout did not name the API head SHA:\n%s", stdout)
		}
		if posts[0].body["state"] != "success" {
			t.Fatalf("zero-finding status state = %v, want success", posts[0].body["state"])
		}
	})

	t.Run("AC-002 --base publishes nothing and keeps its output", func(t *testing.T) {
		root := aur504Root(t)
		repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
		if _, err := os.Stat(repoDir); err != nil {
			t.Fatalf("required --base fixture missing: %v", err)
		}
		fake := &aur504FakeGitHub{headSHA: "never-used"}
		server := httptest.NewServer(fake.handler(t))
		defer server.Close()

		fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + server.URL,
			"AURUMCODE_PR_PERMISSION_MODE=endpoint",
			"GITHUB_SHA=merge-504-ignored",
		}
		code, stdout, stderr := aur504Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("--base exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("--base lost its pre-existing finding:\n%s", stdout)
		}
		requests, posts := fake.snapshot()
		if requests != 0 || len(posts) != 0 {
			t.Fatalf("--base contacted GitHub: %d request(s), %d status post(s)", requests, len(posts))
		}
	})

	t.Run("AC-003 API error fails closed with no status", func(t *testing.T) {
		fake := &aur504FakeGitHub{headSHA: "head-504-irrelevant", metadataErr: true}
		server := httptest.NewServer(fake.handler(t))
		defer server.Close()

		code, stdout, stderr := aur504Run(t, bin, workDir,
			prCheckEnv(server.URL, aur504Fixture(t), "merge-504-cafebabe"),
			"review", "--pr", "42", "--repo", "owner/repo", "--publicar", "--check")
		if code == 0 {
			t.Fatalf("API error must fail closed, got exit 0\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		_, posts := fake.snapshot()
		if len(posts) != 0 {
			t.Fatalf("API error published %d status(es): %+v", len(posts), posts)
		}
		if !strings.Contains(stderr, "head commit") {
			t.Fatalf("stderr did not name the head-resolution reason:\n%s", stderr)
		}
	})

	t.Run("AC-003 missing head SHA fails closed with no status", func(t *testing.T) {
		fake := &aur504FakeGitHub{headSHA: ""}
		server := httptest.NewServer(fake.handler(t))
		defer server.Close()

		code, stdout, stderr := aur504Run(t, bin, workDir,
			prCheckEnv(server.URL, aur504Fixture(t), "merge-504-cafebabe"),
			"review", "--pr", "42", "--repo", "owner/repo", "--publicar", "--check")
		if code == 0 {
			t.Fatalf("a response with no head SHA must fail closed, got exit 0\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		_, posts := fake.snapshot()
		if len(posts) != 0 {
			t.Fatalf("missing head SHA published %d status(es): %+v", len(posts), posts)
		}
		if !strings.Contains(stderr, "head SHA") {
			t.Fatalf("stderr did not name the missing head SHA:\n%s", stderr)
		}
	})
}
