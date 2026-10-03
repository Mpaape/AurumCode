package main

// The model weighs the deterministic evidence: the passes run before the
// model, their findings reach the prompt, the model's per-evidence
// assessment travels beside the engine's origin to the report, the audit
// and the SARIF, and the gate keeps counting policy evidence whatever the
// model says. Driven through the real `aurumcode review --base` command.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur579Assessments answers about the three offered evidence items of the
// fixture (sorted by origin: analysis E1, sast E2, security E3) plus one id
// that was never offered.
const aur579Assessments = `[
 {"evidence_id":"E1","status":"disputed","justification":"o valor e um exemplo de teste, nao uma credencial","correlates_with":["E3"],"priority":"low","suggestion":"mover o exemplo para um arquivo de teste"},
 {"evidence_id":"E2","status":"needs_context","justification":"depende de onde o valor e lido","priority":"medium","suggestion":""},
 {"evidence_id":"E3","status":"disputed","justification":"mesmo trecho de E1","correlates_with":["E1","E9"],"priority":"low","suggestion":""},
 {"evidence_id":"E99","status":"confirmed","justification":"inventado","priority":"high"}
]`

// aur579Fixture writes a conditional offline fixture: when the prompt
// carries the evidence section it answers with assessments, otherwise with
// a plain approval.
func aur579Fixture(t *testing.T, assessments string) string {
	t.Helper()
	body := `{"aurumcode_fixture":{"cases":[{"prompt_contains":"[E1] origem=analysis","response":{"verdict":"comment","strengths":[],"issues":[],"suggestions":[],"ci_analysis":[],"test_plan":[],"limitations":[],"summary":"avaliou a evidencia","evidence_assessments":` + assessments + `}}],"default":{"verdict":"approve","strengths":[],"issues":[],"suggestions":[],"ci_analysis":[],"test_plan":[],"limitations":[],"summary":"sem evidencia"}}}`
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// aur579Repo builds a git repository whose HEAD~1..HEAD diff adds, on
// app.go line 4, a credential that both the security pass and the
// embedded analysis catalog report; no provider is configured.
func aur579Repo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(source, parent string) string {
		blob := gitObject(t, dir, "blob", []byte(source))
		tree := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", blob))
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	base := commit("package demo\n\nfunc Run() {\n}\n", "")
	headSrc := "package demo\n\nfunc Run() {\n\tdbPassword := \"hunter2-correct-horse\"\n\t_ = dbPassword\n}\n"
	head := commit(headSrc, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(headSrc))
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_PROMPT_CAPTURE", "AURUMCODE_POLICY"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Cleanup(chdir(t, dir))
}

// aur579SAST enables SAST in the fixture repository and returns a Semgrep
// runner that reports one finding on the credential line.
func aur579SAST(t *testing.T, extraConfig string) semgrepRunner {
	t.Helper()
	cfg := "quality_gates:\n  sast:\n    enabled: true\n" + extraConfig
	if err := os.MkdirAll(".aurumcode", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".aurumcode", "config.yml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return func(context.Context, string, ...string) (string, string, error) {
		return `{"results":[{"check_id":"generic.secrets.hardcoded","path":"app.go","start":{"line":4},"extra":{"severity":"ERROR","message":"Hardcoded secret"}}]}`, "", nil
	}
}

// aur579Review runs the review with the given semgrep runner.
func aur579Review(t *testing.T, semgrep semgrepRunner, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	rio := reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: reviewDeps{semgrep: semgrep, env: &reviewEnv{}}}
	code := runReviewWith(rio, append([]string{"--base", "HEAD~1"}, args...))
	return code, out.String(), errOut.String()
}

// AC-001: the prompt the model receives already carries the findings of the
// security pass, the embedded analysis and SAST.
func TestAUR579PromptCarriesEveryPassBeforeTheModel(t *testing.T) {
	aur579Repo(t)
	semgrep := aur579SAST(t, "")
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur579Assessments))
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	if code, _, errOut := aur579Review(t, semgrep, "--seguranca"); code != exitFindings {
		t.Fatalf("exit=%d, want the SAST finding to fail the repository's SAST gate; stderr=%s", code, errOut)
	}
	sent, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Deterministic evidence",
		"[E1] origem=analysis regra=analysis/hardcoded-secret local=app.go:4",
		"[E2] origem=sast regra=semgrep:generic.secrets.hardcoded local=app.go:4",
		"[E3] origem=security regra=security/hardcoded-secret local=app.go:4",
		"evidence_assessments",
	} {
		if !strings.Contains(string(sent), want) {
			t.Errorf("the prompt sent to the model lacks %q:\n%s", want, sent)
		}
	}
}

// AC-002: the report, the audit and the SARIF carry the engine's origin
// beside the model's assessment; an assessment of evidence never offered is
// discarded with a warning.
func TestAUR579AssessmentBesideOriginInEverySink(t *testing.T) {
	aur579Repo(t)
	semgrep := aur579SAST(t, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur579Assessments))
	dir := t.TempDir()
	audit, sarif := filepath.Join(dir, "audit.json"), filepath.Join(dir, "out.sarif")
	code, out, errOut := aur579Review(t, semgrep, "--seguranca", "--auditoria", audit, "--sarif", sarif)
	if code != exitFindings {
		t.Fatalf("exit=%d; stderr=%s", code, errOut)
	}
	for _, want := range []string{
		"origem: analysis | avaliacao do modelo: disputed [E1] prioridade low - o valor e um exemplo de teste",
		"correlacao: E3",
		"origem: sast | avaliacao do modelo: needs_context [E2]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, `discarded 1 evidence assessment(s)`) || !strings.Contains(errOut, `"E99"`) {
		t.Errorf("the assessment of evidence never offered must be discarded loudly:\n%s", errOut)
	}
	var rec struct {
		Evidence []struct {
			RuleID     string `json:"rule_id"`
			Origin     string `json:"origin"`
			Assessment struct {
				EvidenceID string   `json:"evidence_id"`
				Status     string   `json:"status"`
				Correlates []string `json:"correlates_with"`
			} `json:"assessment"`
		} `json:"evidence_assessments"`
	}
	readJSON(t, audit, &rec)
	got := map[string]string{}
	for _, e := range rec.Evidence {
		got[e.Origin+" "+e.RuleID] = e.Assessment.EvidenceID + " " + e.Assessment.Status
		if e.Assessment.EvidenceID == "E3" && strings.Join(e.Assessment.Correlates, ",") != "E1" {
			t.Errorf("a correlation to evidence never offered must be dropped: %v", e.Assessment.Correlates)
		}
	}
	for key, want := range map[string]string{
		"analysis analysis/hardcoded-secret":     "E1 disputed",
		"sast semgrep:generic.secrets.hardcoded": "E2 needs_context",
		"security security/hardcoded-secret":     "E3 disputed",
	} {
		if got[key] != want {
			t.Errorf("audit evidence %q = %q, want %q (all: %v)", key, got[key], want, got)
		}
	}
	var log struct {
		Runs []struct {
			Results []struct {
				RuleID     string `json:"ruleId"`
				Properties struct {
					Origin     string `json:"origin"`
					Assessment *struct {
						Status string `json:"status"`
					} `json:"assessment"`
				} `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	readJSON(t, sarif, &log)
	found := false
	for _, r := range log.Runs[0].Results {
		if r.RuleID == "analysis/hardcoded-secret" && r.Properties.Origin == "analysis" && r.Properties.Assessment != nil && r.Properties.Assessment.Status == "disputed" {
			found = true
		}
	}
	if !found {
		t.Errorf("SARIF lacks origin and assessment side by side for analysis/hardcoded-secret")
	}
}

// AC-003: under a central policy the disputed evidence still counts (the
// gate fails) and becomes a proposed exception; without a policy,
// gate.triage: model lets the same dispute demote it and the gate passes.
func TestAUR579PolicyFloorAndRepositoryTriage(t *testing.T) {
	aur579Repo(t)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur579Assessments))
	policy := policyFixture(t, "gate:\n  fail_on: [high]\n  triage:\n    analysis: model\n")
	code, out, errOut := aur579Review(t, nil, "--politica", policy)
	if code != exitFindings {
		t.Fatalf("policy: a disputed policy finding must still fail the gate: exit=%d\nstderr=%s", code, errOut)
	}
	for _, want := range []string{"Excecoes propostas pelo modelo", `rule: "analysis/hardcoded-secret"`, `path: "app.go"`, `owner: "<dono a definir>"`} {
		if !strings.Contains(out, want) {
			t.Errorf("policy: the dispute must become a proposed exception (%q):\n%s", want, out)
		}
	}
	if strings.Contains(errOut, "gate.triage") {
		t.Errorf("policy: nothing may be demoted under a central policy:\n%s", errOut)
	}

	repoCfg := func(triage string) {
		cfg := "gate:\n  fail_on: [high]\n" + triage
		if err := os.MkdirAll(".aurumcode", 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".aurumcode", "config.yml"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repoCfg("")
	if code, _, errOut := aur579Review(t, nil); code != exitFindings {
		t.Fatalf("repository, triage absent (none): the dispute must not demote: exit=%d\nstderr=%s", code, errOut)
	}
	repoCfg("  triage:\n    analysis: model\n")
	code, out, errOut = aur579Review(t, nil)
	if code != 0 {
		t.Fatalf("repository, gate.triage analysis: model: the dispute demotes and the gate passes: exit=%d\nstderr=%s", code, errOut)
	}
	if !strings.Contains(errOut, "gate.triage (analysis: model): app.go:4 analysis/hardcoded-secret") {
		t.Errorf("a demotion is never silent:\n%s", errOut)
	}
	if strings.Contains(out, "Excecoes propostas") {
		t.Errorf("a demoted finding needs no proposed exception:\n%s", out)
	}
}

// AC-004 (behavior half): the assessment exists only because the evidence
// section reached the prompt; the same fixture without the section answers
// its default.
func TestAUR579EvidenceSectionDecidesTheAnswer(t *testing.T) {
	aur579Repo(t)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur579Assessments))
	_, out, errOut := aur579Review(t, nil)
	if !strings.Contains(out, "avaliacao do modelo: disputed [E1]") {
		t.Fatalf("with the evidence section the model's assessment must appear:\n%s\nstderr=%s", out, errOut)
	}
}

// AC-005: the cache and verdict-reuse key moves with the evidence offered,
// and a run without evidence keeps its previous key.
func TestAUR579ContextKeyIncludesEvidence(t *testing.T) {
	s := &reviewState{provider: &review.FakeProvider{NameStr: "m"}}
	legacy := reviewContextCacheKey(s.provider, s.baseModelIdentity, s.reviewLanguage, s.codebaseText, s.memoryNotesText, s.profileIdentity, s.contextBlockDigest, s.ruleCatalogDigest)
	if s.contextCacheKey() != legacy {
		t.Fatal("a run without evidence must keep the key it had before evidence existed")
	}
	s.evidence = []prompt.EvidenceItem{{ID: "E1", Origin: "analysis", RuleID: "analysis/hardcoded-secret", File: "app.go", Line: 4}}
	withEvidence := s.contextCacheKey()
	if withEvidence == legacy {
		t.Fatal("an answer given without the evidence must not be reused once the evidence exists")
	}
	s.evidence[0].Line = 5
	if s.contextCacheKey() == withEvidence {
		t.Fatal("different evidence shared a key")
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, data)
	}
}
