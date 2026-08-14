package unit

// Unit selector for card AUR-451: `aurumcode review --pr <numero> --repo
// <dono>/<projeto> --publicar --na-linha` did not read --seguranca,
// --fail-on, --limite or --modelo at all before this card -- the security
// pass never ran, the severity gate never gated, the cost ceiling never
// capped, and the chosen model was never honored on the one path that is
// this product's main use case, reviewing a pull request. This card wires
// all four into cmd/aurumcode/pr.go by calling the exact functions the
// --base path already uses (see that file's package doc), never a second
// implementation.
//
// This file proves the wiring behaviorally, offline, against a real
// httptest fake GitHub built from this card's read_paths fixtures
// (tests/fixtures/scm/github/repo-read-write.json,
// tests/fixtures/scm/github/comment-created.json) plus a diff body this
// file supplies itself: tests/fixtures/scm/github/pr-42.diff (a read_path,
// not writable by this card) carries no security-matchable content, so a
// vulnerability has to be planted in a diff this proof owns, not in that
// shared fixture. The planted line, `API_KEY=AURUM-FAKE-KEY-9000-2222`, is
// the exact synthetic value tests/unit/AUR-442.go's TestAUR442 already
// proves matches security/hardcoded-secret -- reused here rather than a
// new, unvalidated string.
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-451.sh), the same pattern the other cards in this
// wave use, so the file itself is a plain package file.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur451Root(t *testing.T) string {
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

func aur451Fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(aur451Root(t), "tests", "fixtures", "scm", "github", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("fixture obrigatoria ausente %s: %v", p, err)
	}
	return b
}

// aur451VulnDiff is this card's own PR diff, standing in for the shared
// (read-only) tests/fixtures/scm/github/pr-42.diff, which carries nothing a
// security-category rule of the embedded catalog would ever match. Same
// hunk shape AUR-438's own fixture uses (one file, one added line at new
// line 3) so the addedLineNumbers/isInlineEligible classification this
// card does not touch is exercised on the exact same geometry -- only the
// added line's content differs, to plant a matchable secret.
const aur451VulnDiff = "diff --git a/cmdb/settings.go b/cmdb/settings.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/cmdb/settings.go\n" +
	"+++ b/cmdb/settings.go\n" +
	"@@ -1,3 +1,4 @@\n" +
	" package cmdb\n" +
	" \n" +
	"+API_KEY=AURUM-FAKE-KEY-9000-2222\n" +
	" const RetryLimit = 3\n"

// aur451EmptyResponseFixture writes a deterministic offline model response
// with zero quality findings, so any finding a scenario observes came from
// the --seguranca pass, never from the (irrelevant, here) quality review.
func aur451EmptyResponseFixture(t *testing.T) string {
	t.Helper()
	content := `{"issues": [], "summary": "Nenhum achado de qualidade para AUR-451."}`
	path := filepath.Join(t.TempDir(), "response-empty.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// aur451FakeGitHub is a minimal loopback-only fake GitHub server: GET
// /repos/dono/projeto answers with the write-permission fixture, GET
// /repos/dono/projeto/pulls/42 answers with diffBody (this card's own
// vulnerable diff, or a caller-supplied clean one), and any POST answers
// 201 with the shared comment-created fixture while being counted.
type aur451FakeGitHub struct {
	*httptest.Server
	posts int
}

func aur451NewFakeGitHub(t *testing.T, diffBody []byte) *aur451FakeGitHub {
	t.Helper()
	repoJSON := aur451Fixture(t, "repo-read-write.json")
	created := aur451Fixture(t, "comment-created.json")

	f := &aur451FakeGitHub{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.posts++
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

func aur451BuildBinary(t *testing.T) string {
	t.Helper()
	root := aur451Root(t)
	bin := filepath.Join(t.TempDir(), "aurumcode-aur451")
	build := exec.Command("go", "build", "-o", bin, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return bin
}

func aur451ScrubEnv() []string {
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

func aur451Run(t *testing.T, bin, dir string, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(aur451ScrubEnv(), extraEnv...)
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

// TestAUR451 proves AC-001: `--seguranca`, `--fail-on`, `--limite` and
// `--modelo` reach the `--pr` path and reuse the exact functions the
// `--base` path already calls, plus the pre-existing usage-error and
// compatibility surface is untouched.
func TestAUR451(t *testing.T) {
	bin := aur451BuildBinary(t)
	workDir := t.TempDir() // --pr mode reads nothing from the local git repo

	t.Run("--fail-on unknown level is a usage error on --pr", func(t *testing.T) {
		code, _, stderr := aur451Run(t, bin, workDir, nil,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--fail-on", "catastrophic")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--fail-on: unknown level") {
			t.Fatalf("expected the --fail-on error, got:\n%s", stderr)
		}
	})

	t.Run("empty --modelo is a usage error on --pr", func(t *testing.T) {
		code, _, stderr := aur451Run(t, bin, workDir, nil,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--modelo=")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--modelo: model name must not be empty") {
			t.Fatalf("expected the --modelo error, got:\n%s", stderr)
		}
	})

	t.Run("empty --limite is a usage error on --pr", func(t *testing.T) {
		code, _, stderr := aur451Run(t, bin, workDir, nil,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--limite=")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--limite: value must not be empty") {
			t.Fatalf("expected the --limite error, got:\n%s", stderr)
		}
	})

	t.Run("unparsable --limite is a usage error on --pr", func(t *testing.T) {
		code, _, stderr := aur451Run(t, bin, workDir, nil,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--limite", "not-a-number")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--limite:") {
			t.Fatalf("expected a --limite error, got:\n%s", stderr)
		}
	})

	t.Run("--seguranca on --pr reports the planted vulnerability", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + fake.URL,
			"GITHUB_TOKEN=token-sintetico-write",
			"GITHUB_SHA=aur451-unit-sha",
		}
		code, stdout, stderr := aur451Run(t, bin, workDir, env,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--seguranca")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "cmdb/settings.go:3:") || !strings.Contains(stdout, "[error]") {
			t.Fatalf("expected the security finding on stdout, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "rule security/hardcoded-secret") {
			t.Fatalf("expected the security finding to cite its rule, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "publicado na linha") {
			t.Fatalf("expected the finding published inline (the diff added that exact line), got:\n%s", stdout)
		}
		if fake.posts != 1 {
			t.Fatalf("expected exactly 1 comment POST, got %d", fake.posts)
		}
		if strings.Contains(stdout, "Security findings") {
			t.Fatalf("the --pr path publishes findings as comments, it does not print the --base stdout section header, got:\n%s", stdout)
		}
	})

	t.Run("without --seguranca, the same diff reports nothing (the pre-451 gap this card closes)", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + fake.URL,
			"GITHUB_TOKEN=token-sintetico-write",
			"GITHUB_SHA=aur451-unit-sha",
		}
		code, stdout, stderr := aur451Run(t, bin, workDir, env,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "No issues found.") {
			t.Fatalf("expected the empty-review contract, got:\n%s", stdout)
		}
		if fake.posts != 0 {
			t.Fatalf("expected zero comment POSTs, got %d", fake.posts)
		}
	})

	t.Run("--seguranca plus --fail-on high closes the gate on the grave security finding", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + fake.URL,
			"GITHUB_TOKEN=token-sintetico-write",
			"GITHUB_SHA=aur451-unit-sha",
		}
		code, stdout, stderr := aur451Run(t, bin, workDir, env,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--seguranca", "--fail-on", "high")
		if code != 3 {
			t.Fatalf("expected exit 3 (exitFindings), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "finding(s) at severity error or above (--fail-on error)") {
			t.Fatalf("expected the --fail-on gate note, got:\n%s", stderr)
		}
		if fake.posts != 1 {
			t.Fatalf("expected the finding to still be published before the gate closes, got %d posts", fake.posts)
		}
	})

	t.Run("--limite far below cost refuses before any comment is published", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()
		fixture := aur451EmptyResponseFixture(t)

		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + fake.URL,
			"GITHUB_TOKEN=token-sintetico-write",
			"GITHUB_SHA=aur451-unit-sha",
		}
		code, _, stderr := aur451Run(t, bin, workDir, env,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--limite", "0.0001")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "refusing to call the model") {
			t.Fatalf("expected the AUR-433 refusal message, got:\n%s", stderr)
		}
		if fake.posts != 0 {
			t.Fatalf("expected zero comment POSTs (nothing was spent, nothing to publish), got %d", fake.posts)
		}
	})

	t.Run("--modelo unavailable fails loudly on --pr, never an empty review", func(t *testing.T) {
		fake := aur451NewFakeGitHub(t, []byte(aur451VulnDiff))
		defer fake.Close()

		env := []string{
			"AURUMCODE_GITHUB_API_URL=" + fake.URL,
			"GITHUB_TOKEN=token-sintetico-write",
			"GITHUB_SHA=aur451-unit-sha",
		}
		code, stdout, stderr := aur451Run(t, bin, workDir, env,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha", "--modelo", "local")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, `model "local" is unavailable`) {
			t.Fatalf("expected the reportModelUnavailable message, got:\n%s", stderr)
		}
		if fake.posts != 0 {
			t.Fatalf("expected zero comment POSTs, got %d", fake.posts)
		}
	})

	t.Run("--pr's pre-existing usage surface is untouched", func(t *testing.T) {
		code, _, stderr := aur451Run(t, bin, workDir, nil, "review", "--pr", "42", "--publicar", "--na-linha")
		if code != 2 {
			t.Fatalf("expected usage exit 2, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "--repo is required") {
			t.Fatalf("expected the pre-existing --repo message, got:\n%s", stderr)
		}
	})
}
