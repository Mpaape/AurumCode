package main

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
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur453Fixture runs a local --base review with a controlled model response and
// returns the exit code, stdout and stderr. It is the terminal half of AC-001:
// the user who never opens a pull request still receives the complete summary
// and suggestions, with an eligible replacement marked as applicable.
func aur453BaseReview(t *testing.T, response string, memory bool) (int, string, string) {
	t.Helper()
	localPassFixture(t, memory)
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	return localPassReview()
}

// TestAUR453TerminalSummaryAndSuggestion covers AC-001 on the terminal path: a
// valid summary and an eligible suggestion reach stdout with file, range,
// current code and proposed code, and the summary block is present.
func TestAUR453TerminalSummaryAndSuggestion(t *testing.T) {
	response := `{"summary":"A mudança adiciona um segredo em texto claro.","suggestions":[{"title":"Remover o literal","description":"Use uma variável de ambiente.","kind":"code","file":"app.go","start_line":3,"end_line":3,"current_code":"dbPassword := \"hunter2-super-secret\"","proposed_code":"dbPassword := os.Getenv(\"DB_PASSWORD\")","rationale":"Evita segredo no controle de versão.","verification":"Rode o teste do pacote."}],"issues":[]}`
	code, out, errOut := aur453BaseReview(t, response, false)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{
		"## Code Review Summary",
		"### Suggestions",
		"Remover o literal",
		"Use uma variável de ambiente.",
		"app.go:3",
		"os.Getenv",
		"Applicable replacement",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("terminal report missing %q:\n%s", want, out)
		}
	}
}

// TestAUR453TerminalIneligibleSuggestion covers AC-002 on the terminal path: a
// suggestion whose range is outside the added lines, whose current code
// diverges, or that has no location is never presented as an applicable
// substitution; the text explains the limitation and no patch is applied.
func TestAUR453TerminalIneligibleSuggestion(t *testing.T) {
	response := `{"summary":"Observação.","suggestions":[{"title":"Fora do diff","description":"Não aparece no diff adicionado.","kind":"code","file":"app.go","start_line":99,"end_line":99,"current_code":"nope","proposed_code":"outra coisa"},{"title":"Sem código proposto","description":"Apenas orientação.","kind":"general","file":"app.go","start_line":3,"end_line":3}],"issues":[]}`
	code, out, errOut := aur453BaseReview(t, response, false)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	if strings.Contains(out, "Applicable replacement") {
		t.Fatalf("ineligible suggestion presented as applicable:\n%s", out)
	}
	if !strings.Contains(out, "no applicable replacement") {
		t.Fatalf("terminal report did not declare the limitation:\n%s", out)
	}
	// Every ineligible suggestion's title still reaches the reader: it is shown
	// as guidance, not silently dropped.
	for _, want := range []string{"Fora do diff", "Sem código proposto"} {
		if !strings.Contains(out, want) {
			t.Fatalf("guidance suggestion %q dropped:\n%s", want, out)
		}
	}
}

// TestAUR453FormalReviewCarriesSuggestion covers AC-001 on the formal review
// path: the posted review bodies carry the finding and the native GitHub
// suggestion with the correct file, line range, current code and proposed code.
func TestAUR453FormalReviewCarriesSuggestion(t *testing.T) {
	fixture := `{"summary":"A mudança precisa de uma substituição.","issues":[{"file":"main.go","line":2,"severity":"warning","rule_id":"quality/style","message":"Linha longa.","impact":"Leitura difícil.","evidence":"A linha adicionada concentra tudo.","verification":"Rode o formatador."}],"suggestions":[{"title":"Quebrar a linha","description":"Extraia a chamada.","kind":"code","file":"main.go","start_line":2,"end_line":2,"current_code":"func main() { run(); persist() }","proposed_code":"func main() {\n\trun()\n\tpersist()\n}","rationale":"Melhora a leitura."}]}`

	var posted githubclient.PullRequestReview
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42":
			w.Header().Set("ETag", `"aur453"`)
			_, _ = w.Write([]byte("diff --git a/main.go b/main.go\n@@ -1,1 +1,3 @@\n package main\n+func main() { run(); persist() }\n+// trailing\n"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/pulls/42/reviews":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Fatalf("decoding formal review: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	fixturePath := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixturePath, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixturePath)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 42, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(posted.Body, "### Suggestions") || !strings.Contains(posted.Body, "Quebrar a linha") {
		t.Fatalf("formal review body lost the suggestion:\n%s", posted.Body)
	}
	foundNative := false
	for _, c := range posted.Comments {
		if c.Path == "main.go" && c.Line == 2 && strings.Contains(c.Body, "```suggestion") {
			foundNative = true
			if !strings.Contains(c.Body, "persist()") {
				t.Fatalf("native suggestion lost the proposed code:\n%s", c.Body)
			}
		}
	}
	if !foundNative {
		t.Fatalf("no native suggestion comment at main.go:2:\n%+v", posted.Comments)
	}
}

// TestAUR453RedactionAndConsumerChoice covers AC-003: the consumer's language
// and configuration choice survives publication, a canary secret in a
// suggestion is redacted at the sink, and a review with no suggestions is
// unchanged (no empty section).
func TestAUR453RedactionAndConsumerChoice(t *testing.T) {
	// Consumer choice: a pt-BR config drives the formal review language.
	head := base64.StdEncoding.EncodeToString([]byte("review:\n  language: pt-BR\n  changelog: false\n"))
	var posted githubclient.PullRequestReview
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/9":
			_, _ = w.Write([]byte("diff --git a/app.go b/app.go\n@@ -1,1 +1,2 @@\n package app\n+func run() {}\n"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, head)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Fatalf("decoding formal review: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	secret := "CANARY-AUR453-" + "SECRET-TOKEN"
	fixturePath := filepath.Join(t.TempDir(), "response.json")
	fixture := `{"summary":"Ok.","suggestions":[{"title":"Segredo","description":"Valor ` + secret + ` no texto.","kind":"code","file":"app.go","start_line":2,"end_line":2,"current_code":"func run() {}","proposed_code":"func run() { /* ` + secret + ` */ }"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixturePath)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("AURUM_SECRET_CANARY", secret)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.FromEnv()}, prReviewOptions{prNumber: 9, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(posted.Body, secret) {
		t.Fatalf("canary secret leaked into the formal review:\n%s", posted.Body)
	}
	if !strings.Contains(posted.Body, "revisão de código") && !strings.Contains(posted.Body, "Sugestões") {
		t.Fatalf("consumer language choice was not honored:\n%s", posted.Body)
	}
}

// TestAUR453NoSuggestionIsUnchanged covers AC-003's "behavior without
// suggestions": a review that returns no suggestions prints no Suggestions
// section at all, so the terminal output cannot grow a spurious empty heading.
func TestAUR453NoSuggestionIsUnchanged(t *testing.T) {
	response := `{"summary":"Nada a sugerir.","issues":[]}`
	code, out, errOut := aur453BaseReview(t, response, false)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	if strings.Contains(out, "### Suggestions") {
		t.Fatalf("empty suggestions section emitted:\n%s", out)
	}
}
