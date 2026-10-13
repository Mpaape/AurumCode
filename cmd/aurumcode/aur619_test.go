package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const aur619Denied = `{"message":"Resource not accessible by integration","documentation_url":"https://docs.github.com/rest","status":"403"}`

// TestAUR619PermissionDeniedNamesThePermission: a refused credential names
// the permission of the action and keeps GitHub's answer, in both
// languages; any other error is the same text as before.
func TestAUR619PermissionDeniedNamesThePermission(t *testing.T) {
	refused := &githubclient.APIError{Status: http.StatusForbidden, Body: aur619Denied}
	cases := []struct {
		language string
		action   publishAction
		want     []string
	}{
		{"pt-BR", publishComment, []string{"sem permissão para publicar", "`pull-requests: write`", "`permissions:` do job que chama o workflow", "(HTTP 403: " + aur619Denied + ")"}},
		{"pt-BR", publishStatus, []string{"sem permissão para publicar", "`statuses: write`", "(HTTP 403: " + aur619Denied + ")"}},
		{"", publishComment, []string{"permission denied: the token needs `pull-requests: write`", "`permissions:` of the job that calls the workflow", "(HTTP 403: " + aur619Denied + ")"}},
		{"en", publishStatus, []string{"permission denied: the token needs `statuses: write`", "(HTTP 403: " + aur619Denied + ")"}},
	}
	for _, tc := range cases {
		got := publishError(tc.language, tc.action, refused)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("%s/%d: %q lacks %q", tc.language, tc.action, got, want)
			}
		}
	}
	unauthorized := &githubclient.APIError{Status: http.StatusUnauthorized, Body: `{"message":"Bad credentials"}`}
	if got := publishError("pt-BR", publishStatus, unauthorized); !strings.Contains(got, "`statuses: write`") || !strings.Contains(got, "HTTP 401: ") {
		t.Fatalf("401 is a refused credential too: %q", got)
	}
	for _, other := range []error{&githubclient.APIError{Status: http.StatusUnprocessableEntity, Body: "x"}, errors.New("dial tcp: connection refused")} {
		if got := publishError("pt-BR", publishComment, other); got != other.Error() {
			t.Fatalf("other errors keep their text: %q vs %q", got, other.Error())
		}
	}
}

// deniedGitHub is a GitHub that serves every read and refuses every write
// with the answer a workflow token gets without the permission.
func deniedGitHub(t *testing.T, diffBody, repoConfig string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(repoConfig)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost || r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(aur619Denied))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

// TestAUR619PublishingDeniedReachesTheOperator (AC-001, AC-002): on a pull
// request whose token cannot write, every refused publication -- the
// inline comment, the parecer, the review status and the policy-gate
// status -- tells the operator which permission is missing, keeps GitHub's
// answer, and the run still exits as a failure.
func TestAUR619PublishingDeniedReachesTheOperator(t *testing.T) {
	// The blocking finding is the embedded catalog's own (a world-writable
	// mode on the changed line), so the proof needs no model answer.
	diff := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change(p string) {\n+ _ = os.Chmod(p, 0777)\n+}\n"
	resp := `{"summary":"ok","verdict":"approve","issues":[]}`
	cases := []struct{ name, config, denied string }{
		{"pt-BR", "review:\n  language: pt-BR\ngate:\n  fail_on: [warning]\n", "sem permissão para publicar: o token precisa de `"},
		{"en", "gate:\n  fail_on: [warning]\n", "permission denied: the token needs `"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := chdir(t, t.TempDir())
			defer restore()
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
			server := deniedGitHub(t, diff, tc.config)
			defer server.Close()
			fixture := filepath.Join(t.TempDir(), "response.json")
			if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
				t.Fatal(err)
			}
			setPRGateEnv(t, server, fixture)
			t.Setenv("GITHUB_TOKEN", "test-token")

			var stdout, stderr strings.Builder
			code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true, publicationSet: true, publication: "comments"})
			if code == 0 {
				t.Fatalf("a run that published nothing must not exit 0; stdout=%s stderr=%s", stdout.String(), stderr.String())
			}
			out := stderr.String()
			for _, want := range []string{
				"publishing inline comment on app.go:3: " + tc.denied + "pull-requests: write`",
				"publishing review summary: " + tc.denied + "pull-requests: write`",
				"publishing check status: " + tc.denied + "statuses: write`",
				"publishing policy gate status: " + tc.denied + "statuses: write`",
				"(HTTP 403: " + aur619Denied + ")",
				"summary: " + tc.denied + "pull-requests: write`",
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("stderr lacks %q:\n%s\nstdout:\n%s", want, out, stdout.String())
				}
			}
		})
	}
}
