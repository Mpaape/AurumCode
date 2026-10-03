package main

// AUR-565: the review (--base and --pr) reads the repository's and the
// policy's skill directories (.aurumcode/skills/<dir>/SKILL.md with an
// optional `languages:`) through the skills catalog. Every assertion reads
// the prompt the model actually received (AURUMCODE_PROMPT_CAPTURE) or the
// review text; the model is the deterministic fixture provider.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const aur565Response = `{"summary":"reviewed","issues":[]}`

// aur565Repo builds a two-commit repository whose only change is one file
// named file (so its extension decides the language) and returns its root.
func aur565Repo(t *testing.T, file string) string {
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
	baseBlob := gitObject(t, dir, "blob", []byte("// base\nlet a = 1\n"))
	headBlob := gitObject(t, dir, "blob", []byte("// base\nlet a = 1\nlet b = 2\n"))
	rootBase := gitObject(t, dir, "tree", treeEntry(t, "100644", file, baseBlob))
	rootHead := gitObject(t, dir, "tree", treeEntry(t, "100644", file, headBlob))
	commit := func(tree, parent, msg string) string {
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\n" + msg + "\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	base := commit(rootBase, "", "base")
	head := commit(rootHead, base, "head")
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write(file, []byte("// base\nlet a = 1\nlet b = 2\n"))
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Cleanup(chdir(t, dir))
	return dir
}

func aur565WriteSkill(t *testing.T, root, dir, content string) {
	t.Helper()
	p := filepath.Join(root, ".aurumcode", "skills", dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// aur565Review runs --base HEAD~1 with the fixture model and returns the
// captured prompt, stdout and stderr.
func aur565Review(t *testing.T, extra ...string) (prompt, stdout, stderr string) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(aur565Response), 0600); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	var out, errOut strings.Builder
	args := append([]string{"--base", "HEAD~1"}, extra...)
	if code := runReview(args, &out, &errOut, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	return string(data), out.String(), errOut.String()
}

const aur565TSSkill = "---\nname: estilo-ts\nversion: 1\nlanguages: [typescript]\n---\nMARCADOR-ESTILO-TS\n"

// TestAUR565LanguageSelectsSkill covers AC-001: a skill with
// `languages: [typescript]` reaches the model for a .ts change and not for
// a .go change.
func TestAUR565LanguageSelectsSkill(t *testing.T) {
	for _, tc := range []struct {
		file string
		want bool
	}{{"app.ts", true}, {"app.go", false}} {
		t.Run(tc.file, func(t *testing.T) {
			root := aur565Repo(t, tc.file)
			aur565WriteSkill(t, root, "estilo-ts", aur565TSSkill)
			prompt, _, _ := aur565Review(t)
			if got := strings.Contains(prompt, "MARCADOR-ESTILO-TS"); got != tc.want {
				t.Fatalf("skill in prompt = %v, want %v for %s:\n%s", got, tc.want, tc.file, prompt)
			}
		})
	}
}

// TestAUR565AliasResolvesAndUnknownIsDeclared covers AC-002: the alias `ts`
// selects the skill; an unknown alias is declared in the context sent to the
// model and in the review text, never silently.
func TestAUR565AliasResolvesAndUnknownIsDeclared(t *testing.T) {
	root := aur565Repo(t, "app.ts")
	aur565WriteSkill(t, root, "alias-ts", "---\nname: alias-ts\nlanguages: [ts]\n---\nMARCADOR-ALIAS-TS\n")
	aur565WriteSkill(t, root, "alias-ruim", "---\nname: alias-ruim\nlanguages: [linguagem-inexistente]\n---\nMARCADOR-ALIAS-RUIM\n")
	prompt, stdout, _ := aur565Review(t)
	if !strings.Contains(prompt, "MARCADOR-ALIAS-TS") {
		t.Fatalf("alias ts did not select the skill:\n%s", prompt)
	}
	if strings.Contains(prompt, "MARCADOR-ALIAS-RUIM") {
		t.Fatalf("a skill with an unknown language must match nothing:\n%s", prompt)
	}
	for name, text := range map[string]string{"prompt": prompt, "review": stdout} {
		if !strings.Contains(text, `unknown language "linguagem-inexistente"`) {
			t.Errorf("unknown alias not declared in the %s:\n%s", name, text)
		}
	}
	if !strings.Contains(prompt, "### Skill selection warnings") {
		t.Errorf("prompt has no selection warnings block:\n%s", prompt)
	}
}

// TestAUR565PolicySkillWinsOverRepo covers AC-003: a policy skill with the
// same selector as a repository skill replaces it, with a declared warning.
func TestAUR565PolicySkillWinsOverRepo(t *testing.T) {
	root := aur565Repo(t, "app.ts")
	aur565WriteSkill(t, root, "estilo-ts", "---\nname: estilo-ts\nlanguages: [ts]\n---\nMARCADOR-REPO\n")
	aur565WriteSkill(t, root, "outro", "---\nname: outro\nlanguages: [javascript]\n---\nMARCADOR-OUTRO\n")
	policy := policyFixture(t, "")
	aur565WriteSkill(t, policy, "org-ts", "---\nname: org-ts\nlanguages: [typescript]\n---\nMARCADOR-POLITICA\n")
	prompt, stdout, _ := aur565Review(t, "--politica", policy)
	if !strings.Contains(prompt, "MARCADOR-POLITICA") {
		t.Fatalf("policy skill missing from the prompt:\n%s", prompt)
	}
	if strings.Contains(prompt, "MARCADOR-REPO") {
		t.Fatalf("repository skill with the same selector must not reach the model:\n%s", prompt)
	}
	for name, text := range map[string]string{"prompt": prompt, "review": stdout} {
		if !strings.Contains(text, `policy skill "org-ts"`) || !strings.Contains(text, `overrides repository skill "estilo-ts"`) {
			t.Errorf("override not declared in the %s:\n%s", name, text)
		}
	}
}

// TestAUR565PolicySkillLoadErrorFailsClosed: a policy skill that cannot be
// parsed is a load error before any model call, while the same defect in the
// repository is only declared.
func TestAUR565PolicySkillLoadErrorFailsClosed(t *testing.T) {
	root := aur565Repo(t, "app.ts")
	policy := policyFixture(t, "")
	aur565WriteSkill(t, policy, "quebrada", "---\nname: quebrada\nlanguages: [ts]\n")
	fixture := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(fixture, []byte(aur565Response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1", "--politica", policy}, &out, &errOut, redaction.NewFilter()); code != 1 {
		t.Fatalf("exit=%d, want 1; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "central policy") || !strings.Contains(errOut.String(), "unterminated front matter") {
		t.Fatalf("stderr does not name the policy load error:\n%s", errOut.String())
	}
	aur565WriteSkill(t, root, "quebrada", "---\nname: quebrada\nlanguages: [ts]\n")
	_, stdout, _ := aur565Review(t)
	if !strings.Contains(stdout, "repository skills unavailable") {
		t.Fatalf("repository load error not declared in the review:\n%s", stdout)
	}
}

// TestAUR565InstructionsStillWork: applyTo instructions are untouched.
func TestAUR565InstructionsStillWork(t *testing.T) {
	root := aur565Repo(t, "app.ts")
	p := filepath.Join(root, ".aurumcode", "instructions", "ts.md")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("---\napplyTo: \"**/*.ts\"\n---\nMARCADOR-INSTRUCAO\n"), 0600); err != nil {
		t.Fatal(err)
	}
	prompt, _, _ := aur565Review(t)
	if !strings.Contains(prompt, "MARCADOR-INSTRUCAO") {
		t.Fatalf("instructions regressed:\n%s", prompt)
	}
}

// TestAUR565PRSelectsSkillsAtTheBaseRef: --pr lists and reads the skills at
// the trusted ref through the contents API and applies the same selection.
func TestAUR565PRSelectsSkillsAtTheBaseRef(t *testing.T) {
	diffBody := "diff --git a/app.ts b/app.ts\n@@ -1,1 +1,2 @@\n let a = 1\n+let b = 2\n"
	skill := aur565TSSkill
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/65":
			_, _ = w.Write([]byte(diffBody))
		case r.URL.Path == "/repos/owner/repo/contents/.aurumcode/skills":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "estilo-ts", "type": "dir"}, {"name": "README.md", "type": "file"}})
		case r.URL.Path == "/repos/owner/repo/contents/.aurumcode/skills/estilo-ts/SKILL.md":
			_ = json.NewEncoder(w).Encode(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(skill))})
		case strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments"):
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(`{"id":1}`))
			}
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	fixture, capture := filepath.Join(dir, "response.json"), filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(fixture, []byte(aur565Response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 65, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{publicationSet: true, publication: "review"})
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "MARCADOR-ESTILO-TS") {
		t.Fatalf("remote skill did not reach the prompt:\n%s", data)
	}
}
