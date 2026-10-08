package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// Loose Git objects exercise both the Go reader and Git when installed, without
// requiring a Git binary, subprocess commits, credentials, or global config.
func localPassFixture(t *testing.T, memory bool) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	object := func(kind string, body []byte) string {
		t.Helper()
		payload := append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...)
		sum := sha1.Sum(payload)
		id := hex.EncodeToString(sum[:])
		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		write(".git/objects/"+id[:2]+"/"+id[2:], compressed.Bytes())
		return id
	}
	commit := func(source, parent string) string {
		blob := object("blob", []byte(source))
		raw, _ := hex.DecodeString(blob)
		tree := object("tree", append([]byte("100644 app.go\x00"), raw...))
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return object("commit", []byte(body))
	}
	base := commit("package demo\nfunc Change() {}\n", "")
	source := "package demo\nfunc Change() {\n dbPassword := \"hunter2-super-secret\"\n _ = dbPassword\n}\n"
	head := commit(source, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(source))
	if memory {
		write(".aurumcode/config.yml", []byte("review:\n  memory: local\n"))
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir
}

func localPassReview(args ...string) (int, string, string) {
	var out, errOut strings.Builder
	code := runReview(append([]string{"--base", "HEAD~1"}, args...), &out, &errOut, redaction.NewFilter())
	return code, out.String(), errOut.String()
}

func TestAUR490LocalAnalysis(t *testing.T) {
	localPassFixture(t, false)
	code, out, errOut := localPassReview("--fail-on", "error")
	if code != 3 || !strings.Contains(out, "app.go:3: [error]") || !strings.Contains(out, "analysis/hardcoded-secret") {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(errOut, "quality review skipped") || strings.Contains(out, "**Verdict:** Approve") {
		t.Fatal("incomplete quality review presented as complete")
	}
	code, _, _ = localPassReview("--exigir-qualidade", "--fail-on", "error")
	if code != 1 {
		t.Fatalf("required missing LLM exit=%d, want 1", code)
	}
	code, _, _ = localPassReview("--modelo", "explicit-missing-model")
	if code != 1 {
		t.Fatalf("unavailable explicit model exit=%d", code)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", "/missing/response.json")
	code, _, _ = localPassReview()
	if code != 1 {
		t.Fatalf("broken provider configuration silently downgraded: %d", code)
	}
}

func TestAUR490LocalRender(t *testing.T) {
	localPassFixture(t, false)
	code, out, errOut := localPassReview()
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{"## AurumCode code review", "> [!", `app.go`, "deterministic analysis only"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}

func TestAUR490LocalMemory(t *testing.T) {
	localPassFixture(t, true)
	if code, _, errOut := localPassReview(); code != 0 {
		t.Fatalf("first run: %d %s", code, errOut)
	}
	store, err := newRepoMemory("local", "", "")
	if err != nil {
		t.Fatal(err)
	}
	notes, err := store.Load()
	if err != nil || len(notes) == 0 {
		t.Fatalf("memory not persisted: %v %+v", err, notes)
	}
	notes[0].Body = "prior-observation-from-the-author"
	if err := store.Save(notes); err != nil {
		t.Fatal(err)
	}
	prompts := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		var promptText string
		for _, m := range request.Messages {
			promptText += m.Content
		}
		prompts <- promptText
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"issues\":[],\"summary\":\"Checked.\"}"}}]}`)
	}))
	defer server.Close()
	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_API_KEY", "local-test-key")
	if code, _, errOut := localPassReview(); code != 0 {
		t.Fatalf("second run: %d %s", code, errOut)
	}
	promptText := <-prompts
	for _, want := range []string{"prior-observation-from-the-author", "Change"} {
		if !strings.Contains(promptText, want) {
			t.Fatalf("memory/context absent from actual model prompt: %q", want)
		}
	}
}

func TestAUR490SharedPasses(t *testing.T) {
	// Inspect real call sites, not a hardcoded accessor reporting a fictional
	// list. Both routes share context, memory and static analysis; only local
	// output gets the optional CLI summary/diagram. End-to-end tests execute
	// both publication routes.
	//
	// AUR-536: pr.go's own codebase-context entrypoint is
	// resolveVerifiedCodebaseContext, not resolveCodebaseContext itself --
	// on --pr, the checkout is not necessarily the change under review (see
	// aur515.go/aur536.go), so --pr additionally reads only the exact file
	// set its own clean-checkout proof already verified, by construction,
	// rather than resolveCodebaseContext's own, independent filesystem walk.
	// main.go (--base) keeps calling resolveCodebaseContext directly: there
	// the checkout IS the change under review, so no such proof applies.
	// Both entrypoints still share codebaseContextJSON's marshal-or-empty
	// tail (passes.go) -- asserted directly on their own source files below,
	// since neither main.go nor pr.go calls codebaseContextJSON itself.
	for file, required := range map[string][]string{
		// AUR-557: the bodies of runReview and runPRReview moved into the
		// --base/--pr phase files; the same calls are asserted, now across
		// the files that hold them.
		"review_base_inputs.go": {"resolveCodebaseContext", "openReviewMemory"},
		// The evidence phase runs the analysis before the model, through
		// the one helper both sources call.
		"review_base_analysis.go": {"staticAnalysisIssues"},
		"review_weighing.go":      {"mergeStaticAnalysis"},
		"review_base_publish.go":  {"persistReviewMemory", "renderLocalReport"},
		"review_pr_inputs.go":     {"resolveVerifiedCodebaseContext", "openReviewMemory"},
		"review_pr_analysis.go":   {"staticAnalysisIssues"},
		"review_pr_publish.go":    {"persistReviewMemory", "formatGatedReviewBody"},
		"codebase_context.go":     {"codebaseContextJSON"},
		"verified_checkout.go":    {"codebaseContextJSON"},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		calls := map[string]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok {
					calls[id.Name] = true
				}
			}
			return true
		})
		for _, name := range required {
			if !calls[name] {
				t.Errorf("%s does not call %s", file, name)
			}
		}
	}
	p := &review.FakeProvider{Response: `{"issues":[]}`}
	key := reviewContextCacheKey(p, "base-model", "pt-BR", "context", "first note", "", "", "")
	if key == reviewContextCacheKey(p, "base-model", "pt-BR", "context", "new feedback", "", "", "") {
		t.Fatal("cache ignores changed memory")
	}
	if key == reviewContextCacheKey(p, "base-model", "pt-BR", "changed dependency", "first note", "", "", "") {
		t.Fatal("cache ignores dependency context")
	}
	// AUR-513 (AC-001/AC-002/AC-003-point-3): the four new identity
	// components -- the base model identity captured before any context
	// wrapping, the selected profiles, the assembled context-block digest,
	// the rule-catalog digest -- must each independently change the key
	// too, not only the original three.
	if key == reviewContextCacheKey(p, "different-base-model", "pt-BR", "context", "first note", "", "", "") {
		t.Fatal("cache ignores changed base model identity")
	}
	if key == reviewContextCacheKey(p, "base-model", "pt-BR", "context", "first note", "solid", "", "") {
		t.Fatal("cache ignores changed profile selection")
	}
	if key == reviewContextCacheKey(p, "base-model", "pt-BR", "context", "first note", "", "block-digest-changed", "") {
		t.Fatal("cache ignores changed context-block digest")
	}
	if key == reviewContextCacheKey(p, "base-model", "pt-BR", "context", "first note", "", "", "catalog-digest-changed") {
		t.Fatal("cache ignores changed rule-catalog digest")
	}
}
