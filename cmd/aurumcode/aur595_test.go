package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur595MergeSHA is the synthetic merge commit GitHub puts in GITHUB_SHA
// on a pull_request event: a full commit id the checkout of the pull
// request head does not contain.
const aur595MergeSHA = "a1ee8f28087b656a18e24619716ef0f14ab831f7"

const aur595Config = "gate:\n  fail_on: [error]\n  inconclusive: block\nquality_gates:\n  scanners:\n    - engine: gitleaks\n      required: true\n"

const aur595Diff = "diff --git a/app.go b/app.go\n@@ -1,1 +1,2 @@\n package demo\n+func Change() {}\n"

// aur595Checkout builds, with the real git, the checkout a pull_request job
// has: origin owner/repo, a base commit and the pull request head on top of
// it, clean, as the working directory. It returns the base and head ids.
func aur595Checkout(t *testing.T) (base, head string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("remote", "add", "origin", "https://github.com/owner/repo.git")
	write("package demo\n")
	git("add", "app.go")
	git("commit", "-q", "-m", "base")
	base = git("rev-parse", "HEAD")
	write("package demo\nfunc Change() {}\n")
	git("commit", "-q", "-am", "head")
	head = git("rev-parse", "HEAD")
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Cleanup(chdir(t, dir))
	return base, head
}

// aur595Gitleaks puts a fake gitleaks first on PATH: the pinned version,
// and a scan that records its --log-opts range and writes an empty report.
// stderrLine, when set, makes the scan fail with that error output. The
// range is never checked here: the engine's own verifyRange runs the real
// git against the checkout first.
func aur595Gitleaks(t *testing.T, stderrLine string) (record string) {
	t.Helper()
	bin := t.TempDir()
	record = filepath.Join(bin, "range.txt")
	fail := ""
	if stderrLine != "" {
		fail = fmt.Sprintf("printf '%%s\\n' %s >&2; exit 2\n", shellQuote(stderrLine))
	}
	script := "#!/usr/bin/env bash\n" +
		"if [ \"$1\" = version ]; then echo v8.30.1; exit 0; fi\n" + fail +
		"for a in \"$@\"; do case \"$a\" in --log-opts=*) printf '%s\\n' \"${a#--log-opts=}\" > " + shellQuote(record) + ";; esac; done\n" +
		"while [ $# -gt 0 ]; do if [ \"$1\" = --report-path ]; then printf '[]' > \"$2\"; fi; shift; done\n"
	writeExec(t, filepath.Join(bin, "gitleaks"), script)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
}

// aur595Server is the GitHub API of pull request 48 whose head is headSHA:
// the diff, the configuration from the base, and the posted review body.
func aur595Server(t *testing.T, headSHA string, posted *string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(aur595Diff))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprintf(w, `{"head":{"sha":%q}}`, headSHA)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(aur595Config)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			var payload struct {
				Body string `json:"body"`
			}
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			_ = json.Unmarshal(buf.Bytes(), &payload)
			*posted = payload.Body
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// aur595Review runs --pr 48 as the review workflow does: GITHUB_SHA is the
// merge commit, AURUMCODE_BASE_SHA the given base. It returns the exit
// code, stderr, the posted body and the audit's gate reason.
func aur595Review(t *testing.T, base, head string) (code int, stderr, posted, auditReason string) {
	t.Helper()
	server := aur595Server(t, head, &posted)
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"reviewed","issues":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("GITHUB_SHA", aur595MergeSHA)
	t.Setenv("AURUMCODE_BASE_SHA", base)
	audit := filepath.Join(t.TempDir(), "audit.json")
	var out, errOut strings.Builder
	code = runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true,
		publicationSet: true, publication: "review", auditoriaPath: audit})
	var rec aur580Audit
	readJSON(t, audit, &rec)
	return code, errOut.String(), posted, rec.Gate.Reason
}

// AC-002: GITHUB_SHA names the merge commit the checkout does not hold;
// the engine still scans the pull request's own base..head and concludes.
func TestAUR595GitleaksScansThePullRequestHeadNotTheMergeCommit(t *testing.T) {
	base, head := aur595Checkout(t)
	record := aur595Gitleaks(t, "")
	code, stderr, _, reason := aur595Review(t, base, head)
	scanned, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("gitleaks never scanned (exit=%d): %v\nstderr=%s", code, err, stderr)
	}
	if got := strings.TrimSpace(string(scanned)); got != base+".."+head {
		t.Fatalf("gitleaks scanned %q, want the pull request range %s..%s", got, base, head)
	}
	if code != 0 || strings.Contains(stderr, "secrets_execution_error") || strings.Contains(reason, "inconclusivo") {
		t.Fatalf("exit=%d audit reason=%q, want a concluded scan; stderr=%s", code, reason, stderr)
	}
}

// AC-002/AC-003: a range end really absent from the checkout stays
// inconclusive, and the motive says which end and why, in the gate line,
// the audit and the published review.
func TestAUR595AbsentBaseIsInconclusiveWithItsDetail(t *testing.T) {
	_, head := aur595Checkout(t)
	aur595Gitleaks(t, "")
	missing := strings.Repeat("0", 39) + "9"
	code, stderr, posted, reason := aur595Review(t, missing, head)
	detail := "base commit " + missing + " not found in the checkout"
	if code == 0 || !strings.Contains(stderr, "inconclusivo (secrets_execution_error) [detalhe: ") || !strings.Contains(stderr, detail) {
		t.Fatalf("exit=%d, want an inconclusive gate line naming the absent base; stderr=%s", code, stderr)
	}
	if !strings.Contains(reason, detail) {
		t.Fatalf("audit gate reason %q does not carry the detail", reason)
	}
	if !strings.Contains(posted, "secrets_execution_error") || !strings.Contains(posted, detail) {
		t.Fatalf("published review does not carry the detail:\n%s", posted)
	}
}

// AC-003: an engine failure exposes its own error line, summarized and
// redacted: a credential the engine printed never reaches any output.
func TestAUR595EngineFailureDetailIsRedacted(t *testing.T) {
	base, head := aur595Checkout(t)
	credential := "ghp_" + strings.Repeat("Zq9", 12)
	aur595Gitleaks(t, "fatal: cannot read pack for token "+credential)
	code, stderr, posted, reason := aur595Review(t, base, head)
	for name, text := range map[string]string{"stderr": stderr, "audit": reason, "review": posted} {
		if strings.Contains(text, credential) {
			t.Fatalf("%s carries the credential the engine printed:\n%s", name, text)
		}
		if !strings.Contains(text, "gitleaks: execution failed: exit status 2: fatal: cannot read pack for token "+redaction.Marker) {
			t.Fatalf("%s does not carry the redacted engine detail:\n%s", name, text)
		}
	}
	if code == 0 {
		t.Fatalf("exit=0 for a failed required scan; stderr=%s", stderr)
	}
}

// AC-001 end to end: a base prompt above max_cost_tokens, the model asks
// for no tool, the review concludes; the audit shows the base prompt apart
// from the deliberation's own cost.
func TestAUR595BasePromptAboveTheCeilingConcludes(t *testing.T) {
	aur580Setup(t, strings.Replace(aur580Config, "max_rounds: 3", "max_rounds: 3\n  max_cost_tokens: 300", 1), "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"lines_above":50,"tool":"scanner_fakescan"}]`))
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, _, errOut := aur579Review(t, nil, "--auditoria", audit)
	var rec struct {
		Deliberation *struct {
			Outcome    string `json:"outcome"`
			BaseTokens int    `json:"base_tokens"`
			CostTokens int    `json:"cost_tokens"`
		} `json:"deliberation"`
	}
	readJSON(t, audit, &rec)
	if code != 0 || strings.Contains(errOut, "deliberation_limit") || rec.Deliberation == nil || rec.Deliberation.Outcome != "answered" {
		t.Fatalf("exit=%d deliberation=%+v, want an answered deliberation; stderr=%s", code, rec.Deliberation, errOut)
	}
	if d := rec.Deliberation; d.BaseTokens <= 300 || d.CostTokens > 300 {
		t.Fatalf("base_tokens=%d cost_tokens=%d, want a base prompt above the 300 ceiling and a cost within it", d.BaseTokens, d.CostTokens)
	}
}
