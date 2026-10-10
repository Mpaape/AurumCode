package main

// AUR-613: the engineering standards layer. A second central directory,
// given with --padroes (or AURUMCODE_STANDARDS), contributes skills and
// analysts below the central policy and above the repository; it never
// decides gate, ignore, exceptions, rules, scanners or publication. Every
// test drives the real command (review --base, --pr, the agent server)
// with the offline fixture provider.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	// aur613Skill is the standards layer's skill: one citable section.
	aur613Skill = "---\nname: erros-go\nlanguages: [go]\n---\nMARCADOR-PADROES\n\n## Erro com contexto\nseverity: error\nTodo erro devolvido carrega contexto.\n"
	aur613Rule  = "erros-go#erro-com-contexto"
	// aur613Finding cites the standards rule on an added line of app.go.
	aur613Finding = `{"summary":"x","verdict":"comment","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"` + aur613Rule + `","message":"MARCADOR-ACHADO erro sem contexto.","impact":"O erro perde a origem.","evidence":"app.go devolve o erro sem contexto.","suggestion":"Envolva o erro com contexto.","verification":"Leia a linha 3."}]}`
	aur613LabelEN = "[engineering standards]"
	aur613LabelPT = "[padrões de engenharia]"
	aur613Analyst = "profiles:\n  - name: \"qa-padroes\"\n    version: \"1\"\n    emphasis: \"qa do time de engenharia\"\n    families: [\"quality\"]\n    instructions: \"MARCADOR-ANALISTA-PADROES\"\n"
)

// aur613Repo builds a two-commit repository whose HEAD adds a function to
// app.go (lines 2-5) and clears every variable a layer could leak from.
func aur613Repo(t *testing.T, configYAML string) string {
	t.Helper()
	for _, key := range []string{"AURUMCODE_STANDARDS", "AURUMCODE_POLICY", "AURUMCODE_STANDARDS_REPOSITORY", "AURUMCODE_STANDARDS_REF", "AURUMCODE_PROMPT_CAPTURE"} {
		t.Setenv(key, "")
	}
	base := map[string][]byte{"app.go": []byte("package demo\n")}
	head := map[string][]byte{"app.go": []byte("package demo\n\nfunc Conecta() error {\n\treturn nil\n}\n")}
	return aur522Repo(t, base, head, configYAML)
}

// aur613Layer writes files (paths relative to the layer root) into a fresh
// directory outside every reviewed tree and returns it; .aurumcode/ always
// exists.
func aur613Layer(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	aur613Write(t, dir, files)
	if err := os.MkdirAll(filepath.Join(dir, ".aurumcode"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func aur613Write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// aur613Review runs review --base HEAD~1 with extra and returns the exit,
// stdout, stderr and the captured prompt.
func aur613Review(t *testing.T, extra ...string) (code int, stdout, stderr, prompt string) {
	t.Helper()
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	freshCache(t)
	var out, errOut strings.Builder
	code = runReview(append([]string{"--base", "HEAD~1"}, extra...), &out, &errOut, redaction.NewFilter())
	data, _ := os.ReadFile(capture)
	return code, out.String(), errOut.String(), string(data)
}

func aur613StandardsSkillLayer(t *testing.T) string {
	return aur613Layer(t, map[string]string{".aurumcode/skills/erros-go/SKILL.md": aur613Skill})
}

// AC-001 (--base): the standards skill reaches the model, its section is
// citable, and the review names the layer. Without the layer the same
// citation is an unknown rule and the finding never appears.
func TestAUR613StandardsSkillFindingOnBase(t *testing.T) {
	for _, tc := range []struct{ name, config, label string }{
		{"en", "", aur613LabelEN},
		{"pt-BR", "review:\n  language: pt-BR\n", aur613LabelPT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aur613Repo(t, tc.config)
			aur522Fixture(t, aur613Finding)
			layer := aur613StandardsSkillLayer(t)
			code, out, errOut, _ := aur613Review(t)
			if code != 0 || strings.Contains(out, "MARCADOR-ACHADO") {
				t.Fatalf("without the layer the citation must be unknown and dropped (exit=%d):\n%s%s", code, out, errOut)
			}
			code, out, errOut, prompt := aur613Review(t, "--padroes", layer)
			if code != 0 {
				t.Fatalf("exit=%d\n%s%s", code, out, errOut)
			}
			if !strings.Contains(prompt, "MARCADOR-PADROES") {
				t.Fatalf("the standards skill did not reach the prompt (%d bytes)", len(prompt))
			}
			if !strings.Contains(out, "MARCADOR-ACHADO") || !strings.Contains(out, tc.label) {
				t.Fatalf("the finding must appear naming the layer %q:\n%s", tc.label, out)
			}
		})
	}
}

// AC-001: AURUMCODE_STANDARDS declares the layer without the flag, and an
// explicitly empty --padroes is a usage error.
func TestAUR613EnvDeclaresLayerAndEmptyFlagIsUsageError(t *testing.T) {
	aur613Repo(t, "")
	aur522Fixture(t, aur613Finding)
	t.Setenv("AURUMCODE_STANDARDS", aur613StandardsSkillLayer(t))
	code, out, errOut, _ := aur613Review(t)
	if code != 0 || !strings.Contains(out, "MARCADOR-ACHADO") || !strings.Contains(out, aur613LabelEN) {
		t.Fatalf("AURUMCODE_STANDARDS did not apply the layer (exit=%d):\n%s%s", code, out, errOut)
	}
	if code, _, errOut, _ := aur613Review(t, "--padroes", ""); code != 2 || !strings.Contains(errOut, "--padroes: directory must not be empty") {
		t.Fatalf("empty --padroes: exit=%d, want 2 naming the flag:\n%s", code, errOut)
	}
}

// aur613PRServer serves one pull request (app.go, the aur613Repo change),
// the files at the base ref (404 for the rest) and captures the posted
// review body.
func aur613PRServer(t *testing.T, files map[string]string, posted *string) *httptest.Server {
	t.Helper()
	diff := "diff --git a/app.go b/app.go\n@@ -1,1 +1,5 @@\n package demo\n+\n+func Conecta() error {\n+\treturn nil\n+}\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const contents = "/repos/owner/repo/contents/"
		switch {
		case r.URL.Path == "/repos/owner/repo/pulls/61":
			_, _ = w.Write([]byte(diff))
		case strings.HasPrefix(r.URL.Path, contents):
			data, ok := files[strings.TrimPrefix(r.URL.Path, contents)]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(data))})
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			*posted = buf.String()
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	return server
}

// aur613PRReview runs review --pr 61 with the standards layer at dir and
// returns the exit, stderr and captured prompt.
func aur613PRReview(t *testing.T, standardsDir string) (code int, stderr, prompt string) {
	t.Helper()
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	var stdout, errOut strings.Builder
	code = runPRReview(reviewIO{stdout: &stdout, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{
		prNumber: 61, repo: "owner/repo", publicar: true, naLinha: true, publicationSet: true, publication: "review",
		standardsDir: standardsDir,
	})
	data, _ := os.ReadFile(capture)
	return code, stdout.String() + errOut.String(), string(data)
}

// AC-001 (--pr): the standards skill comes from the workflow's local
// checkout, the finding citing it is published and names the layer.
func TestAUR613StandardsSkillFindingOnPR(t *testing.T) {
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", "")
	aur522Fixture(t, aur613Finding)
	var posted string
	aur613PRServer(t, nil, &posted)
	code, errOut, prompt := aur613PRReview(t, aur613StandardsSkillLayer(t))
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, errOut)
	}
	if !strings.Contains(prompt, "MARCADOR-PADROES") {
		t.Fatalf("the standards skill did not reach the PR prompt (%d bytes)", len(prompt))
	}
	if !strings.Contains(posted, "MARCADOR-ACHADO") || !strings.Contains(posted, aur613LabelEN) {
		t.Fatalf("the published review must carry the finding naming the layer:\n%s", posted)
	}
}

// AC-001 (agent server, the local diff): AURUMCODE_STANDARDS reaches the
// review session the server runs and the rules listing names the layer.
func TestAUR613StandardsSkillThroughAgentServer(t *testing.T) {
	aur613Repo(t, "")
	aur522Fixture(t, aur613Finding)
	layer := aur613StandardsSkillLayer(t)
	t.Setenv("AURUMCODE_STANDARDS", layer)
	freshCache(t)
	review := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_review", `{"base":"HEAD~1"}`))[0])
	raw, _ := json.Marshal(review)
	if !strings.Contains(string(raw), "MARCADOR-ACHADO") || !strings.Contains(string(raw), aur613LabelEN) {
		t.Fatalf("aurum_review lost the standards finding or its layer: %s", raw)
	}
	rules := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_rules", `{"paths":["app.go"]}`))[0])
	if rules["standards"] != layer {
		t.Fatalf("aurum_rules does not name the standards layer: %v", rules)
	}
	found := false
	for _, s := range rules["skills"].([]any) {
		found = found || s.(map[string]any)["layer"] == "standards"
	}
	if !found {
		t.Fatalf("aurum_rules does not list a skill of the standards layer: %v", rules)
	}
}

// AC-001: the standards finding counts in the gate as source skills, under
// a central policy's gate and under the repository's own.
func TestAUR613StandardsFindingCountsInTheGate(t *testing.T) {
	gate := "gate:\n  fail_on: [error]\n"
	for _, tc := range []struct{ name, repoConfig, policyConfig string }{
		{"central policy", "", gate},
		{"repository gate", gate, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aur613Repo(t, tc.repoConfig)
			aur522Fixture(t, aur613Finding)
			extra := []string{"--padroes", aur613StandardsSkillLayer(t)}
			if tc.policyConfig != "" {
				extra = append(extra, "--politica", aur613Layer(t, map[string]string{".aurumcode/config.yml": tc.policyConfig}))
			}
			audit := filepath.Join(t.TempDir(), "audit.json")
			code, out, errOut, _ := aur613Review(t, append(extra, "--auditoria", audit)...)
			if code != exitFindings {
				t.Fatalf("exit=%d, want %d (the gate must fail on the standards finding)\n%s%s", code, exitFindings, out, errOut)
			}
			if !strings.Contains(out+errOut, aur613Rule) || !strings.Contains(out+errOut, "origem skills") {
				t.Fatalf("the gate line must name the rule with origin skills:\n%s%s", out, errOut)
			}
			var rec struct {
				Blocking []struct {
					RuleID string `json:"rule_id"`
					Origin string `json:"origin"`
				} `json:"blocking_findings"`
			}
			data, err := os.ReadFile(audit)
			if err != nil || json.Unmarshal(data, &rec) != nil || len(rec.Blocking) != 1 || rec.Blocking[0].RuleID != aur613Rule || rec.Blocking[0].Origin != "skills" {
				t.Fatalf("audit blocking findings = %+v (err %v), want the standards rule with origin skills", rec.Blocking, err)
			}
		})
	}
}

// aur613Finding builds the fixture answer citing the given rules, one
// finding per rule, each on an added line of app.go with its own marker.
func aur613Findings(rules map[string]string) string {
	var issues []string
	line := 3
	for rule, marker := range rules {
		issues = append(issues, `{"file":"app.go","line":`+itoaTest(line)+`,"severity":"error","rule_id":"`+rule+`","message":"`+marker+` violação.","impact":"Impacto.","evidence":"app.go.","suggestion":"Corrija.","verification":"Leia."}`)
		line++
	}
	return `{"summary":"x","verdict":"comment","issues":[` + strings.Join(issues, ",") + `]}`
}

// aur613SkillDoc is a skill with one citable section (## title) and a
// marker in its instructions, selected by the given front-matter selector.
func aur613SkillDoc(name, selector, title, marker string) string {
	return "---\nname: " + name + "\n" + selector + "\n---\n" + marker + "\n\n## " + title + "\nseverity: error\nRegra.\n"
}

// AC-002 (--base): an analyst the standards layer defines runs when the
// service selects it in review.profiles; without the layer the same
// selection is an unknown analyst and no model call happens.
func TestAUR613StandardsAnalystRunsWhenSelected(t *testing.T) {
	aur613Repo(t, "review:\n  profiles: [qa-padroes]\n")
	aur522Fixture(t, aur522Clean)
	layer := aur613Layer(t, map[string]string{".aurumcode/profiles.yml": aur613Analyst})
	code, _, errOut, prompt := aur613Review(t)
	if code != 2 || prompt != "" || !strings.Contains(errOut, "qa-padroes") {
		t.Fatalf("without the layer the analyst must be unknown before any model call (exit=%d, prompt=%d bytes):\n%s", code, len(prompt), errOut)
	}
	code, out, errOut, prompt := aur613Review(t, "--padroes", layer)
	if code != 0 {
		t.Fatalf("exit=%d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(prompt, "MARCADOR-ANALISTA-PADROES") || !strings.Contains(errOut, "profiles: qa-padroes") {
		t.Fatalf("the standards analyst did not run:\nstderr: %s\nprompt bytes: %d", errOut, len(prompt))
	}
}

// AC-002 (--pr): review.profiles comes from the base ref; the analyst it
// names comes from the standards layer of the workflow's local checkout.
func TestAUR613StandardsAnalystRunsOnPR(t *testing.T) {
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", "")
	aur522Fixture(t, aur522Clean)
	var posted string
	aur613PRServer(t, map[string]string{".aurumcode/config.yml": "review:\n  profiles: [qa-padroes]\n"}, &posted)
	code, errOut, prompt := aur613PRReview(t, aur613Layer(t, map[string]string{".aurumcode/profiles.yml": aur613Analyst}))
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, errOut)
	}
	if !strings.Contains(prompt, "MARCADOR-ANALISTA-PADROES") {
		t.Fatalf("the standards analyst did not run the PR pass (%d bytes):\n%s", len(prompt), errOut)
	}
}

// AC-002: a repository analyst with the name of a standards analyst is
// ignored with a warning naming both layers; the standards one runs.
func TestAUR613StandardsAnalystShadowsRepositoryAnalyst(t *testing.T) {
	repoAnalyst := strings.Replace(aur613Analyst, "MARCADOR-ANALISTA-PADROES", "MARCADOR-ANALISTA-REPO", 1)
	dir := aur613Repo(t, "review:\n  profiles: [qa-padroes]\n")
	aur613Write(t, dir, map[string]string{".aurumcode/profiles.yml": repoAnalyst})
	aur522Fixture(t, aur522Clean)
	code, out, errOut, prompt := aur613Review(t, "--padroes", aur613Layer(t, map[string]string{".aurumcode/profiles.yml": aur613Analyst}))
	if code != 0 {
		t.Fatalf("exit=%d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(prompt, "MARCADOR-ANALISTA-PADROES") || strings.Contains(prompt, "MARCADOR-ANALISTA-REPO") {
		t.Fatalf("the standards analyst must win over the repository's (%d bytes)", len(prompt))
	}
	if !strings.Contains(errOut, `analyst "qa-padroes" of the repository layer ignored: the engineering standards layer declares an analyst with the same name`) {
		t.Fatalf("the warning must name both layers:\n%s", errOut)
	}
}

// aur613ThreeLayers builds the three directories of a corporate review --
// the security policy (with the gate), the engineering standards and the
// service -- each with skills, so the same fixture serves the layering
// proofs and the multi-repository tutorials. Same-name skills: erros-go in
// the policy and the standards (the policy wins), nomes-go in the standards
// and the service (the standards win); testes-go is the service's alone.
func aur613ThreeLayers(t *testing.T) (policy, standards string) {
	t.Helper()
	repo := aur613Repo(t, "")
	aur613Write(t, repo, map[string]string{
		".aurumcode/skills/nomes-go/SKILL.md":  aur613SkillDoc("nomes-go", `paths: ["app.go"]`, "Nome claro", "MARCADOR-REPO-NOMES"),
		".aurumcode/skills/testes-go/SKILL.md": aur613SkillDoc("testes-go", `paths: ["*.go"]`, "Teste junto", "MARCADOR-REPO-TESTES"),
	})
	policy = aur613Layer(t, map[string]string{
		".aurumcode/config.yml":                "gate:\n  fail_on: [error]\n",
		".aurumcode/skills/erros-go/SKILL.md": aur613SkillDoc("erros-go", "languages: [go]", "Erro com contexto", "MARCADOR-POLITICA-ERROS"),
	})
	standards = aur613Layer(t, map[string]string{
		".aurumcode/skills/erros-go/SKILL.md": aur613SkillDoc("erros-go", "languages: [go]", "Erro com contexto", "MARCADOR-PADROES-ERROS"),
		".aurumcode/skills/nomes-go/SKILL.md": aur613SkillDoc("nomes-go", `paths: ["app.go"]`, "Nome claro", "MARCADOR-PADROES-NOMES"),
	})
	return policy, standards
}

// AC-003 and the three-layer integration: policy > standards > service.
// The prompt carries the winner of each name, every loser is announced
// with both layers named, the finding of the standards rule is attributed
// to the layer in the review, and under the policy's gate the policy's and
// the standards' findings block while the service's own does not.
func TestAUR613ThreeLayersOrderAndAttribution(t *testing.T) {
	policy, standards := aur613ThreeLayers(t)
	aur522Fixture(t, aur613Findings(map[string]string{
		"erros-go#erro-com-contexto": "MARCADOR-ACHADO-ERROS",
		"nomes-go#nome-claro":        "MARCADOR-ACHADO-NOMES",
		"testes-go#teste-junto":      "MARCADOR-ACHADO-TESTES",
	}))
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, out, errOut, prompt := aur613Review(t, "--politica", policy, "--padroes", standards, "--auditoria", audit)
	if code != exitFindings {
		t.Fatalf("exit=%d, want %d\n%s%s", code, exitFindings, out, errOut)
	}
	for _, want := range []string{"MARCADOR-POLITICA-ERROS", "MARCADOR-PADROES-NOMES", "MARCADOR-REPO-TESTES"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("the winner %s did not reach the prompt (%d bytes)", want, len(prompt))
		}
	}
	for _, lost := range []string{"MARCADOR-PADROES-ERROS", "MARCADOR-REPO-NOMES"} {
		if strings.Contains(prompt, lost) {
			t.Fatalf("the shadowed skill %s reached the prompt", lost)
		}
	}
	for _, warning := range []string{
		`skill "erros-go" of the engineering standards layer ignored: the security policy layer declares a skill with the same name`,
		`skill "nomes-go" of the repository layer ignored: the engineering standards layer declares a skill with the same name`,
	} {
		if !strings.Contains(errOut, warning) {
			t.Fatalf("missing warning %q in:\n%s", warning, errOut)
		}
	}
	if line := aur613Line(out, "MARCADOR-ACHADO-NOMES"); !strings.HasSuffix(line, "(rule nomes-go#nome-claro: Nome claro) "+aur613LabelEN) {
		t.Fatalf("the standards finding must be attributed to the layer:\n%s", line)
	}
	for _, marker := range []string{"MARCADOR-ACHADO-ERROS", "MARCADOR-ACHADO-TESTES"} {
		if line := aur613Line(out, marker); strings.Contains(line, "["+"engineering") || !strings.HasSuffix(line, ")") {
			t.Fatalf("only the standards finding carries a layer label:\n%s", line)
		}
	}
	var rec struct {
		Blocking []struct {
			RuleID string `json:"rule_id"`
			Origin string `json:"origin"`
		} `json:"blocking_findings"`
	}
	data, err := os.ReadFile(audit)
	if err != nil || json.Unmarshal(data, &rec) != nil {
		t.Fatalf("audit: %v", err)
	}
	blocking := map[string]string{}
	for _, f := range rec.Blocking {
		blocking[f.RuleID] = f.Origin
	}
	if len(blocking) != 2 || blocking["erros-go#erro-com-contexto"] != "skills" || blocking["nomes-go#nome-claro"] != "skills" {
		t.Fatalf("blocking = %v, want the policy's and the standards' rules as source skills and not the service's", blocking)
	}
}

// AC-003 without a policy: the standards layer wins over the service and
// the service's same-name skill is announced, naming both layers.
func TestAUR613StandardsWinOverRepositorySkill(t *testing.T) {
	_, standards := aur613ThreeLayers(t)
	aur522Fixture(t, aur613Findings(map[string]string{"nomes-go#nome-claro": "MARCADOR-ACHADO-NOMES"}))
	code, out, errOut, prompt := aur613Review(t, "--padroes", standards)
	if code != 0 {
		t.Fatalf("exit=%d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(prompt, "MARCADOR-PADROES-NOMES") || strings.Contains(prompt, "MARCADOR-REPO-NOMES") || !strings.Contains(prompt, "MARCADOR-PADROES-ERROS") {
		t.Fatalf("without a policy the standards skills must win and all apply (%d bytes)", len(prompt))
	}
	if !strings.Contains(errOut, `skill "nomes-go" of the repository layer ignored: the engineering standards layer declares a skill with the same name`) {
		t.Fatalf("missing the warning naming both layers:\n%s", errOut)
	}
	if line := aur613Line(out, "MARCADOR-ACHADO-NOMES"); !strings.HasSuffix(line, aur613LabelEN) {
		t.Fatalf("the finding must name the layer:\n%s", line)
	}
}

// AC-004: a config.yml in the standards layer that declares any section is
// refused with an error that names the section, what is allowed and the
// layer; a comment-only file is accepted. The refusal happens before any
// model call.
func TestAUR613StandardsConfigWithSectionIsRefused(t *testing.T) {
	aur613Repo(t, "")
	aur522Fixture(t, aur522Clean)
	for _, section := range []string{"gate", "ignore", "exceptions", "rules", "quality_gates", "publication", "dependencies", "review"} {
		t.Run(section, func(t *testing.T) {
			layer := aur613Layer(t, map[string]string{".aurumcode/config.yml": section + ": {}\n"})
			code, _, errOut, prompt := aur613Review(t, "--padroes", layer)
			want := `standards layer: ` + filepath.Join(layer, ".aurumcode", "config.yml") + ` declares the sections "` + section + `"; allowed: no section`
			if code != 1 || !strings.Contains(errOut, want) || prompt != "" {
				t.Fatalf("exit=%d (prompt %d bytes), want 1 and %q in:\n%s", code, len(prompt), want, errOut)
			}
		})
	}
	t.Run("second document", func(t *testing.T) {
		layer := aur613Layer(t, map[string]string{".aurumcode/config.yml": "# nothing\n---\ngate: {}\n"})
		if code, _, errOut, _ := aur613Review(t, "--padroes", layer); code != 1 || !strings.Contains(errOut, `declares the sections "gate"`) {
			t.Fatalf("a second YAML document must not carry a section past the check (exit=%d):\n%s", code, errOut)
		}
	})
	t.Run("not a mapping", func(t *testing.T) {
		layer := aur613Layer(t, map[string]string{".aurumcode/config.yml": "- gate\n"})
		if code, _, errOut, _ := aur613Review(t, "--padroes", layer); code != 1 || !strings.Contains(errOut, "found: a sequence document, not a mapping of sections") {
			t.Fatalf("a sequence document must be refused naming what was found (exit=%d):\n%s", code, errOut)
		}
	})
	t.Run("comment only", func(t *testing.T) {
		layer := aur613Layer(t, map[string]string{".aurumcode/config.yml": "# the standards layer declares nothing\n"})
		if code, out, errOut, _ := aur613Review(t, "--padroes", layer); code != 0 {
			t.Fatalf("a comment-only config.yml must be accepted (exit=%d):\n%s%s", code, out, errOut)
		}
	})
	t.Run("pt-BR", func(t *testing.T) {
		aur613Repo(t, "review:\n  language: pt-BR\n")
		layer := aur613Layer(t, map[string]string{".aurumcode/config.yml": "gate: {}\n"})
		if code, _, errOut, _ := aur613Review(t, "--padroes", layer); code != 1 || !strings.Contains(errOut, `camada de padrões: `) || !strings.Contains(errOut, `declara as seções "gate"; permitido: nenhuma seção`) {
			t.Fatalf("the refusal must be in the review language (exit=%d):\n%s", code, errOut)
		}
	})
}

// AC-005: a standards directory inside the reviewed tree (the tree itself
// or a directory under it) is refused before any model call, like the
// policy; a missing .aurumcode in the declared layer is an error too.
func TestAUR613StandardsInsideReviewedTreeIsRefused(t *testing.T) {
	repo := aur613Repo(t, "")
	aur522Fixture(t, aur522Clean)
	aur613Write(t, repo, map[string]string{"padroes/.aurumcode/skills/erros-go/SKILL.md": aur613Skill})
	for name, dir := range map[string]string{"the tree itself": repo, "a directory under it": filepath.Join(repo, "padroes"), "relative": "padroes"} {
		t.Run(name, func(t *testing.T) {
			code, _, errOut, prompt := aur613Review(t, "--padroes", dir)
			if code != 1 || !strings.Contains(errOut, "standards layer: "+dir+" is inside the reviewed repository") || !strings.Contains(errOut, "a standards layer must come from outside the reviewed tree") || prompt != "" {
				t.Fatalf("exit=%d (prompt %d bytes), want 1 and the refusal:\n%s", code, len(prompt), errOut)
			}
		})
	}
	t.Run("missing .aurumcode", func(t *testing.T) {
		dir := t.TempDir()
		if code, _, errOut, _ := aur613Review(t, "--padroes", dir); code != 1 || !strings.Contains(errOut, "standards layer: "+filepath.Join(dir, ".aurumcode")+" does not exist") {
			t.Fatalf("exit=%d:\n%s", code, errOut)
		}
	})
}

// AC-006: review.yml checks standards_repository out into
// .aurumcode-standards, read-only and without persisting credentials, only
// when the input is set, and passes --padroes (with the mount) under the
// same condition; without the input no argument and no mount change.
func TestAUR613WorkflowChecksOutStandardsLayer(t *testing.T) {
	wf, steps := loadAUR555Workflow(t)
	raw, err := os.ReadFile(aur613WorkflowPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"      standards_repository:\n        description:", "      standards_ref:\n        description:",
		"          repository: ${{ inputs.standards_repository }}", "          ref: ${{ inputs.standards_ref }}",
		"          path: .aurumcode-standards", "          token: ${{ secrets.AURUMCODE_STANDARDS_TOKEN || github.token }}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("review.yml lacks %q", want)
		}
	}
	secret, ok := wf.On.WorkflowCall.Secrets["AURUMCODE_STANDARDS_TOKEN"]
	if !ok || secret.Required == nil || *secret.Required {
		t.Fatalf("AURUMCODE_STANDARDS_TOKEN must be an optional workflow_call secret: %+v", secret)
	}
	checkout := steps[aur555Index(t, steps, "Checkout engineering standards")]
	if !strings.Contains(checkout.If, "inputs.standards_repository != ''") {
		t.Fatalf("the checkout must run only with the input: if=%q", checkout.If)
	}
	block := text[strings.Index(text, "name: Checkout engineering standards"):]
	block = block[:strings.Index(block, "- name:")]
	if !strings.Contains(block, "persist-credentials: false") || !strings.Contains(block, "path: .aurumcode-standards") {
		t.Fatalf("the standards checkout must not persist credentials and must land in .aurumcode-standards:\n%s", block)
	}
	review := steps[aur555Index(t, steps, aur555ReviewStep)]
	if review.Env["AURUMCODE_STANDARDS_REPOSITORY"] != "${{ inputs.standards_repository }}" || review.Env["AURUMCODE_STANDARDS_REF"] != "${{ inputs.standards_ref }}" {
		t.Fatalf("the review step must receive the standards origin for the audit: %v", review.Env)
	}
	guard := `if [ -n "${AURUMCODE_STANDARDS_REPOSITORY}" ]; then`
	if strings.Count(review.Run, guard) != 2 {
		t.Fatalf("--padroes and the mount must each sit under %q (found %d)", guard, strings.Count(review.Run, guard))
	}
	run := strings.Join(strings.Fields(review.Run), " ")
	for _, want := range []string{
		guard + " args+=(--padroes /github/standards) fi",
		guard + " docker_args+=(-v \"${GITHUB_WORKSPACE}/.aurumcode-standards:/github/standards:ro\" -e AURUMCODE_STANDARDS_REPOSITORY -e AURUMCODE_STANDARDS_REF) fi",
	} {
		if !strings.Contains(run, want) {
			t.Fatalf("review step lacks:\n%s\nin:\n%s", want, review.Run)
		}
	}
	var code []string
	for _, line := range strings.Split(review.Run, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			code = append(code, line)
		}
	}
	if shell := strings.Join(code, "\n"); strings.Count(shell, "--padroes") != 1 || strings.Count(shell, "/github/standards") != 2 {
		t.Fatalf("--padroes and the mount appear only under the guard:\n%s", shell)
	}
}

// aur613WorkflowPath is review.yml, or the copy the acceptance script
// points AUR555_WORKFLOW at.
func aur613WorkflowPath() string {
	if p := os.Getenv("AUR555_WORKFLOW"); p != "" {
		return p
	}
	return filepath.Join("..", "..", ".github", "workflows", "review.yml")
}

// aur613Audit runs review --base with --auditoria and returns the record's
// raw JSON object.
func aur613Audit(t *testing.T, extra ...string) map[string]json.RawMessage {
	t.Helper()
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, out, errOut, _ := aur613Review(t, append(extra, "--auditoria", audit)...)
	if code != 0 {
		t.Fatalf("exit=%d\n%s%s", code, out, errOut)
	}
	data, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]json.RawMessage
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// AC-007: the audit record carries the standards layer -- where it came
// from and the digest of its content -- only when one was declared; the
// workflow's origin is recorded when present, "local" otherwise; and the
// digest follows the content.
func TestAUR613AuditRecordsStandardsLayerOnlyWhenDeclared(t *testing.T) {
	aur613Repo(t, "")
	aur522Fixture(t, aur522Clean)
	layer := aur613StandardsSkillLayer(t)
	if raw, ok := aur613Audit(t)["standards"]; ok {
		t.Fatalf("without the layer the audit must not carry standards: %s", raw)
	}
	var entry struct{ Source, Ref, Digest string }
	if err := json.Unmarshal(aur613Audit(t, "--padroes", layer)["standards"], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Source != "local" || entry.Ref != "" || len(entry.Digest) != 64 {
		t.Fatalf("standards = %+v, want source local, no ref and a sha256 digest", entry)
	}
	first := entry.Digest
	t.Setenv("AURUMCODE_STANDARDS_REPOSITORY", "org/padroes-engenharia")
	t.Setenv("AURUMCODE_STANDARDS_REF", "0123abcd")
	aur613Write(t, layer, map[string]string{".aurumcode/skills/erros-go/SKILL.md": aur613Skill + "\nMais uma linha.\n"})
	if err := json.Unmarshal(aur613Audit(t, "--padroes", layer)["standards"], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Source != "org/padroes-engenharia" || entry.Ref != "0123abcd" || entry.Digest == first || len(entry.Digest) != 64 {
		t.Fatalf("standards = %+v, want the workflow's origin and a digest that follows the content (was %s)", entry, first)
	}
}

// AC-008: without --padroes and AURUMCODE_STANDARDS nothing names the
// layer: not the report, not the warnings, not the audit record, not the
// agent server's rules listing; and the layer-aware seams are identities.
func TestAUR613WithoutTheLayerNothingChanges(t *testing.T) {
	policy, _ := aur613ThreeLayers(t)
	aur522Fixture(t, aur613Findings(map[string]string{"testes-go#teste-junto": "MARCADOR-ACHADO-TESTES"}))
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, out, errOut, prompt := aur613Review(t, "--politica", policy, "--auditoria", audit)
	if code != 0 {
		t.Fatalf("exit=%d\n%s%s", code, out, errOut)
	}
	data, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"stdout": out, "stderr": errOut, "prompt": prompt, "audit": string(data)} {
		for _, trace := range []string{"standards", "padrões", "padroes", "engineering"} {
			if strings.Contains(strings.ToLower(text), trace) {
				t.Fatalf("%s names the layer (%q) although none was declared:\n%s", name, trace, text)
			}
		}
	}
	if !strings.Contains(prompt, "MARCADOR-POLITICA-ERROS") || !strings.Contains(prompt, "MARCADOR-REPO-NOMES") || !strings.Contains(prompt, "MARCADOR-REPO-TESTES") {
		t.Fatalf("without the layer the policy's and the repository's skills all apply, same-name or not (%d bytes)", len(prompt))
	}
	freshCache(t)
	rules := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_rules", `{"paths":["app.go"]}`))[0])
	if _, ok := rules["standards"]; ok {
		t.Fatalf("aurum_rules names the layer although none was declared: %v", rules)
	}
	issues := []types.ReviewIssue{{RuleID: "x", Message: "m"}}
	if got := attributeLayerFindings(issues, nil, config.Layers[config.LayerStandards], "en"); len(got) != 1 || got[0].Message != "m" {
		t.Fatalf("attribution without a layer changed the issues: %+v", got)
	}
	if layer, err := loadCentralLayer(config.Layers[config.LayerStandards], "", ".", "en", reviewEnv{}); layer != nil || err != nil {
		t.Fatalf("an undeclared layer must load as nothing: %v %v", layer, err)
	}
}

// aur613Line is the line of text that carries marker, "" when none does.
func aur613Line(text, marker string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, marker) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
