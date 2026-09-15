package unit

// Unit selector for card AUR-505: when the model answers with a response the
// parser cannot validate, `aurumcode review --pr ... --publicar --seguranca
// --check` degrades instead of crashing.
//
// WHAT THIS PROVES (real assertions, offline, no network):
//
//   AC-001: with an invalid model response the deterministic findings
//   (static analysis + the --seguranca pass) are still published, a
//   declared limitation names the model failure, and the exit code follows
//   the deterministic gate -- never a blanket 1 with empty output. Two
//   diffs prove both directions of that gate: a diff that trips a
//   deterministic error finding exits 3 (the --check failure code), and a
//   deterministic-clean diff exits 0 even though the model answered
//   garbage.
//
//   AC-002: a valid response still merges normally (its finding is
//   published and no invalid-output limitation appears).
//
//   AC-003: the invalid response is recorded (never silent) and NO finding
//   is fabricated from it -- the deterministic finding set published for
//   the invalid response is byte-identical to the set published for a
//   valid empty response over the same diff.
//
// The harness bridges this file with a generated _test.go shim (see
// tests/acceptance/AUR-505.sh), the same pattern AUR-439's unit file uses,
// so this is a plain package file.
//
// EXIT CODES: this file is a Go test; the acceptance script owns 64/79.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur505Root(t *testing.T) string {
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

func aur505BuildBinary(t *testing.T) string {
	t.Helper()
	root := aur505Root(t)
	bin := filepath.Join(t.TempDir(), "aurumcode-aur505")
	build := exec.Command("go", "build", "-o", bin, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return bin
}

func aur505ScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":        true,
		"LLM_API_KEY":                  true,
		"LLM_BASE_URL":                 true,
		"LLM_MODEL":                    true,
		"AURUMCODE_GITHUB_API_URL":     true,
		"AURUMCODE_PR_PERMISSION_MODE": true,
		"GITHUB_TOKEN":                 true,
		"GITHUB_SHA":                   true,
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

func aur505Run(t *testing.T, bin, dir string, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(aur505ScrubEnv(), extraEnv...)
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

// aur505Fake is a loopback-only fake GitHub. Every POST body is recorded so
// the published findings and summary can be asserted on directly; no file
// from the repository is read, so the unit proof is self-contained.
type aur505Fake struct {
	server   *httptest.Server
	posts    []string
	diffBody string
}

const aur505WriteJSON = `{"id":505001,"full_name":"dono/projeto","private":true,"permissions":{"admin":false,"maintain":false,"push":true,"triage":true,"pull":true}}`
const aur505CreatedJSON = `{"id":505100,"path":"app.go","line":2}`

func aur505StartFake(t *testing.T, diffBody string) *aur505Fake {
	t.Helper()
	f := &aur505Fake{diffBody: diffBody}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := readAllString(r)
			f.posts = append(f.posts, body)
			w.Header().Set("Content-Type", "application/json")
			if strings.HasPrefix(r.URL.Path, "/repos/dono/projeto/statuses/") {
				var v struct {
					State string `json:"state"`
				}
				_ = json.Unmarshal([]byte(body), &v)
				if v.State == "" {
					w.WriteHeader(http.StatusUnprocessableEntity)
					_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
					return
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"id":505200,"state":%q}`, v.State)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(aur505CreatedJSON))
			return
		}
		switch r.URL.Path {
		case "/repos/dono/projeto":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(aur505WriteJSON))
		case "/repos/dono/projeto/pulls/42":
			if strings.Contains(r.Header.Get("Accept"), "diff") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(f.diffBody))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"number":42,"title":"t","body":"b","head":{"sha":"aur505-unit-head"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func readAllString(r *http.Request) (string, error) {
	b, err := io.ReadAll(r.Body)
	return string(b), err
}

func (f *aur505Fake) joinedPosts() string { return strings.Join(f.posts, "\n") }

func (f *aur505Fake) findingComments() []string {
	var out []string
	for _, p := range f.posts {
		// A published finding comment names a [severity] and is not the
		// review summary (which starts with the HTML marker).
		if strings.Contains(p, "**[") && !strings.Contains(p, "aurumcode-review") {
			out = append(out, p)
		}
	}
	return out
}

func aur505WriteFixture(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return p
}

// aur505SecretDiff adds a hardcoded secret on line 2, which the
// deterministic static-analysis catalog flags as an error. Full context
// lines make it a valid unified diff.
const aur505SecretDiff = "diff --git a/app.go b/app.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/app.go\n" +
	"+++ b/app.go\n" +
	"@@ -1,1 +1,2 @@\n" +
	" package app\n" +
	"+password := \"hunter2-super-secret\"\n"

const aur505CleanDiff = "diff --git a/app.go b/app.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/app.go\n" +
	"+++ b/app.go\n" +
	"@@ -1,1 +1,2 @@\n" +
	" package app\n" +
	"+// apenas um comentario\n"

// aur505Invalid is the invalid model response: the JSON decodes but the
// verdict is not in the accepted enum, so the parser returns
// validation_failed. It carries a sentinel that must never surface as a
// published finding, and no line matches the parser's degraded free-form
// recovery, so it cannot be silently adopted.
const aur505Invalid = `{"verdict":"banana","issues":[],"summary":"AUR505-SENTINEL-INVALID"}`

const aur505ValidEmpty = `{"issues":[],"summary":"Nada a relatar."}`

const aur505ValidFinding = `{
  "issues": [
    {
      "file": "app.go",
      "line": 2,
      "severity": "error",
      "rule_id": "quality/long-function",
      "message": "AUR505-VALID-MODEL-FINDING",
      "impact": "Impacto sintetico da resposta valida.",
      "evidence": "Evidencia sintetica da resposta valida.",
      "verification": "Verificacao sintetica da resposta valida."
    }
  ],
  "summary": "Resposta valida sintetica."
}`

func TestAUR505(t *testing.T) {
	bin := aur505BuildBinary(t)
	workDir := t.TempDir()

	run := func(t *testing.T, diff, fixture string) (int, string, string, *aur505Fake) {
		t.Helper()
		fake := aur505StartFake(t, diff)
		env := []string{
			"AURUMCODE_LLM_FIXTURE=" + fixture,
			"AURUMCODE_GITHUB_API_URL=" + fake.server.URL,
			"GITHUB_TOKEN=token-sintetico-unit",
			"GITHUB_SHA=aur505-github-sha",
		}
		code, stdout, stderr := aur505Run(t, bin, workDir, env,
			"review", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--seguranca", "--check")
		return code, stdout, stderr, fake
	}

	t.Run("AC-001", func(t *testing.T) {
		invalid := aur505WriteFixture(t, aur505Invalid)

		// (a) deterministic error finding present: the exit code must be
		//     the --check failure gate (3), not a blanket 1, and the
		//     deterministic finding must actually be published.
		code, stdout, stderr, fake := run(t, aur505SecretDiff, invalid)
		if code != 3 {
			t.Fatalf("invalid response with a deterministic error finding: want exit 3 (--check failure gate), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		posts := fake.joinedPosts()
		if !strings.Contains(posts, "Hardcoded secret or credential assigned inline") {
			t.Fatalf("deterministic finding was not published; POSTs:\n%s", posts)
		}
		if !strings.Contains(posts, "Quality review inconclusive") {
			t.Fatalf("declared model-failure limitation missing from published review; POSTs:\n%s", posts)
		}
		if !strings.Contains(stderr, "could not understand the model's response (validation_failed)") {
			t.Fatalf("model failure not recorded on stderr:\n%s", stderr)
		}
		if strings.Contains(posts, "AUR505-SENTINEL-INVALID") {
			t.Fatalf("invalid response bytes leaked into the published review:\n%s", posts)
		}

		// (b) deterministic-clean diff: the gate is clean, so the run must
		//     exit 0 -- this is the CI behavior the card fixes. A blanket
		//     1 would fail here.
		code, stdout, stderr, fake = run(t, aur505CleanDiff, invalid)
		if code != 0 {
			t.Fatalf("invalid response with a deterministic-clean diff: want exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		posts = fake.joinedPosts()
		if !strings.Contains(posts, "Quality review inconclusive") {
			t.Fatalf("declared limitation missing on the clean deterministic run; POSTs:\n%s", posts)
		}
	})

	t.Run("AC-002", func(t *testing.T) {
		valid := aur505WriteFixture(t, aur505ValidFinding)
		code, stdout, stderr, fake := run(t, aur505SecretDiff, valid)
		if code != 3 {
			t.Fatalf("valid error finding: want exit 3 (--check failure), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		posts := fake.joinedPosts()
		if !strings.Contains(posts, "AUR505-VALID-MODEL-FINDING") {
			t.Fatalf("valid response finding was not merged/published; POSTs:\n%s", posts)
		}
		if strings.Contains(posts, "Quality review inconclusive") {
			t.Fatalf("valid response wrongly declared the model inconclusive; POSTs:\n%s", posts)
		}
	})

	t.Run("AC-003", func(t *testing.T) {
		invalid := aur505WriteFixture(t, aur505Invalid)
		validEmpty := aur505WriteFixture(t, aur505ValidEmpty)

		_, _, _, invalidFake := run(t, aur505SecretDiff, invalid)
		_, _, _, validFake := run(t, aur505SecretDiff, validEmpty)

		invalidFindings := sortedStrings(invalidFake.findingComments())
		validFindings := sortedStrings(validFake.findingComments())
		if len(invalidFindings) == 0 {
			t.Fatalf("no deterministic finding published on the invalid run; POSTs:\n%s", invalidFake.joinedPosts())
		}
		if strings.Join(invalidFindings, "\x00") != strings.Join(validFindings, "\x00") {
			t.Fatalf("invalid run fabricated a finding: invalid=%q valid=%q\ninvalid POSTs:\n%s",
				invalidFindings, validFindings, invalidFake.joinedPosts())
		}
		if !strings.Contains(invalidFake.joinedPosts(), "Quality review inconclusive") {
			t.Fatalf("failure not recorded on the invalid run; POSTs:\n%s", invalidFake.joinedPosts())
		}
	})
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
