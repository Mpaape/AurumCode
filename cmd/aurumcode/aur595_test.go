package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur595MergeSHA is the synthetic merge commit GitHub puts in GITHUB_SHA
// on a pull_request event: a full commit id the checkout of the pull
// request head does not contain.
const aur595MergeSHA = "a1ee8f28087b656a18e24619716ef0f14ab831f7"

const aur595Config = "gate:\n  fail_on: [error]\n  inconclusive: block\nquality_gates:\n  scanners:\n    - engine: gitleaks\n      required: true\n"

const aur595Diff = "diff --git a/app.go b/app.go\n@@ -1,1 +1,2 @@\n package demo\n+func Change() {}\n"

// aur595Checkout builds the checkout a pull_request job has, without a git
// binary (the sealed profile has none): origin owner/repo, a base commit
// and the pull request head on top of it as loose objects, clean, as the
// working directory. It returns the base and head ids.
func aur595Checkout(t *testing.T) (base, head string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(content, parent, message string) string {
		t.Helper()
		blob := gitObject(t, dir, "blob", []byte(content))
		tree := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", blob))
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\n" + message + "\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	base = commit("package demo\n", "", "base")
	headContent := "package demo\nfunc Change() {}\n"
	head = commit(headContent, base, "head")
	write(".git/HEAD", "ref: refs/heads/main\n")
	write(".git/refs/heads/main", head+"\n")
	write(".git/config", "[core]\n\trepositoryformatversion = 0\n\tbare = false\n[remote \"origin\"]\n\turl = https://github.com/owner/repo.git\n")
	write("app.go", headContent)
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Cleanup(chdir(t, dir))
	return base, head
}

// aur595Runner is the scanner command runner of these tests. git answers
// from the checkout's own object store (a commit is present only when its
// object is), so the engine's verifyRange decides on the real repository;
// gitleaks reports the pinned version and, for a scan, records its
// --log-opts range and writes an empty report, or fails with stderrLine.
type aur595Runner struct {
	stderrLine string
	scanned    *string
}

func (r aur595Runner) run(_ context.Context, dir, binary string, args ...string) (string, string, error) {
	switch {
	case binary == "git" && len(args) == 2 && args[0] == "rev-parse" && args[1] == "--is-shallow-repository":
		if _, err := os.Stat(filepath.Join(dir, ".git", "shallow")); err == nil {
			return "true\n", "", nil
		}
		return "false\n", "", nil
	case binary == "git" && len(args) == 3 && args[0] == "cat-file" && args[1] == "-e":
		id := strings.TrimSuffix(args[2], "^{commit}")
		if len(id) == 40 {
			if _, err := os.Stat(filepath.Join(dir, ".git", "objects", id[:2], id[2:])); err == nil {
				return "", "", nil
			}
		}
		return "", "fatal: Not a valid object name " + args[2] + "\n", errors.New("exit status 128")
	case binary == "gitleaks" && len(args) == 1 && args[0] == "version":
		return "v8.30.1\n", "", nil
	case binary == "gitleaks" && r.stderrLine != "":
		return "", r.stderrLine + "\n", errors.New("exit status 2")
	case binary == "gitleaks":
		for i, a := range args {
			if strings.HasPrefix(a, "--log-opts=") {
				*r.scanned = strings.TrimPrefix(a, "--log-opts=")
			}
			if a == "--report-path" && i+1 < len(args) {
				if err := os.WriteFile(args[i+1], []byte("[]"), 0o600); err != nil {
					return "", "", err
				}
			}
		}
		return "", "", nil
	}
	return "", "", fmt.Errorf("unexpected command %s %v", binary, args)
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
func aur595Review(t *testing.T, base, head string, runner aur595Runner) (code int, stderr, posted, auditReason string) {
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
	deps := reviewDeps{scanners: scanner.Executor{Command: runner.run}}
	code = runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: deps}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true,
		publicationSet: true, publication: "review", auditoriaPath: audit})
	var rec aur580Audit
	readJSON(t, audit, &rec)
	return code, errOut.String(), posted, rec.Gate.Reason
}

// AC-002: GITHUB_SHA names the merge commit the checkout does not hold;
// the engine still scans the pull request's own base..head and concludes.
func TestAUR595GitleaksScansThePullRequestHeadNotTheMergeCommit(t *testing.T) {
	base, head := aur595Checkout(t)
	var scanned string
	code, stderr, _, reason := aur595Review(t, base, head, aur595Runner{scanned: &scanned})
	if scanned == "" {
		t.Fatalf("gitleaks never scanned (exit=%d)\nstderr=%s", code, stderr)
	}
	if got := scanned; got != base+".."+head {
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
	var scanned string
	missing := strings.Repeat("0", 39) + "9"
	code, stderr, posted, reason := aur595Review(t, missing, head, aur595Runner{scanned: &scanned})
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
	var scanned string
	code, stderr, posted, reason := aur595Review(t, base, head, aur595Runner{scanned: &scanned, stderrLine: "fatal: cannot read pack for token " + credential})
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

// AC-003: an auth header the engine printed as its last error line is
// redacted in every output, though the detail is joined into one line.
func TestAUR595EngineHeaderLineIsRedacted(t *testing.T) {
	base, head := aur595Checkout(t)
	secret := "tok" + strings.Repeat("8", 14)
	var scanned string
	_, stderr, posted, reason := aur595Review(t, base, head, aur595Runner{scanned: &scanned, stderrLine: "Authorization: Bearer " + secret})
	for name, text := range map[string]string{"stderr": stderr, "audit": reason, "review": posted} {
		if strings.Contains(text, secret) {
			t.Fatalf("%s carries the header value:\n%s", name, text)
		}
		if !strings.Contains(text, "gitleaks: execution failed: exit status 2: Authorization: "+redaction.Marker) {
			t.Fatalf("%s does not carry the redacted header detail:\n%s", name, text)
		}
	}
}
