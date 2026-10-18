package main

// The parecer, the formal review event, the aurumcode/review status and the
// exit agree under an explicit --fail-on in the cases a first fix missed: a
// security finding published in its own section under --base, a run without
// a declared gate, and one problem two passes reported on one line.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// failOnPR is what one --pr review published.
type failOnPR struct {
	code     int
	stderr   string
	review   githubclient.PullRequestReview
	statuses map[string]githubclient.CommitStatus
}

// runFailOnPR runs one --pr --check review of a pull request whose diff is
// diffBody, under the repository config cfg ("" for none) and the model
// answer response.
func runFailOnPR(t *testing.T, diffBody, cfg, response string, extra ...string) failOnPR {
	t.Helper()
	got := failOnPR{statuses: map[string]githubclient.CommitStatus{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7" && strings.Contains(r.Header.Get("Accept"), "diff"):
			fmt.Fprint(w, diffBody)
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7":
			fmt.Fprint(w, `{"title":"Add feature","body":"Implements the feature.","head":{"sha":"new-head"},"base":{"sha":"base"}}`)
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7/commits":
			fmt.Fprint(w, `[{"sha":"aaa111","commit":{"message":"feat: add feature"}}]`)
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/contents/.aurumcode/config.yml" && cfg != "":
			fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(cfg)))
		case r.Method == "GET" && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			fmt.Fprint(w, "[]")
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/reviews"):
			if err := json.NewDecoder(r.Body).Decode(&got.review); err != nil {
				t.Error(err)
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"id":9}`)
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/repos/team/project/statuses/"):
			var status githubclient.CommitStatus
			if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
				t.Error(err)
			}
			got.statuses[status.Context] = status
			w.WriteHeader(201)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(response), 0o600); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"AURUMCODE_LLM_FIXTURE": fixture, "AURUMCODE_GITHUB_API_URL": server.URL, "AURUMCODE_PR_PERMISSION_MODE": "endpoint",
		"GITHUB_SHA": "new-head", "AURUMCODE_BASE_SHA": "base", "AURUMCODE_CI_CONTEXT_FILE": "", "AURUMCODE_OUTPUT_FILE": "",
		"AURUMCODE_POLICY": "", "XDG_CACHE_HOME": t.TempDir(), "AURUMCODE_CACHE_DIR": t.TempDir(),
	} {
		t.Setenv(k, v)
	}
	args := append([]string{"--pr", "7", "--repo", "team/project", "--publicar", "--modo-publicacao", "review", "--check"}, extra...)
	var stdout, stderr strings.Builder
	got.code = runReviewWith(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter(), deps: reviewDeps{}}, args)
	got.stderr = stderr.String()
	return got
}

// Under --base the security pass's findings are published in their own
// section, never among the issues the headline reads. With the policy gate
// passing and --fail-on error catching only the security pass's error, the
// parecer reads blocked by that one problem, not "inconclusive: the gate
// failed without a finding", and still says the policy gate passed.
func TestParecerCountsTheSecurityFindingFailOnCaught(t *testing.T) {
	aur579Repo(t)
	writeRepoConfig(t, "review:\n  language: pt-BR\ngate:\n  fail_on: [error]\n  inconclusive: warn\n  triage:\n    analysis: none\n    sast: none\n  sources: [skills]\nrules:\n  analysis/hardcoded-secret:\n    severity: warning\n")
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"Nada a apontar.","issues":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	code, out, errOut := aur579Review(t, scanner.Command(nil), "--seguranca", "--fail-on", "error")
	if code != exitFindings {
		t.Fatalf("exit=%d, want %d (--fail-on error catches the security pass's error)\nstdout=%s\nstderr=%s", code, exitFindings, out, errOut)
	}
	if !strings.Contains(errOut, "1 finding(s) at severity error") && !strings.Contains(errOut, "1 achado") {
		t.Fatalf("the threshold must catch exactly the security finding:\n%s", errOut)
	}
	if !strings.Contains(out, "Bloqueado: 1 problema precisa de correção") {
		t.Fatalf("the parecer must read blocked by the one caught finding:\n%s", out)
	}
	if strings.Contains(out, "Inconclusivo") {
		t.Fatalf("a threshold catch is not a gate failure without a finding:\n%s", out)
	}
	if !strings.Contains(out, "gate da política: passou") {
		t.Fatalf("the policy gate itself passed and the parecer must say so:\n%s", out)
	}
}

// Without a declared gate an explicit --fail-on info that catches an info
// finding blocks everywhere: the headline, the formal review event, the
// aurumcode/review status and the exit.
func TestFailOnWithoutAGateBlocksEverywhere(t *testing.T) {
	const diffBody = "diff --git a/app.go b/app.go\n@@ -1,1 +1,1 @@\n-old := 1\n+new := 2\n"
	const response = `{"summary":"Uma observação.","issues":[{"file":"app.go","line":1,"severity":"info","rule_id":"quality/magic-numbers","message":"Número mágico sem nome.","impact":"Leitura difícil.","evidence":"A linha adicionada usa o literal direto.","verification":"Dê nome à constante."}]}`
	got := runFailOnPR(t, diffBody, "", response, "--fail-on", "info")
	if got.code != exitFindings {
		t.Fatalf("exit=%d, want %d\nstderr=%s\nbody=%s", got.code, exitFindings, got.stderr, got.review.Body)
	}
	if !strings.Contains(got.review.Body, "Blocked: 1 problem must be fixed") {
		t.Fatalf("the parecer must read blocked by the caught info:\n%s", got.review.Body)
	}
	if got.review.Event != "REQUEST_CHANGES" {
		t.Fatalf("formal event=%q, want REQUEST_CHANGES under the --fail-on catch", got.review.Event)
	}
	status, ok := got.statuses[checkContext]
	if !ok || status.State != "failure" {
		t.Fatalf("the %s status must fail with the exit: %+v (all: %+v)", checkContext, status, got.statuses)
	}
}

// One problem two passes reported on one line (the model's and the static
// analysis' secret) is published, and counted, once. With the policy gate
// passing and --fail-on error catching both problems, the headline counts
// two, as the list shows two.
func TestFailOnCountsAnAbsorbedProblemOnce(t *testing.T) {
	const diffBody = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n func Run() {\n+\tdbPassword := \"hunter2-correct-horse\"\n+\ttimeout := 86400\n"
	const response = `{"summary":"Dois problemas.","issues":[` +
		`{"file":"app.go","line":3,"severity":"error","rule_id":"security/hardcoded-secret","message":"Senha escrita no código.","impact":"Vazamento.","evidence":"A linha adicionada atribui a senha literal.","verification":"Leia a senha do ambiente."},` +
		`{"file":"app.go","line":4,"severity":"error","rule_id":"quality/magic-numbers","message":"Número mágico sem nome.","impact":"Leitura difícil.","evidence":"A linha adicionada usa o literal direto.","verification":"Dê nome à constante."}]}`
	const cfg = "gate:\n  fail_on: [error]\n  inconclusive: warn\n  triage:\n    analysis: none\n    sast: none\n  sources: [skills]\n"
	got := runFailOnPR(t, diffBody, cfg, response, "--fail-on", "error")
	if got.code != exitFindings {
		t.Fatalf("exit=%d, want %d\nstderr=%s\nbody=%s", got.code, exitFindings, got.stderr, got.review.Body)
	}
	body := got.review.Body
	if !strings.Contains(body, "policy gate: passed") {
		t.Fatalf("the policy gate itself passed:\n%s", body)
	}
	if !strings.Contains(body, "Blocked: 2 problems must be fixed") {
		t.Fatalf("the headline must count the two listed problems:\n%s", body)
	}
	if !strings.Contains(body, "consolidation") && !strings.Contains(body, "published once") {
		t.Fatalf("the secret two passes reported must be published inside one item:\n%s", body)
	}
	fix := body[strings.Index(body, "### Fix before merge"):strings.Index(body, "### Summary")]
	for _, at := range []string{"**`app.go:3`**", "**`app.go:4`**"} {
		if strings.Count(fix, at) != 1 {
			t.Fatalf("want %s listed once as blocking:\n%s", at, body)
		}
	}
}
