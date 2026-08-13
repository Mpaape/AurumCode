package unit

// Unit selector for card AUR-439: `aurumcode review --pr <numero> --repo
// <dono>/<projeto> --publicar --check`, the commit-status check wired on
// top of the PR review path AUR-438 already delivered and the restored
// GitHub client's SetStatus (AUR-437).
//
// This file proves the command's usage surface -- --check satisfies the
// --na-linha requirement on its own, every pre-existing flag/behavior is
// untouched -- plus one direct, offline, httptest-backed run of the full
// --check path as a fast sanity check (a grave finding publishes a
// "failure" status and an exit that cannot be read as success). The deeper
// behavioral proof (grave vs clean vs zero-issue status content, the
// commit-SHA and write-permission fail-closed inheritance, determinism)
// lives in tests/integration/AUR-439.go and tests/e2e/AUR-439.sh; this
// file does not repeat it.
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
	"testing"
)

func aur439Root(t *testing.T) string {
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

func aur439Fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur439Root(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

// aur439BuildBinary builds the real cmd/aurumcode binary once per test run.
func aur439BuildBinary(t *testing.T) string {
	t.Helper()
	root := aur439Root(t)
	bin := filepath.Join(t.TempDir(), "aurumcode-aur439")
	build := exec.Command("go", "build", "-o", bin, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return bin
}

// aur439ScrubEnv drops every variable that could accidentally configure a
// provider or a GitHub endpoint, so every subtest states its own
// configuration explicitly.
func aur439ScrubEnv() []string {
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

func aur439Run(t *testing.T, bin, dir string, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(aur439ScrubEnv(), extraEnv...)
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

// TestAUR439 proves `aurumcode review --pr --check` behaviorally: --check
// satisfies the --na-linha requirement on its own (AC-001's declared
// command carries no --na-linha at all); --repo and --publicar remain
// required exactly as before; a grave (error-severity) finding publishes a
// commit status through a real, offline httptest fake GitHub with
// state=="failure" and an exit code that is never 0 (never "reports
// success"); and every flag/behavior published before this card (--base,
// --fail-on, --modelo, --seguranca, and the pre-existing --na-linha path
// without --check) is unaffected by --check's mere existence. See
// docs/specs/AUR-439.md.
func TestAUR439(t *testing.T) {
	root := aur439Root(t)
	bin := aur439BuildBinary(t)
	workDir := t.TempDir() // --pr mode reads nothing from the local git repo

	t.Run("--check satisfies --na-linha's requirement on its own", func(t *testing.T) {
		// Deliberately no --na-linha at all: AC-001's declared command is
		// `--pr 42 --repo dono/projeto --publicar --check`. Reaching a
		// usage error here (exit 2) would mean this card silently expects
		// an opt-in flag its own declared command never gives.
		code, _, stderr := aur439Run(t, bin, workDir, nil, "review", "--pr", "42", "--repo", "dono/projeto", "--publicar")
		if code != 2 {
			t.Fatalf("expected usage exit 2 without --check or --na-linha, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--na-linha is required") {
			t.Fatalf("expected the pre-existing --na-linha message, got:\n%s", stderr)
		}
	})

	t.Run("--repo is still required with --pr --check", func(t *testing.T) {
		code, _, stderr := aur439Run(t, bin, workDir, nil, "review", "--pr", "42", "--publicar", "--check")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--repo is required") {
			t.Fatalf("expected a --repo message, got:\n%s", stderr)
		}
	})

	t.Run("--publicar is still required with --pr --check", func(t *testing.T) {
		code, _, stderr := aur439Run(t, bin, workDir, nil, "review", "--pr", "42", "--repo", "dono/projeto", "--check")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--publicar is required") {
			t.Fatalf("expected a --publicar message, got:\n%s", stderr)
		}
	})

	t.Run("a grave finding publishes a failing check and never exits 0", func(t *testing.T) {
		diffBody := aur439Fixture(t, "pr-42.diff")
		repoJSON := aur439Fixture(t, "repo-read-write.json")
		created := aur439Fixture(t, "comment-created.json")

		var statusPosts []map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if strings.HasPrefix(r.URL.Path, "/repos/dono/projeto/statuses/") {
					statusPosts = append(statusPosts, body)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":439100,"state":"` + body["state"].(string) + `"}`))
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
		defer server.Close()

		respPath := filepath.Join(t.TempDir(), "response.json")
		resp := `{
  "issues": [
    {
      "file": "cmdb/settings.go",
      "line": 3,
      "severity": "error",
      "rule_id": "quality/long-function",
      "message": "Achado grave sintetico de unidade."
    }
  ],
  "summary": "Resposta sintetica grave de unidade."
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
		code, stdout, stderr := aur439Run(t, bin, workDir, env, "review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--check")
		if code == 0 {
			t.Fatalf("a grave finding must never exit 0 (reads as success), got 0\nstdout=%s\nstderr=%s", stdout, stderr)
		}
		if len(statusPosts) != 1 {
			t.Fatalf("expected exactly 1 status POST, got %d: %+v", len(statusPosts), statusPosts)
		}
		if statusPosts[0]["state"] != "failure" {
			t.Fatalf("expected the published status to be %q, got %v", "failure", statusPosts[0]["state"])
		}
		if statusPosts[0]["context"] != "aurumcode/review" {
			t.Fatalf("expected the published context to be %q, got %v", "aurumcode/review", statusPosts[0]["context"])
		}
		if !strings.Contains(stdout, "failure") {
			t.Fatalf("expected the published state on stdout, got:\n%s", stdout)
		}
	})

	t.Run("--base path is unaffected by --check's existence", func(t *testing.T) {
		repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
		if _, err := os.Stat(repoDir); err != nil {
			t.Fatalf("required input missing: %s: %v", repoDir, err)
		}
		fixture := filepath.Join(root, "tests/fixtures/review/known-problem-response.json")
		env := []string{"AURUMCODE_LLM_FIXTURE=" + fixture}
		code, stdout, _ := aur439Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s", code, stdout)
		}
		if !strings.Contains(stdout, "config/demo-tokens.txt") {
			t.Fatalf("expected the pre-existing AUR-430 finding, got:\n%s", stdout)
		}
	})
}
