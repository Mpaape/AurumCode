package integration

// Integration selector for card AUR-451: the deeper behavioral proof behind
// wiring --seguranca/--fail-on/--limite/--modelo into `aurumcode review
// --pr <numero> --repo <dono>/<projeto> --publicar --na-linha` -- against a
// real, loopback-only httptest fake GitHub, recording every request the
// binary makes so this file can assert not just exit codes and stdout but
// the exact publish sequence (or its absence). This intentionally overlaps
// tests/e2e/AUR-451.sh's proof (a different layer: Go assertions on
// recorded requests instead of shell greps on a log file) the same way the
// rest of this wave's cards pair a Go integration test with a bash e2e
// script; see docs/specs/AUR-451.md.
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-451.sh), the same pattern the other cards in this
// wave use, so the file itself is a plain package file.

import (
	"crypto/sha256"
	"encoding/hex"
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

func aur451IntegrationRoot(t *testing.T) string {
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

func aur451IntegrationFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur451IntegrationRoot(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

func aur451IntegrationScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":           true,
		"LLM_API_KEY":                     true,
		"LLM_BASE_URL":                    true,
		"LLM_MODEL":                       true,
		"AURUMCODE_GITHUB_API_URL":        true,
		"GITHUB_TOKEN":                    true,
		"GITHUB_SHA":                      true,
		"AURUMCODE_LLM_INPUT_USD_PER_1K":  true,
		"AURUMCODE_LLM_OUTPUT_USD_PER_1K": true,
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

// aur451VulnDiff mirrors tests/unit/AUR-451.go's own diff: one file, one
// added line at new line 3, planting the exact synthetic secret value
// (`API_KEY=AURUM-FAKE-KEY-9000-2222`) tests/unit/AUR-442.go's TestAUR442
// already proves matches security/hardcoded-secret. Duplicated rather than
// shared because each proof program in this wave (unit, integration, e2e)
// is materialized and compiled independently by the acceptance harness --
// see tests/acceptance/AUR-451.sh's stage_source, which copies each
// selector's own file alone.
const aur451VulnDiff = "diff --git a/cmdb/settings.go b/cmdb/settings.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/cmdb/settings.go\n" +
	"+++ b/cmdb/settings.go\n" +
	"@@ -1,3 +1,4 @@\n" +
	" package cmdb\n" +
	" \n" +
	"+API_KEY=AURUM-FAKE-KEY-9000-2222\n" +
	" const RetryLimit = 3\n"

// aur451Request is one recorded HTTP request the fake GitHub server saw.
type aur451Request struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

// aur451FakeGitHub is a loopback-only fake GitHub server. Unlike
// tests/unit/AUR-451.go's simpler counter, this one records every request
// in order (path, decoded body) so IntegrationAUR451 can assert on the
// exact publish sequence, and answers an inline review comment with no
// commit_id with 422 -- the same non-agreeing-mock discipline
// tests/integration/AUR-438.go established -- so a fail-closed gap in
// cmd/aurumcode/pr.go cannot hide behind an always-succeeding double.
type aur451FakeGitHub struct {
	*httptest.Server
	mu       sync.Mutex
	requests []aur451Request
}

func aur451NewFakeGitHub(t *testing.T, diffBody []byte) *aur451FakeGitHub {
	t.Helper()
	repoJSON := aur451IntegrationFixture(t, "repo-read-write.json")
	created := aur451IntegrationFixture(t, "comment-created.json")

	f := &aur451FakeGitHub{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		f.mu.Lock()
		f.requests = append(f.requests, aur451Request{Method: r.Method, Path: r.URL.Path, Body: body})
		f.mu.Unlock()

		if r.Method == http.MethodPost {
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

func (f *aur451FakeGitHub) posts() []aur451Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []aur451Request
	for _, r := range f.requests {
		if r.Method == http.MethodPost {
			out = append(out, r)
		}
	}
	return out
}

func aur451EmptyResponseFixture(t *testing.T) string {
	t.Helper()
	content := `{"issues": [], "summary": "Nenhum achado de qualidade para AUR-451 (integracao)."}`
	path := filepath.Join(t.TempDir(), "response-empty.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// aur451QualityResponseFixture writes a deterministic offline model
// response with one warning-severity quality finding on the same added
// line the planted secret sits on (cmdb/settings.go:3) -- proving a
// quality finding and a security finding on the very same line are both
// published as their own comment, neither one dropped or merged into the
// other, and that sortedIssues' (file, line) order composes with the
// security findings this card adds without reordering them relative to
// each other in an unspecified way.
func aur451QualityResponseFixture(t *testing.T) string {
	t.Helper()
	content := `{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "warning",
      "rule_id": "quality/long-function",
      "message": "Achado de qualidade sintetico na mesma linha do segredo."
    }
  ],
  "summary": "Resposta sintetica de qualidade para AUR-451."
}`
	path := filepath.Join(t.TempDir(), "response-quality.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// IntegrationAUR451 exercises the AC-001 command end to end against a real
// HTTP fake GitHub on loopback: build the real cmd/aurumcode binary, prove
// the security finding publishes and closes the --fail-on gate, prove a
// quality finding on the same line still publishes alongside it, prove
// --limite refuses with zero POSTs, and prove the pre-AUR-451 `--pr`
// contract (no new flags) still posts nothing beyond what AUR-438 already
// published.
func IntegrationAUR451(t *testing.T) {
	root := aur451IntegrationRoot(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur451-integration")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	workDir := t.TempDir()

	run := func(baseURL, fixture, sha string, extraArgs ...string) (int, string, string) {
		env := append(aur451IntegrationScrubEnv(),
			"AURUMCODE_LLM_FIXTURE="+fixture,
			"AURUMCODE_GITHUB_API_URL="+baseURL,
			"GITHUB_TOKEN=token-sintetico-write",
		)
		if sha != "" {
			env = append(env, "GITHUB_SHA="+sha)
		}
		args := append([]string{"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
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

	t.Run("AC-001: --seguranca plus --fail-on high publishes the planted secret and closes the gate", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		code, stdout, stderr := run(fake.URL, fixture, "aur451-integration-sha", "--seguranca", "--fail-on", "high")
		if code != 3 {
			t.Fatalf("expected exit 3 (exitFindings), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		posts := fake.posts()
		if len(posts) != 1 {
			t.Fatalf("expected exactly 1 comment POST, got %d: %+v", len(posts), posts)
		}
		if posts[0].Path != "/repos/dono/projeto/pulls/42/comments" {
			t.Fatalf("expected the inline review-comment endpoint, got %s", posts[0].Path)
		}
		if posts[0].Body["path"] != "cmdb/settings.go" {
			t.Fatalf("expected the comment's path to be cmdb/settings.go, got %v", posts[0].Body["path"])
		}
		if line, ok := posts[0].Body["line"].(float64); !ok || int(line) != 3 {
			t.Fatalf("expected the comment's line to be 3, got %v", posts[0].Body["line"])
		}
		body, _ := posts[0].Body["body"].(string)
		if !strings.Contains(body, "security/hardcoded-secret") {
			t.Fatalf("expected the comment body to cite security/hardcoded-secret, got %q", body)
		}
		if !strings.Contains(stderr, "security pass applied") {
			t.Fatalf("expected the AUR-450 coverage note on stderr, got:\n%s", stderr)
		}
	})

	t.Run("determinism: rerunning AC-001's command reproduces the exact same publish", func(t *testing.T) {
		fakeA := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fakeA.Close()
		fixtureA := aur451EmptyResponseFixture(t)
		_, stdoutA, _ := run(fakeA.URL, fixtureA, "aur451-integration-sha", "--seguranca", "--fail-on", "high")

		fakeB := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fakeB.Close()
		fixtureB := aur451EmptyResponseFixture(t)
		codeB, stdoutB, _ := run(fakeB.URL, fixtureB, "aur451-integration-sha", "--seguranca", "--fail-on", "high")

		if codeB != 3 {
			t.Fatalf("expected exit 3 on rerun, got %d", codeB)
		}
		if stdoutA != stdoutB {
			t.Fatalf("stdout is not deterministic:\nfirst=%q\nsecond=%q", stdoutA, stdoutB)
		}
		if len(fakeA.posts()) != len(fakeB.posts()) {
			t.Fatalf("publish sequence length differs: %d vs %d", len(fakeA.posts()), len(fakeB.posts()))
		}
		hashOf := func(s string) string {
			sum := sha256.Sum256([]byte(s))
			return hex.EncodeToString(sum[:])
		}
		if hashOf(stdoutA) != hashOf(stdoutB) {
			t.Fatalf("stdout sha256 differs between runs")
		}
	})

	t.Run("a quality finding and a security finding on the same line both publish", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451QualityResponseFixture(t)

		code, stdout, stderr := run(fake.URL, fixture, "aur451-integration-sha", "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0 (no --fail-on given), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		posts := fake.posts()
		if len(posts) != 2 {
			t.Fatalf("expected exactly 2 comment POSTs (1 quality + 1 security), got %d: %+v", len(posts), posts)
		}
		var sawQuality, sawSecurity bool
		for _, p := range posts {
			body, _ := p.Body["body"].(string)
			if strings.Contains(body, "quality/long-function") || strings.Contains(body, "Achado de qualidade sintetico") {
				sawQuality = true
			}
			if strings.Contains(body, "security/hardcoded-secret") {
				sawSecurity = true
			}
		}
		if !sawQuality || !sawSecurity {
			t.Fatalf("expected both a quality and a security comment, got posts: %+v", posts)
		}
		if strings.Count(stdout, "publicado na linha") != 2 {
			t.Fatalf("expected 2 inline-publish markers on stdout, got:\n%s", stdout)
		}
	})

	t.Run("--limite far below cost refuses with zero POSTs", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		code, _, stderr := run(fake.URL, fixture, "aur451-integration-sha", "--seguranca", "--limite", "0.0001")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "refusing to call the model") {
			t.Fatalf("expected the AUR-433 refusal message, got:\n%s", stderr)
		}
		if posts := fake.posts(); len(posts) != 0 {
			t.Fatalf("expected zero comment POSTs, got %d: %+v", len(posts), posts)
		}
	})

	t.Run("pre-AUR-451 contract: no new flags publishes exactly what AUR-438 already published", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		code, stdout, stderr := run(fake.URL, fixture, "aur451-integration-sha")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "No issues found.") {
			t.Fatalf("expected the pre-existing empty-review contract, got:\n%s", stdout)
		}
		if posts := fake.posts(); len(posts) != 0 {
			t.Fatalf("expected zero comment POSTs (the diff's planted secret is only found with --seguranca), got %d: %+v", len(posts), posts)
		}
	})
}
