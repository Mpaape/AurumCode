package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// The analysts a repository selects in review.profiles review the pull
// request too: one model pass per profile, the team's own profile read
// from .aurumcode/profiles.yml at the base ref, and each finding names the
// analyst that found it in the published parecer.
func TestPRReviewRunsTheRepositoryAnalystProfiles(t *testing.T) {
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,5 @@\n package demo\n+func Run(cmd string) {\n+ exec(cmd)\n+ _ = 1\n+}\n"
	config := "review:\n  profiles: [seguranca, qa]\ngate:\n  fail_on: [error]\n"
	team := "profiles:\n  - name: qa\n    version: \"1\"\n    emphasis: \"tests and regressions\"\n    families: [\"quality\"]\n    instructions: \"QA-PROFILE: avalie se o caminho alterado tem teste.\"\n"
	var reviewBody string
	var teamRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = w.Write([]byte(`{"head":{"sha":"head-sha"}}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/contents/.aurumcode/config.yml"):
			_, _ = w.Write([]byte(`{"content":"` + b64(config) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/contents/.aurumcode/profiles.yml"):
			teamRequests++
			if r.URL.Query().Get("ref") != "base" {
				t.Errorf("team profiles read at ref %q, want the base ref", r.URL.Query().Get("ref"))
			}
			_, _ = w.Write([]byte(`{"content":"` + b64(team) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			raw, _ := io.ReadAll(r.Body)
			reviewBody = string(raw)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/statuses/"):
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	// Each analyst answers its own pass: the security profile's
	// instructions and the team profile's marker select the case.
	fixture := filepath.Join(t.TempDir(), "response.json")
	security := `{"verdict":"changes_requested","summary":"s","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security/command-injection","message":"Command runs caller input.","impact":"Arbitrary commands.","evidence":"Line 3 calls exec(cmd).","suggestion":"Use an allow-list.","verification":"Rerun."}]}`
	qa := `{"verdict":"comment","summary":"q","issues":[{"file":"app.go","line":2,"severity":"warning","rule_id":"quality/missing-error-handling","message":"Run has no test.","impact":"Regressions pass.","evidence":"No test covers Run.","suggestion":"Add a test.","verification":"Rerun."}]}`
	envelope := `{"aurumcode_fixture":{"cases":[` +
		`{"prompt_contains":"AURUMCODE-VERIFICACAO-DE-ACHADO","response":{"verdict":"confirmed","reason":"exec runs it","quote":"exec(cmd)"}},` +
		`{"prompt_contains":"QA-PROFILE","response":` + qa + `},` +
		`{"prompt_contains":"Priorize segredos embutidos","response":` + security + `}],` +
		`"default":{"verdict":"approve","summary":"none","issues":[]}}}`
	if err := os.WriteFile(fixture, []byte(envelope), 0600); err != nil {
		t.Fatal(err)
	}
	setPRGateEnv(t, server, fixture)
	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, check: true,
		publicationSet: true, publication: "review",
	})
	// A model finding that cites only the built-in catalog guides, it does
	// not block (only a team or policy rule decides): both analysts' findings
	// are published as observations, each naming its analyst.
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stderr=%s", code, stderr.String())
	}
	if teamRequests == 0 {
		t.Fatalf("the team profile file was never read from the base ref; stderr=%s", stderr.String())
	}
	for _, want := range []string{"Approved with 2 observations", "[profile seguranca]", "[profile qa]", "security/command-injection", "quality/missing-error-handling"} {
		if !strings.Contains(reviewBody, want) {
			t.Errorf("published parecer lacks %q:\n%s", want, reviewBody)
		}
	}
}
