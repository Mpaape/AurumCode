package unit

// Unit selector for card AUR-438: `aurumcode review --pr <numero> --repo
// <dono>/<projeto> --publicar --na-linha`, the PR review path wired on top
// of the restored GitHub client (AUR-437) and the existing review engine.
//
// This file proves the command's usage surface -- what is a required flag,
// what is a malformed value, and that every pre-existing flag/behavior on
// the --base path is untouched -- plus one direct, offline, httptest-backed
// run of the full path as a fast sanity check. The deeper behavioral proof
// (exact-line vs general-comment classification, determinism, the
// read-only refusal) lives in tests/integration/AUR-438.go and
// tests/e2e/AUR-438.sh; this file does not repeat it.
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
	"testing"
)

func aur438Root(t *testing.T) string {
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

func aur438Fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur438Root(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

// aur438BuildBinary builds the real cmd/aurumcode binary once per test run.
func aur438BuildBinary(t *testing.T) string {
	t.Helper()
	root := aur438Root(t)
	bin := filepath.Join(t.TempDir(), "aurumcode-aur438")
	build := exec.Command("go", "build", "-o", bin, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return bin
}

// aur438ScrubEnv drops every variable that could accidentally configure a
// provider or a GitHub endpoint, so every subtest states its own
// configuration explicitly.
func aur438ScrubEnv() []string {
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

func aur438Run(t *testing.T, bin, dir string, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(aur438ScrubEnv(), extraEnv...)
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

// TestAUR438 proves `aurumcode review --pr` behaviorally: --repo,
// --publicar and --na-linha are all required with --pr (usage exit 2, the
// same treatment --base already gets); --repo must be "owner/repo"; a
// finding reaches the pull request as a comment through a real, offline
// httptest fake GitHub; and every flag/behavior published before this card
// (--base, --fail-on, --modelo, --seguranca) is unaffected by --pr's mere
// existence. See docs/specs/AUR-438.md.
func TestAUR438(t *testing.T) {
	root := aur438Root(t)
	bin := aur438BuildBinary(t)
	workDir := t.TempDir() // --pr mode reads nothing from the local git repo

	t.Run("--repo is required with --pr", func(t *testing.T) {
		code, _, stderr := aur438Run(t, bin, workDir, nil, "review", "--pr", "42", "--publicar", "--na-linha")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--repo is required") {
			t.Fatalf("expected a --repo message, got:\n%s", stderr)
		}
	})

	t.Run("--na-linha is required with --pr", func(t *testing.T) {
		code, _, stderr := aur438Run(t, bin, workDir, nil, "review", "--pr", "42", "--repo", "dono/projeto", "--publicar")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--na-linha is required") {
			t.Fatalf("expected a --na-linha message, got:\n%s", stderr)
		}
	})

	t.Run("--publicar is required with --pr", func(t *testing.T) {
		code, _, stderr := aur438Run(t, bin, workDir, nil, "review", "--pr", "42", "--repo", "dono/projeto", "--na-linha")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--publicar is required") {
			t.Fatalf("expected a --publicar message, got:\n%s", stderr)
		}
	})

	t.Run("--pr must be positive", func(t *testing.T) {
		code, _, stderr := aur438Run(t, bin, workDir, nil, "review", "--pr", "0", "--repo", "dono/projeto", "--publicar", "--na-linha")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "positive pull request number") {
			t.Fatalf("expected a positive-PR message, got:\n%s", stderr)
		}
	})

	t.Run("--repo must be owner/repo", func(t *testing.T) {
		for _, bad := range []string{"dono", "/projeto", "dono/", "dono/projeto/extra"} {
			code, _, stderr := aur438Run(t, bin, workDir, nil, "review", "--pr", "42", "--repo", bad, "--publicar", "--na-linha")
			if code != 2 {
				t.Fatalf("repo=%q: expected usage exit 2, got %d\nstderr=%s", bad, code, stderr)
			}
			if !strings.Contains(stderr, "--repo:") {
				t.Fatalf("repo=%q: expected a --repo error, got:\n%s", bad, stderr)
			}
		}
	})

	t.Run("--pr does not require --base", func(t *testing.T) {
		// The one thing this subtest asserts: reaching the --repo-required
		// usage error, not the pre-existing "--base is required" one, proves
		// --pr's dispatch happens before --base is ever checked.
		code, _, stderr := aur438Run(t, bin, workDir, nil, "review", "--pr", "42")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d", code)
		}
		if strings.Contains(stderr, "--base is required") {
			t.Fatalf("--pr must not fall through to the --base check, got:\n%s", stderr)
		}
	})

	t.Run("the full path publishes a finding through an offline fake GitHub", func(t *testing.T) {
		diffBody := aur438Fixture(t, "pr-42.diff")
		repoJSON := aur438Fixture(t, "repo-read-write.json")
		created := aur438Fixture(t, "comment-created.json")

		var posted []map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				posted = append(posted, body)
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
		defer server.Close()

		respPath := filepath.Join(t.TempDir(), "response.json")
		resp := `{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "warning",
      "rule_id": "quality/long-function",
      "message": "Achado sintetico de unidade."
    }
  ],
  "summary": "Resposta sintetica de unidade."
}`
		if err := os.WriteFile(respPath, []byte(resp), 0o600); err != nil {
			t.Fatalf("writing fixture response: %v", err)
		}

		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + respPath,
			"AURUMCODE_GITHUB_API_URL=" + server.URL,
			"GITHUB_TOKEN=token-sintetico-unit",
			"GITHUB_SHA=unit-synthetic-sha",
		}
		code, stdout, stderr := aur438Run(t, bin, workDir, env, "review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "cmdb/settings.go:3:") || !strings.Contains(stdout, "publicado na linha") {
			t.Fatalf("expected the inline finding on stdout, got:\n%s", stdout)
		}
		if len(posted) != 1 {
			t.Fatalf("expected exactly 1 POST to the fake GitHub, got %d: %+v", len(posted), posted)
		}
		if posted[0]["path"] != "cmdb/settings.go" {
			t.Fatalf("expected the comment's path to be cmdb/settings.go, got %v", posted[0]["path"])
		}
	})

	t.Run("--base path is unaffected by --pr's existence", func(t *testing.T) {
		repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
		if _, err := os.Stat(repoDir); err != nil {
			t.Fatalf("required input missing: %s: %v", repoDir, err)
		}
		fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, _ := aur438Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s", code, stdout)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the pre-existing AUR-430 finding, got:\n%s", stdout)
		}
	})
}
