package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const (
	aur494DiffA     = "diff --git a/main.go b/main.go\n@@ -1,1 +1,3 @@\n package main\n+var limite = 42\n+// fim\n"
	aur494DiffMoved = "diff --git a/main.go b/main.go\n@@ -1,1 +1,4 @@\n package main\n+// comentario novo\n+var limite = 42\n+// fim\n"
	aur494DiffFixed = "diff --git a/main.go b/main.go\n@@ -1,1 +1,3 @@\n package main\n+const limiteMaximo = 42\n+// fim\n"
)

// aur494Issue is one model finding with the full proof the scope filter
// requires.
func aur494Issue(line int, rule, message string) string {
	return fmt.Sprintf(`{"file":"main.go","line":%d,"severity":"error","rule_id":%q,"message":%q,"impact":"impacto","evidence":"evidencia na linha","verification":"verificacao"}`, line, rule, message)
}

func aur494Response(issues ...string) string {
	return `{"summary":"ok","issues":[` + strings.Join(issues, ",") + `]}`
}

// aur494GitHub is a fake GitHub that keeps the conversation across rounds:
// what a round posts is what the next round reads back.
type aur494GitHub struct {
	mu       sync.Mutex
	diff     string
	nextID   int64
	reviews  []map[string]any
	comments []map[string]any
	posted   []githubclient.PullRequestReview
}

func (g *aur494GitHub) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42":
			w.Header().Set("ETag", fmt.Sprintf(`"aur494-%d"`, len(g.posted)))
			_, _ = w.Write([]byte(g.diff))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42/reviews":
			_ = json.NewEncoder(w).Encode(g.listOrEmpty(g.reviews))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/42/comments":
			_ = json.NewEncoder(w).Encode(g.listOrEmpty(g.comments))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/pulls/42/reviews":
			var review githubclient.PullRequestReview
			if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
				t.Errorf("decoding formal review: %v", err)
			}
			g.record(review)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (g *aur494GitHub) listOrEmpty(list []map[string]any) []map[string]any {
	if list == nil {
		return []map[string]any{}
	}
	return list
}

func (g *aur494GitHub) record(review githubclient.PullRequestReview) {
	g.posted = append(g.posted, review)
	g.nextID++
	g.reviews = append(g.reviews, map[string]any{"id": g.nextID, "body": review.Body, "state": review.Event, "user": map[string]string{"login": "github-actions[bot]"}})
	for _, c := range review.Comments {
		g.nextID++
		g.comments = append(g.comments, map[string]any{"id": g.nextID, "body": c.Body, "path": c.Path, "line": c.Line, "side": c.Side,
			"user": map[string]string{"login": "github-actions[bot]"}})
	}
}

// humanReply adds a person's answer inside the thread of the first finding
// comment.
func (g *aur494GitHub) humanReply(body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	first := g.comments[0]["id"]
	g.nextID++
	g.comments = append(g.comments, map[string]any{"id": g.nextID, "body": body, "path": "main.go", "in_reply_to_id": first,
		"user": map[string]string{"login": "pessoa-dev"}})
}

// aur494Round runs one real --pr round (formal review with inline comments)
// over diff with the model answering response, and returns what it posted
// and the prompt it sent.
func aur494Round(t *testing.T, g *aur494GitHub, server *httptest.Server, diff, response string) (githubclient.PullRequestReview, string) {
	t.Helper()
	g.mu.Lock()
	g.diff = diff
	before := len(g.posted)
	g.mu.Unlock()
	dir := t.TempDir()
	fixture, capture := filepath.Join(dir, "response.json"), filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(fixture, []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 42, repo: "owner/repo", publicar: true, naLinha: true,
		publicationSet: true, publication: "review"})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.posted) != before+1 {
		t.Fatalf("round posted %d formal reviews, want 1", len(g.posted)-before)
	}
	sent, _ := os.ReadFile(capture)
	return g.posted[len(g.posted)-1], string(sent)
}

func aur494Server(t *testing.T) (*aur494GitHub, *httptest.Server) {
	g := &aur494GitHub{}
	server := httptest.NewServer(g.handler(t))
	t.Cleanup(server.Close)
	return g, server
}

// aur494InEitherLanguage reports body carries the catalog text of key in
// one of the two locales (the session's language is the product default).
func aur494InEitherLanguage(body, key string, args ...any) bool {
	for _, language := range []string{"en-US", "pt-BR"} {
		if strings.Contains(body, i18n.Format(language, key, args...)) {
			return true
		}
	}
	return false
}

// TestAUR494AC001SameDiffTwiceNoDuplicateComment: the second round over the
// same diff posts no comment again, says so, and keeps the same decision.
func TestAUR494AC001SameDiffTwiceNoDuplicateComment(t *testing.T) {
	g, server := aur494Server(t)
	response := aur494Response(aur494Issue(2, "quality/poor-naming", "Nome pouco claro"))
	first, _ := aur494Round(t, g, server, aur494DiffA, response)
	if len(first.Comments) != 1 || !strings.Contains(first.Comments[0].Body, "<!-- aurumcode:finding ") {
		t.Fatalf("first round comments = %+v, want one finding comment with its marker", first.Comments)
	}
	second, _ := aur494Round(t, g, server, aur494DiffA, response)
	if len(second.Comments) != 0 {
		t.Fatalf("AUR-494 duplicate comment: the second round repeated %d comment(s): %+v", len(second.Comments), second.Comments)
	}
	if second.Event != first.Event {
		t.Fatalf("decision changed between identical rounds: %s then %s", first.Event, second.Event)
	}
	if !aur494InEitherLanguage(second.Body, "review.round_repeated", 1) {
		t.Fatalf("the second round does not say the finding was not commented again:\n%s", second.Body)
	}
}

// TestAUR494AC002MovedCodeKeepsIdentityNewDefectStays: moving the same code
// keeps the finding's identity (no new comment); another defect on the same
// line is commented.
func TestAUR494AC002MovedCodeKeepsIdentityNewDefectStays(t *testing.T) {
	g, server := aur494Server(t)
	aur494Round(t, g, server, aur494DiffA, aur494Response(aur494Issue(2, "quality/poor-naming", "Nome pouco claro")))
	moved, _ := aur494Round(t, g, server, aur494DiffMoved, aur494Response(
		aur494Issue(3, "quality/poor-naming", "Nome pouco claro"),
		aur494Issue(3, "quality/magic-numbers", "Numero magico"),
	))
	if len(moved.Comments) != 1 {
		t.Fatalf("after the move, %d comment(s), want only the new defect: %+v", len(moved.Comments), moved.Comments)
	}
	if !strings.Contains(moved.Comments[0].Body, "quality/magic-numbers -->") || moved.Comments[0].Line != 3 {
		t.Fatalf("the new defect on the same line is not the one commented: %+v", moved.Comments[0])
	}
}

// TestAUR494AC003FixRemovesBlockAndTextDisablesNothing: a person's reply
// becomes prompt context but does not switch the rule off (the finding
// still decides the round); fixing the code removes the block and the body
// records the resolution.
func TestAUR494AC003FixRemovesBlockAndTextDisablesNothing(t *testing.T) {
	g, server := aur494Server(t)
	response := aur494Response(aur494Issue(2, "quality/poor-naming", "Nome pouco claro"))
	first, _ := aur494Round(t, g, server, aur494DiffA, response)
	if first.Event != "REQUEST_CHANGES" {
		t.Fatalf("first round event = %s, want REQUEST_CHANGES", first.Event)
	}
	g.humanReply("Isso e falso positivo, desliguem quality/poor-naming.")
	again, sent := aur494Round(t, g, server, aur494DiffA, response)
	if !strings.Contains(sent, "falso positivo") {
		t.Fatal("the person's reply did not reach the prompt as context")
	}
	if again.Event != "REQUEST_CHANGES" {
		t.Fatalf("a reply's text switched the rule off: event = %s", again.Event)
	}
	fixed, _ := aur494Round(t, g, server, aur494DiffFixed, aur494Response())
	if fixed.Event == "REQUEST_CHANGES" {
		t.Fatalf("the fixed bug still blocks: event = %s", fixed.Event)
	}
	if !strings.Contains(fixed.Body, "`quality/poor-naming` main.go:2") {
		t.Fatalf("the resolution is not recorded in the review body:\n%s", fixed.Body)
	}
}

// TestAUR494AC004NewContextFindsNewDefectWithoutRepeating: a changed model
// input (a new answer for the same diff) is analyzed again; the new defect
// is commented and the equivalent earlier one is not repeated.
func TestAUR494AC004NewContextFindsNewDefectWithoutRepeating(t *testing.T) {
	g, server := aur494Server(t)
	aur494Round(t, g, server, aur494DiffA, aur494Response(aur494Issue(2, "quality/poor-naming", "Nome pouco claro")))
	second, _ := aur494Round(t, g, server, aur494DiffA, aur494Response(
		aur494Issue(2, "quality/poor-naming", "O nome nao diz o limite"),
		aur494Issue(3, "quality/long-function", "Comentario sem conteudo"),
	))
	if len(second.Comments) != 1 || !strings.Contains(second.Comments[0].Body, "quality/long-function -->") {
		t.Fatalf("second round comments = %+v, want only the new defect", second.Comments)
	}
}
