package integration

// Integration selector for card AUR-439: the deeper behavioral proof behind
// `aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar
// --check` against a real, loopback-only httptest fake GitHub built from
// AUR-438's fixtures (tests/fixtures/scm/github) -- grave-vs-clean-vs-
// zero-issue status content, the exit code that must never read as
// success while a grave finding is present (MUT-001), the commit-SHA and
// write-permission fail-closed inheritance from AUR-437/AUR-438, and
// determinism across repeated runs. This intentionally overlaps
// tests/e2e/AUR-439.sh's proof (a different layer, Go assertions on parsed
// JSON bodies instead of shell greps on a log file) the same way the rest
// of this wave's cards pair a Go integration test with a bash e2e script;
// see docs/specs/AUR-439.md.
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-439.sh), the same pattern the other cards in this
// wave use, so the file itself is a plain package file.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func aur439IntegrationRoot(t *testing.T) string {
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

func aur439IntegrationFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur439IntegrationRoot(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

func aur439IntegrationScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":    true,
		"LLM_API_KEY":              true,
		"LLM_BASE_URL":             true,
		"LLM_MODEL":                true,
		"AURUMCODE_GITHUB_API_URL": true,
		"GITHUB_TOKEN":             true,
		"GITHUB_SHA":               true,
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

// aur439Request is one recorded HTTP request the fake GitHub server saw.
type aur439Request struct {
	Method string
	Path   string
	Body   map[string]interface{} // nil for a GET
}

// aur439FakeGitHub is a loopback-only fake GitHub server built from
// AUR-438's fixtures, extended with the statuses endpoint AUR-439 needs.
// It is deliberately not a mock that "always agrees": a POST to the inline
// review-comment endpoint with an empty commit_id is answered 422 (the
// AUR-438 rule, inherited unchanged), and a POST to
// /repos/{owner}/{repo}/statuses/{sha} with a missing or invalid "state"
// is answered 422 too, exactly like the real GitHub API would reject it.
type aur439FakeGitHub struct {
	*httptest.Server
	mu       sync.Mutex
	requests []aur439Request
}

func aur439NewFakeGitHub(t *testing.T, scenario string) *aur439FakeGitHub {
	t.Helper()
	diffBody := aur439IntegrationFixture(t, "pr-42.diff")
	created := aur439IntegrationFixture(t, "comment-created.json")
	var repoJSON []byte
	if scenario == "write" {
		repoJSON = aur439IntegrationFixture(t, "repo-read-write.json")
	} else {
		repoJSON = aur439IntegrationFixture(t, "repo-read-only.json")
	}

	validStates := map[string]bool{"pending": true, "success": true, "error": true, "failure": true}

	f := &aur439FakeGitHub{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, aur439Request{Method: r.Method, Path: r.URL.Path, Body: body})
		f.mu.Unlock()

		if r.Method == http.MethodPost {
			if strings.HasPrefix(r.URL.Path, "/repos/dono/projeto/statuses/") {
				state, _ := body["state"].(string)
				if !validStates[state] {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"state","code":"invalid"}]}`))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":439200,"state":"` + state + `"}`))
				return
			}
			if r.URL.Path == "/repos/dono/projeto/pulls/42/comments" {
				commitID, _ := body["commit_id"].(string)
				if commitID == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"commit_id","code":"missing_field"}]}`))
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(created)
			return
		}
		switch r.URL.Path {
		case "/repos/dono/projeto":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(repoJSON)
		case "/repos/dono/projeto/pulls/42":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(diffBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return f
}

func (f *aur439FakeGitHub) posts() []aur439Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []aur439Request
	for _, r := range f.requests {
		if r.Method == http.MethodPost {
			out = append(out, r)
		}
	}
	return out
}

func (f *aur439FakeGitHub) statusPosts() []aur439Request {
	var out []aur439Request
	for _, r := range f.posts() {
		if strings.HasPrefix(r.Path, "/repos/dono/projeto/statuses/") {
			out = append(out, r)
		}
	}
	return out
}

func aur439ResponseFixture(t *testing.T, issuesJSON string) string {
	t.Helper()
	content := `{"issues": ` + issuesJSON + `, "summary": "Resposta sintetica de integracao para AUR-439."}`
	path := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

const (
	aur439GraveIssues = `[
    {"file": "cmdb/settings.go", "line": 3, "severity": "error", "rule_id": "quality/long-function", "message": "Achado grave sintetico."},
    {"file": "docs/notas.md", "line": 99, "severity": "info", "rule_id": "quality/long-function", "message": "Achado informativo fora das linhas alteradas."}
  ]`
	aur439CleanIssues       = `[{"file": "cmdb/settings.go", "line": 3, "severity": "warning", "rule_id": "quality/long-function", "message": "Achado nao grave sintetico."}]`
	aur439GeneralOnlyIssues = `[{"file": "docs/notas.md", "line": 99, "severity": "info", "rule_id": "quality/long-function", "message": "Achado geral sintetico."}]`
	aur439EmptyIssues       = `[]`
)

// IntegrationAUR439 exercises the full --check path end to end against a
// real HTTP fake GitHub on loopback: build the real cmd/aurumcode binary,
// prove a grave finding publishes a "failure" status with an exit that
// never reads as success, a clean finding publishes "success", zero
// findings still publish "success" ("No issues found." plus a cleared
// check), --check needs a commit SHA on its own even when nothing is
// inline-eligible, a read-only token is refused before any POST
// (including to the statuses endpoint), and repeating the grave run
// reproduces the exact same status.
func IntegrationAUR439(t *testing.T) {
	root := aur439IntegrationRoot(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur439-integration")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	workDir := t.TempDir()

	// run drives the binary once with --check. sha == "" means GITHUB_SHA
	// is left genuinely absent from the child's environment (scrubbed,
	// never merely empty).
	run := func(baseURL, token, fixture, sha string) (int, string, string) {
		env := append(aur439IntegrationScrubEnv(),
			"AURUMCODE_LLM_FIXTURE="+fixture,
			"AURUMCODE_GITHUB_API_URL="+baseURL,
			"GITHUB_TOKEN="+token,
		)
		if sha != "" {
			env = append(env, "GITHUB_SHA="+sha)
		}
		cmd := exec.Command(binPath, "review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--check")
		cmd.Dir = workDir
		cmd.Env = env
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("running binary: %v", err)
			}
			code = ee.ExitCode()
		}
		return code, stdout.String(), stderr.String()
	}

	t.Run("grave finding: failure status, exit never reads as success", func(t *testing.T) {
		fake := aur439NewFakeGitHub(t, "write")
		defer fake.Close()
		fixture := aur439ResponseFixture(t, aur439GraveIssues)

		code, stdout, stderr := run(fake.URL, "token-sintetico-write", fixture, "integration-sha-grave")
		if code == 0 {
			t.Fatalf("a grave finding must never exit 0, got 0\nstdout=%s\nstderr=%s", stdout, stderr)
		}

		statuses := fake.statusPosts()
		if len(statuses) != 1 {
			t.Fatalf("expected exactly 1 status POST, got %d: %+v", len(statuses), statuses)
		}
		if statuses[0].Body["state"] != "failure" {
			t.Fatalf("expected state %q, got %v", "failure", statuses[0].Body["state"])
		}
		if statuses[0].Path != "/repos/dono/projeto/statuses/integration-sha-grave" {
			t.Fatalf("expected the status POST anchored at the run's commit SHA, got %s", statuses[0].Path)
		}
		if statuses[0].Body["context"] != "aurumcode/review" {
			t.Fatalf("expected context %q, got %v", "aurumcode/review", statuses[0].Body["context"])
		}
	})

	t.Run("clean finding: success status, exit 0", func(t *testing.T) {
		fake := aur439NewFakeGitHub(t, "write")
		defer fake.Close()
		fixture := aur439ResponseFixture(t, aur439CleanIssues)

		code, stdout, stderr := run(fake.URL, "token-sintetico-write", fixture, "integration-sha-clean")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		statuses := fake.statusPosts()
		if len(statuses) != 1 {
			t.Fatalf("expected exactly 1 status POST, got %d: %+v", len(statuses), statuses)
		}
		if statuses[0].Body["state"] != "success" {
			t.Fatalf("expected state %q, got %v", "success", statuses[0].Body["state"])
		}
	})

	t.Run("zero findings: still publishes a success status", func(t *testing.T) {
		fake := aur439NewFakeGitHub(t, "write")
		defer fake.Close()
		fixture := aur439ResponseFixture(t, aur439EmptyIssues)

		code, stdout, _ := run(fake.URL, "token-sintetico-write", fixture, "integration-sha-empty")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s", code, stdout)
		}
		if !strings.Contains(stdout, "No issues found.") {
			t.Fatalf("expected the pre-existing zero-issue line, got:\n%s", stdout)
		}
		statuses := fake.statusPosts()
		if len(statuses) != 1 {
			t.Fatalf("expected exactly 1 status POST even with zero findings, got %d: %+v", len(statuses), statuses)
		}
		if statuses[0].Body["state"] != "success" {
			t.Fatalf("expected state %q, got %v", "success", statuses[0].Body["state"])
		}
		if len(fake.posts()) != 1 {
			t.Fatalf("expected zero comment POSTs alongside the one status POST, got %d: %+v", len(fake.posts()), fake.posts())
		}
	})

	t.Run("--check needs a commit SHA on its own, even with no inline-eligible finding", func(t *testing.T) {
		fake := aur439NewFakeGitHub(t, "write")
		defer fake.Close()
		fixture := aur439ResponseFixture(t, aur439GeneralOnlyIssues)

		code, stdout, stderr := run(fake.URL, "token-sintetico-write", fixture, "")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "commit SHA") {
			t.Fatalf("expected a clear commit-SHA refusal message, got:\n%s", stderr)
		}
		if posts := fake.posts(); len(posts) != 0 {
			t.Fatalf("expected zero POSTs, got %d: %+v", len(posts), posts)
		}
	})

	t.Run("read-only token: refused before any POST, including to statuses", func(t *testing.T) {
		fake := aur439NewFakeGitHub(t, "readonly")
		defer fake.Close()
		fixture := aur439ResponseFixture(t, aur439GraveIssues)

		code, _, stderr := run(fake.URL, "token-sintetico-readonly", fixture, "integration-sha-readonly")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "refusing to publish") || !strings.Contains(stderr, "write permission") {
			t.Fatalf("expected a clear refusal message, got:\n%s", stderr)
		}
		if posts := fake.posts(); len(posts) != 0 {
			t.Fatalf("expected zero POSTs with a read-only token, got %d: %+v", len(posts), posts)
		}
	})

	t.Run("determinism: same grave input, same published status", func(t *testing.T) {
		fixture := aur439ResponseFixture(t, aur439GraveIssues)

		fakeA := aur439NewFakeGitHub(t, "write")
		defer fakeA.Close()
		codeA, stdoutA, _ := run(fakeA.URL, "token-sintetico-write", fixture, "integration-sha-det")

		fakeB := aur439NewFakeGitHub(t, "write")
		defer fakeB.Close()
		codeB, stdoutB, _ := run(fakeB.URL, "token-sintetico-write", fixture, "integration-sha-det")

		if codeA != codeB {
			t.Fatalf("exit code is not deterministic: %d vs %d", codeA, codeB)
		}
		if stdoutA != stdoutB {
			t.Fatalf("stdout is not deterministic:\nfirst=%q\nsecond=%q", stdoutA, stdoutB)
		}
		if len(fakeA.statusPosts()) != 1 || len(fakeB.statusPosts()) != 1 {
			t.Fatalf("expected exactly 1 status POST per run, got %d and %d", len(fakeA.statusPosts()), len(fakeB.statusPosts()))
		}
		if fakeA.statusPosts()[0].Body["state"] != fakeB.statusPosts()[0].Body["state"] {
			t.Fatalf("published state is not deterministic: %v vs %v", fakeA.statusPosts()[0].Body["state"], fakeB.statusPosts()[0].Body["state"])
		}
	})
}
