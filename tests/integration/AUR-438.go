package integration

// Integration selector for card AUR-438: the deeper behavioral proof behind
// `aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar
// --na-linha` against a real, loopback-only httptest fake GitHub built
// from this card's fixtures (tests/fixtures/scm/github) -- exact-line vs
// general-comment classification, determinism across repeated runs, and
// the read-only-token refusal (fail-closed, zero POSTs). This
// intentionally overlaps tests/e2e/AUR-438.sh's proof (a different layer,
// Go assertions on parsed JSON bodies instead of shell greps on a log
// file) the same way the rest of this wave's cards pair a Go integration
// test with a bash e2e script; see docs/specs/AUR-438.md.
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-438.sh), the same pattern the other cards in this
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

func aur438IntegrationRoot(t *testing.T) string {
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

func aur438IntegrationFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur438IntegrationRoot(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

func aur438IntegrationScrubEnv() []string {
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

// aur438Request is one recorded HTTP request the fake GitHub server saw.
type aur438Request struct {
	Method string
	Path   string
	Body   map[string]interface{} // nil for a GET
}

// aur438FakeGitHub is a loopback-only fake GitHub server built from this
// card's fixtures. scenario selects the permissions fixture ("write" or
// "readonly"); every request is recorded, in order, for the caller to
// assert on.
//
// It is deliberately not a mock that "always agrees": a POST to the inline
// review-comment endpoint with an empty (or missing) commit_id is answered
// 422, exactly like the real GitHub API -- an inline comment cannot be
// anchored to no commit at all. A prior version of this fixture accepted
// any commit_id, which is why the missing pre-POST SHA gate in
// cmd/aurumcode/pr.go went unnoticed: nothing in the test double could ever
// fail the way the real API does. FailNthPost/FailStatus additionally let a
// test make one specific POST in a sequence fail (for any reason) without
// touching the commit_id rule, to prove a single failed POST does not stop
// the rest of the publish loop from being attempted.
type aur438FakeGitHub struct {
	*httptest.Server
	mu       sync.Mutex
	requests []aur438Request
	postSeen int

	// FailNthPost, when non-zero, makes the Nth POST this server receives
	// (1-indexed, across both endpoints) fail with FailStatus instead of
	// succeeding. Set before the server receives its first request.
	FailNthPost int
	FailStatus  int
}

func aur438NewFakeGitHub(t *testing.T, scenario string) *aur438FakeGitHub {
	t.Helper()
	diffBody := aur438IntegrationFixture(t, "pr-42.diff")
	created := aur438IntegrationFixture(t, "comment-created.json")
	var repoJSON []byte
	if scenario == "write" {
		repoJSON = aur438IntegrationFixture(t, "repo-read-write.json")
	} else {
		repoJSON = aur438IntegrationFixture(t, "repo-read-only.json")
	}

	f := &aur438FakeGitHub{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, aur438Request{Method: r.Method, Path: r.URL.Path, Body: body})
		var postN int
		if r.Method == http.MethodPost {
			f.postSeen++
			postN = f.postSeen
		}
		failNth, failStatus := f.FailNthPost, f.FailStatus
		f.mu.Unlock()

		if r.Method == http.MethodPost {
			// Real GitHub: an inline review comment with no commit_id is a
			// validation failure, not a success.
			if r.URL.Path == "/repos/dono/projeto/pulls/42/comments" {
				commitID, _ := body["commit_id"].(string)
				if commitID == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"field":"commit_id","code":"missing_field"}]}`))
					return
				}
			}
			if failNth != 0 && postN == failNth {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(failStatus)
				_, _ = w.Write([]byte(`{"message":"synthetic failure injected for AUR-438's own test"}`))
				return
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

func (f *aur438FakeGitHub) posts() []aur438Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []aur438Request
	for _, r := range f.requests {
		if r.Method == http.MethodPost {
			out = append(out, r)
		}
	}
	return out
}

// aur438ResponseFixture writes a deterministic offline model response with
// two findings -- one on cmdb/settings.go's added line 3 (inline-eligible),
// one on docs/notas.md line 99 (nowhere near that file's single +1-line
// hunk, so it must become a general comment, never dropped) -- and returns
// its path.
func aur438ResponseFixture(t *testing.T) string {
	t.Helper()
	content := `{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "warning",
      "rule_id": "quality/long-function",
      "message": "Achado sintetico na linha que o diff adicionou."
    },
    {
      "file": "docs/notas.md",
      "line": 99,
      "severity": "info",
      "rule_id": "quality/long-function",
      "message": "Achado sintetico fora das linhas alteradas."
    }
  ],
  "summary": "Resposta sintetica de integracao para AUR-438."
}`
	path := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// IntegrationAUR438 exercises the full PR review path end to end against a
// real HTTP fake GitHub on loopback: build the real cmd/aurumcode binary,
// run it with a write-permission token and assert the exact publish
// transcript (one inline review comment anchored at file+line, one general
// issue comment for the out-of-range finding), repeat it and assert byte-
// identical stdout and an identical publish sequence, then run it again
// with a read-only token and assert zero POSTs reached the server.
func IntegrationAUR438(t *testing.T) {
	root := aur438IntegrationRoot(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur438-integration")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	fixture := aur438ResponseFixture(t)
	workDir := t.TempDir()

	// run drives the binary once. sha == "" means GITHUB_SHA is left
	// genuinely absent from the child's environment (aur438IntegrationScrubEnv
	// already strips any inherited copy), not merely empty -- the two are
	// indistinguishable to os.Getenv, but this keeps the intent explicit at
	// each call site.
	run := func(baseURL, token, sha string) (int, string, string) {
		env := append(aur438IntegrationScrubEnv(),
			"AURUMCODE_LLM_FIXTURE="+fixture,
			"AURUMCODE_GITHUB_API_URL="+baseURL,
			"GITHUB_TOKEN="+token,
		)
		if sha != "" {
			env = append(env, "GITHUB_SHA="+sha)
		}
		cmd := exec.Command(binPath, "review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha")
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

	t.Run("write token: inline plus general, nothing dropped", func(t *testing.T) {
		fake := aur438NewFakeGitHub(t, "write")
		defer fake.Close()

		code, stdout, stderr := run(fake.URL, "token-sintetico-write", "integration-synthetic-sha")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}

		posts := fake.posts()
		if len(posts) != 2 {
			t.Fatalf("expected exactly 2 POSTs (1 inline + 1 general), got %d: %+v", len(posts), posts)
		}

		inline := posts[0]
		if inline.Path != "/repos/dono/projeto/pulls/42/comments" {
			t.Fatalf("expected the first POST to be the inline review-comment endpoint, got %s", inline.Path)
		}
		if inline.Body["path"] != "cmdb/settings.go" {
			t.Fatalf("expected inline comment path cmdb/settings.go, got %v", inline.Body["path"])
		}
		if line, ok := inline.Body["line"].(float64); !ok || int(line) != 3 {
			t.Fatalf("expected inline comment line 3, got %v", inline.Body["line"])
		}

		general := posts[1]
		if general.Path != "/repos/dono/projeto/issues/42/comments" {
			t.Fatalf("expected the second POST to be the general issue-comment endpoint, got %s", general.Path)
		}
		body, _ := general.Body["body"].(string)
		if !strings.Contains(body, "docs/notas.md:99") {
			t.Fatalf("expected the general comment to name docs/notas.md:99, got %q", body)
		}

		if !strings.Contains(stdout, "publicado na linha") || !strings.Contains(stdout, "publicado como comentario geral") {
			t.Fatalf("expected both publish markers on stdout, got:\n%s", stdout)
		}
	})

	t.Run("determinism: same input, same output, same publish sequence", func(t *testing.T) {
		fakeA := aur438NewFakeGitHub(t, "write")
		defer fakeA.Close()
		_, stdoutA, _ := run(fakeA.URL, "token-sintetico-write", "integration-synthetic-sha")

		fakeB := aur438NewFakeGitHub(t, "write")
		defer fakeB.Close()
		codeB, stdoutB, _ := run(fakeB.URL, "token-sintetico-write", "integration-synthetic-sha")

		if codeB != 0 {
			t.Fatalf("expected exit 0 on rerun, got %d", codeB)
		}
		if stdoutA != stdoutB {
			t.Fatalf("stdout is not deterministic:\nfirst=%q\nsecond=%q", stdoutA, stdoutB)
		}
		if len(fakeA.posts()) != len(fakeB.posts()) {
			t.Fatalf("publish sequence length differs: %d vs %d", len(fakeA.posts()), len(fakeB.posts()))
		}
		for i := range fakeA.posts() {
			if fakeA.posts()[i].Path != fakeB.posts()[i].Path {
				t.Fatalf("publish sequence order differs at %d: %s vs %s", i, fakeA.posts()[i].Path, fakeB.posts()[i].Path)
			}
		}
	})

	t.Run("read-only token: refused before any POST (MUT-001's neighbor, AUR-437's guarantee)", func(t *testing.T) {
		fake := aur438NewFakeGitHub(t, "readonly")
		defer fake.Close()

		code, _, stderr := run(fake.URL, "token-sintetico-readonly", "integration-synthetic-sha")
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

	t.Run("no GITHUB_SHA: refused before any POST, never a POST with an empty commit_id", func(t *testing.T) {
		// The exact gap the reviewer found: AC-001's declared command sets
		// no GITHUB_SHA in Preconditions. Without the pre-POST gate in
		// cmd/aurumcode/pr.go, the first (inline) POST would carry
		// commit_id:"" and this fake -- unlike the old, always-agreeing one
		// -- would answer 422, exactly like the real GitHub API. The
		// assertion below is stronger than "the 422 happened": it checks
		// that ZERO POSTs were attempted at all, proving the refusal is a
		// pre-POST gate in this card's own code, not a response the fake
		// server had to reject after the fact.
		fake := aur438NewFakeGitHub(t, "write")
		defer fake.Close()

		code, stdout, stderr := run(fake.URL, "token-sintetico-write", "")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "commit SHA") {
			t.Fatalf("expected a clear commit-SHA refusal message, got:\n%s", stderr)
		}
		if posts := fake.posts(); len(posts) != 0 {
			t.Fatalf("expected zero POSTs with no GITHUB_SHA, got %d: %+v", len(posts), posts)
		}
	})

	t.Run("one failed POST does not swallow the other finding", func(t *testing.T) {
		// The other half of the same reviewer finding: cmd/aurumcode/pr.go
		// used to `return 1` the instant any single POST failed, which
		// meant the sorted publish order (cmdb/settings.go:3 before
		// docs/notas.md:99) made an inline failure silently cost the
		// general comment too. FailNthPost fails the first POST this
		// server receives with a plain 400 -- not the 422/commit_id rule
		// above, and deliberately not a 5xx or 429/403-rate-limit shape:
		// internal/git/githubclient's own client already retries those
		// transparently (see its doRequest), which would silently recover
		// from the injected failure on the next attempt and prove
		// nothing. A 400 is returned immediately, no retry, so it reaches
		// runPRReview as a real, single failed publish. GITHUB_SHA is set,
		// so the fake's commit_id check is satisfied and would not itself
		// reject anything.
		fake := aur438NewFakeGitHub(t, "write")
		fake.FailNthPost = 1
		fake.FailStatus = http.StatusBadRequest
		defer fake.Close()

		code, stdout, stderr := run(fake.URL, "token-sintetico-write", "integration-synthetic-sha")
		if code != 1 {
			t.Fatalf("expected exit 1 (one publish failed), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}

		posts := fake.posts()
		if len(posts) != 2 {
			t.Fatalf("expected both POSTs to still be attempted, got %d: %+v", len(posts), posts)
		}
		if posts[0].Path != "/repos/dono/projeto/pulls/42/comments" {
			t.Fatalf("expected the first (failing) POST to be the inline endpoint, got %s", posts[0].Path)
		}
		if posts[1].Path != "/repos/dono/projeto/issues/42/comments" {
			t.Fatalf("expected the second POST to still be the general endpoint, got %s", posts[1].Path)
		}
		if !strings.Contains(stdout, "docs/notas.md:99") || !strings.Contains(stdout, "publicado como comentario geral") {
			t.Fatalf("expected the second finding to still be published despite the first POST failing, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "cmdb/settings.go:3") && strings.Contains(stdout, "publicado na linha") {
			t.Fatalf("the failed inline POST must not be reported as published, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, "1 comentario(s) falharam") {
			t.Fatalf("expected the aggregated-failure summary on stderr, got:\n%s", stderr)
		}
	})
}
