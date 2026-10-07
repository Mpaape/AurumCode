package main

// A large pull request is reviewed: when the API refuses the diff for its
// size, the same range is read from the verified checkout; an unverified
// checkout is still refused.

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gittest"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur594HeadMarker exists only in a file the pull request adds: it reaches
// the prompt only if the review read the pull request's real diff.
const aur594HeadMarker = "AUR594OnlyInThePullRequestHead"

// aur594TooLargeBody is GitHub's answer to a diff above its line limit.
const aur594TooLargeBody = `{"message":"Sorry, the diff exceeded the maximum number of lines (20000)","errors":[{"resource":"PullRequest","field":"diff","code":"too_large"}],"status":"406"}`

// aur594Checkout builds a real repository with git: main (the base) and a
// head commit that edits app.go and adds lib/extra.go. origin names
// owner/repo. It returns the directory, the base and the head commits.
func aur594Checkout(t *testing.T) (dir, base, head string) {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir = t.TempDir()
	home := t.TempDir()
	for _, kv := range gittest.HermeticEnv(home) {
		if k, v, ok := strings.Cut(kv, "="); ok {
			t.Setenv(k, v)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(gitBin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("app.go", "package demo\n\nfunc Base() int {\n\treturn 1\n}\n")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	base = git("rev-parse", "HEAD")
	write("app.go", "package demo\n\nfunc Base() int {\n\treturn 2\n}\n")
	write("lib/extra.go", "package lib\n\nfunc "+aur594HeadMarker+"() {}\n")
	git("add", ".")
	git("commit", "-q", "-m", "head")
	head = git("rev-parse", "HEAD")
	git("remote", "add", "origin", "https://github.com/owner/repo.git")
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Cleanup(chdir(t, dir))
	return dir, base, head
}

// aur594Server is a GitHub that refuses the diff as too large and reports
// the pull request's base and head commits.
func aur594Server(t *testing.T, base, head string, posted *struct{ Body string }) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = w.Write([]byte(aur594TooLargeBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprintf(w, `{"head":{"sha":%q},"base":{"sha":%q}}`, head, base)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			posted.Body = buf.String()
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// runAUR594PR runs --pr 48 against the server and returns what it did.
func runAUR594PR(t *testing.T, base, head string) (code int, stderr, prompt, posted string) {
	t.Helper()
	var body struct{ Body string }
	server := aur594Server(t, base, head, &body)
	capturePath := aur515Env(t, server.URL)
	var out, errOut strings.Builder
	code = runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true,
		publicationSet: true, publication: "review"})
	captured, _ := os.ReadFile(capturePath)
	return code, out.String() + errOut.String(), string(captured), body.Body
}

// AC-001: the API refuses the diff (406 too_large); the verified checkout
// yields the same base...head range and the review reads it.
func TestTooLargeDiffIsReadFromTheVerifiedCheckout(t *testing.T) {
	_, base, head := aur594Checkout(t)
	code, stderr, prompt, posted := runAUR594PR(t, base, head)
	if code != 0 {
		t.Fatalf("exit=%d, want 0: the verified checkout's diff must be reviewed\n%s", code, stderr)
	}
	for _, want := range []string{aur594HeadMarker, "lib/extra.go", "+\treturn 2"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("the prompt does not carry %q: the review did not read the pull request's diff\n%s", want, prompt)
		}
	}
	if !strings.Contains(stderr, "computed from the verified checkout") || posted == "" {
		t.Fatalf("the review must declare the local diff and publish (posted=%q)\n%s", posted, stderr)
	}
}

// AC-001, the refusal half: a checkout that is not verified (an untracked
// file) never stands in for the API's diff; nothing is published.
func TestTooLargeDiffWithUnverifiedCheckoutFails(t *testing.T) {
	dir, base, head := aur594Checkout(t)
	if err := os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stderr, prompt, posted := runAUR594PR(t, base, head)
	if code != 1 || posted != "" || prompt != "" {
		t.Fatalf("exit=%d posted=%q prompt=%d bytes, want exit 1 and nothing sent or published\n%s", code, posted, len(prompt), stderr)
	}
	if !strings.Contains(stderr, "not verified") {
		t.Fatalf("the failure must name the unverified checkout\n%s", stderr)
	}
}
