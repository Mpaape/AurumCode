package main

// AUR-556 behavior proofs: the embedded analysis catalog counts toward the
// policy gate, gate.sources restricts it, and without a gate nothing
// changes. Driven through the real `aurumcode review --base` command with
// a silent model fixture, so only the deterministic analysis pass can find
// the defect.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur556SQLiFixture builds a git repo whose HEAD~1..HEAD diff adds a SQL
// query built by string concatenation to app.go (analysis/sql-injection,
// error). repoConfig is the repository's own .aurumcode/config.yml.
func aur556SQLiFixture(t *testing.T, repoConfig string) string {
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
	base := commit("package demo\nfunc Find(id string) {}\n", "")
	headSrc := "package demo\nfunc Find(id string) {\n q := \"SELECT * FROM users WHERE id=\" + id\n _ = q\n}\n"
	head := commit(headSrc, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(headSrc))
	if repoConfig != "" {
		write(".aurumcode/config.yml", []byte(repoConfig))
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Cleanup(chdir(t, dir))
	// A silent model: it reports nothing and approves.
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	return dir
}

func aur556Run(t *testing.T, extra ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e strings.Builder
	args := append([]string{"--base", "HEAD~1"}, extra...)
	code = runReview(args, &o, &e, redaction.NewFilter())
	return code, o.String(), e.String()
}

// TestAUR556AnalysisFindingFailsPolicyGate is AC-001.
func TestAUR556AnalysisFindingFailsPolicyGate(t *testing.T) {
	aur556SQLiFixture(t, "")
	policy := policyFixture(t, "gate:\n  fail_on: [error]\n")
	audit := filepath.Join(t.TempDir(), "audit.json")
	sarif := filepath.Join(t.TempDir(), "out.sarif")
	code, out, errOut := aur556Run(t, "--politica", policy, "--auditoria", audit, "--sarif", sarif)
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	const origin = "origem analysis"
	if !strings.Contains(errOut, "analysis/sql-injection") || !strings.Contains(errOut, origin) {
		t.Errorf("stderr must cite the rule and origin:\n%s", errOut)
	}
	rawAudit, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawAudit), `"decision": "fail"`) && !strings.Contains(string(rawAudit), `"decision":"fail"`) {
		t.Errorf("audit gate decision must be fail:\n%s", rawAudit)
	}
	var rec auditFile
	if err := json.Unmarshal(rawAudit, &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.BlockingFindings) != 1 || rec.BlockingFindings[0].RuleID != "analysis/sql-injection" || rec.BlockingFindings[0].Origin != "analysis" {
		t.Errorf("audit blocking_findings must carry the typed origin: %+v", rec.BlockingFindings)
	}
	rawSARIF, err := os.ReadFile(sarif)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Runs []struct {
			Results []struct {
				RuleID     string                  `json:"ruleId"`
				Message    struct{ Text string }   `json:"message"`
				Properties struct{ Origin string } `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(rawSARIF, &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range doc.Runs[0].Results {
		if r.RuleID == "analysis/sql-injection" {
			found = true
			if r.Properties.Origin != "analysis" {
				t.Errorf("SARIF properties.origin=%q, want analysis", r.Properties.Origin)
			}
			if strings.Contains(r.Message.Text, "origem") {
				t.Errorf("origin must not be a message suffix: %q", r.Message.Text)
			}
		}
	}
	if !found {
		t.Errorf("SARIF lacks the finding:\n%s", rawSARIF)
	}
}

// TestAUR556SourcesRestrictAndPolicyGoverns is AC-002.
func TestAUR556SourcesRestrictAndPolicyGoverns(t *testing.T) {
	// gate.sources: [skills] on the policy: the same finding does not fail.
	aur556SQLiFixture(t, "")
	policy := policyFixture(t, "gate:\n  fail_on: [error]\n  sources: [skills]\n")
	if code, out, errOut := aur556Run(t, "--politica", policy); code != 0 {
		t.Fatalf("sources [skills] must not count analysis: exit=%d stdout=%s stderr=%s", code, out, errOut)
	}

	// The repository's own gate/sources cannot change the policy's list.
	aur556SQLiFixture(t, "gate:\n  fail_on: [error]\n  sources: [analysis]\n")
	if code, out, errOut := aur556Run(t, "--politica", policy); code != 0 {
		t.Fatalf("repo sources must be ignored under a policy: exit=%d stdout=%s stderr=%s", code, out, errOut)
	}

	// And a repo cannot loosen a policy that counts every source.
	aur556SQLiFixture(t, "gate:\n  fail_on: [error]\n  sources: [skills]\n")
	open := policyFixture(t, "gate:\n  fail_on: [error]\n")
	if code, out, errOut := aur556Run(t, "--politica", open); code != exitFindings {
		t.Fatalf("repo sources [skills] must not loosen the policy: exit=%d stdout=%s stderr=%s", code, out, errOut)
	}
}

// TestAUR556NoGateUnchanged is AC-003: no gate declared, same diff, today's
// behavior (the finding is reported, the check does not fail, no gate lines).
func TestAUR556NoGateUnchanged(t *testing.T) {
	aur556SQLiFixture(t, "")
	policy := policyFixture(t, "")
	code, out, errOut := aur556Run(t, "--politica", policy)
	if code != 0 {
		t.Fatalf("no gate: exit=%d, want 0; stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "analysis/sql-injection") {
		t.Errorf("the finding is still reported in the parecer:\n%s", out)
	}
	if strings.Contains(out+errOut, "policy gate") {
		t.Errorf("no gate declared must leave no policy-gate trace:\n%s\n%s", out, errOut)
	}
}

// TestAUR556GateSourcesValidation covers the closed list.
func TestAUR556GateSourcesValidation(t *testing.T) {
	if _, err := config.Parse([]byte("gate:\n  fail_on: [error]\n  sources: [skills, analysis, sast]\n"), "t.yml"); err != nil {
		t.Fatalf("valid sources rejected: %v", err)
	}
	if _, err := config.Parse([]byte("gate:\n  fail_on: [error]\n  sources: [skills, mystery]\n"), "t.yml"); err == nil {
		t.Fatal("unknown gate.sources value must be a load error")
	}
	g := config.GateConfig{}
	for _, s := range []string{"skills", "analysis", "sast"} {
		if !g.SourceEnabled(s) {
			t.Errorf("empty sources must enable %s", s)
		}
	}
	g.Sources = []string{"skills"}
	if g.SourceEnabled("analysis") || !g.SourceEnabled("skills") {
		t.Error("sources [skills] must enable only skills")
	}
}

// TestAUR556PRPathCountsAnalysisFinding is AC-001 on the --pr path: the
// published parecer, the audit record and the commit status all carry the
// analysis origin.
func TestAUR556PRPathCountsAnalysisFinding(t *testing.T) {
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Find(id string) {\n+ q := \"SELECT * FROM users WHERE id=\" + id\n+ _ = q\n+}\n"
	headConfig := "gate:\n  fail_on: [error]\n"
	var reviewBody string
	var publishedStatus githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = w.Write([]byte(`{"head":{"sha":"head-sha"}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = w.Write([]byte(`{"content":"` + b64(headConfig) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			raw, _ := io.ReadAll(r.Body)
			reviewBody = string(raw)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(&publishedStatus)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	setPRGateEnv(t, server, fixture)
	auditPath := filepath.Join(t.TempDir(), "audit.json")
	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true,
		publicationSet: true, publication: "review", auditoriaPath: auditPath,
	})
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(reviewBody, "origem analysis") {
		t.Errorf("published parecer must cite the analysis origin:\n%s", reviewBody)
	}
	if publishedStatus.Context != policyGateContext || publishedStatus.State != "failure" || !strings.Contains(publishedStatus.Description, "analysis") {
		t.Errorf("policy gate status=%+v, want context %s, failure, description citing analysis", publishedStatus, policyGateContext)
	}
	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var rec auditFile
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.BlockingFindings) != 1 || rec.BlockingFindings[0].Origin != "analysis" {
		t.Errorf("audit blocking origin: %+v", rec.BlockingFindings)
	}
}
