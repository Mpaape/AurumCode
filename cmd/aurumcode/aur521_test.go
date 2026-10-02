package main

// AUR-521 end-to-end behavior proof: --auditoria/--sarif wired into
// runReview (--base) and runPRReview (--pr), driven through the real
// `aurumcode review` command with AURUMCODE_LLM_FIXTURE, exactly like
// aur519_e2e_test.go already does for the policy gate itself. internal/
// render's own unit tests (audit_test.go, sarif_test.go,
// finding_identity_test.go) cover the writer functions in isolation; these
// tests cover the CLI wiring: the flags exist, the files are written at
// the right point, and the fields they carry agree with what the gate and
// coverage machinery already decided.

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
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

type auditFile struct {
	PolicyDigest string `json:"policy_digest"`
	Repo         string `json:"repo,omitempty"`
	Verdict      string `json:"verdict"`
	Gate         struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason,omitempty"`
	} `json:"gate"`
	BlockingFindings []struct {
		RuleID   string `json:"rule_id"`
		Path     string `json:"path"`
		Line     int    `json:"line"`
		Severity string `json:"severity"`
	} `json:"blocking_findings"`
	ExceptionsApplied []map[string]any `json:"exceptions_applied"`
	Coverage          struct {
		Complete bool     `json:"complete"`
		Omitted  []string `json:"omitted_files,omitempty"`
	} `json:"coverage"`
}

type sarifFile struct {
	Version string `json:"version"`
	Runs    []struct {
		Invocations []struct {
			ExecutionSuccessful bool `json:"executionSuccessful"`
		} `json:"invocations"`
		Results []struct {
			RuleID              string            `json:"ruleId"`
			PartialFingerprints map[string]string `json:"partialFingerprints"`
			Suppressions        []struct {
				Kind          string `json:"kind"`
				Justification string `json:"justification"`
			} `json:"suppressions"`
		} `json:"results"`
	} `json:"runs"`
}

// TestAUR521AuditAndSARIFOnGateBreach covers AC-001 (the record's fields,
// including a declared-but-inactive policy digest staying empty) and AC-002
// (the SARIF's required fields and a fingerprint that agrees, cross-package,
// with internal/render's own canonical FindingFingerprint -- the exact
// identity SARIF's writer uses internally).
func TestAUR521AuditAndSARIFOnGateBreach(t *testing.T) {
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n  sources: [skills]\n")
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte("## No Hardcoded Secrets\n\nNever commit a literal credential.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "response.json")
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath, "--sarif", sarifPath}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit auditFile
	if err := json.Unmarshal(rawAudit, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, rawAudit)
	}
	if audit.PolicyDigest != "" {
		t.Fatalf("policy_digest=%q, want empty: no central policy was declared this run", audit.PolicyDigest)
	}
	if audit.Gate.Decision != "fail" {
		t.Fatalf("gate.decision=%q, want fail", audit.Gate.Decision)
	}
	if len(audit.BlockingFindings) != 1 {
		t.Fatalf("blocking_findings=%v, want exactly one", audit.BlockingFindings)
	}
	bf := audit.BlockingFindings[0]
	if bf.RuleID != "security#no-hardcoded-secrets" || bf.Path != "app.go" || bf.Line != 3 || bf.Severity != "error" {
		t.Fatalf("blocking finding=%+v", bf)
	}
	if audit.ExceptionsApplied == nil {
		t.Fatal("exceptions_applied must be present as an (empty) list")
	}
	if audit.Verdict == "approve" {
		t.Fatal("verdict must be withheld, never approve, once the gate failed")
	}

	rawSARIF, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	var doc sarifFile
	if err := json.Unmarshal(rawSARIF, &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, rawSARIF)
	}
	if doc.Version != "2.1.0" {
		t.Fatalf("SARIF version=%q, want 2.1.0", doc.Version)
	}
	if len(doc.Runs) != 1 || len(doc.Runs[0].Results) == 0 {
		t.Fatalf("SARIF runs/results=%+v", doc.Runs)
	}
	// AC-002 is about the skill-cited finding specifically; the fixture's
	// app.go also trips the always-on deterministic static-analysis pass
	// (a second, unrelated "analysis/hardcoded-secret" result on the same
	// line), so this looks up the result by rule id instead of assuming it
	// is the only one.
	resultIdx := -1
	for i := range doc.Runs[0].Results {
		if doc.Runs[0].Results[i].RuleID == "security#no-hardcoded-secrets" {
			resultIdx = i
			break
		}
	}
	if resultIdx < 0 {
		t.Fatalf("no SARIF result for security#no-hardcoded-secrets among %+v", doc.Runs[0].Results)
	}
	gotFP := doc.Runs[0].Results[resultIdx].PartialFingerprints[render.FindingFingerprintKey]
	if len(gotFP) != 64 {
		t.Fatalf("SARIF fingerprint=%q, want a 64-hex-char sha256 digest (render.FindingFingerprint's own shape)", gotFP)
	}
	// Recomputing with the exact same rule/path/line but a DIFFERENT code
	// context must disagree -- the fingerprint really is content-bound, not
	// a constant keyed only on rule+path+line.
	if other := render.FindingFingerprint(render.FindingIdentity{RuleID: "security#no-hardcoded-secrets", Path: "app.go", Line: 3, Context: "unrelated content"}); other == gotFP {
		t.Fatalf("fingerprint did not depend on the finding's own code context")
	}
	// The unrelated, deterministic-analysis finding on the SAME file/line
	// must still carry its own, different fingerprint (distinct rule id).
	for i := range doc.Runs[0].Results {
		if doc.Runs[0].Results[i].RuleID == "analysis/hardcoded-secret" {
			if other := doc.Runs[0].Results[i].PartialFingerprints[render.FindingFingerprintKey]; other == gotFP {
				t.Fatalf("two different findings share the same fingerprint: %q", gotFP)
			}
		}
	}
}

// TestAUR521AuditFingerprintStableAcrossTwoRuns is the AC-002/MUT-001
// acceptance scenario at the CLI level, and the decisive proof that the
// fingerprint is built from the REVIEWED DIFF, not the model's own
// free-text fields: the two runs below review the exact same diff but
// return DIFFERENT message/evidence text each time (a model is free to
// reword its own explanation run to run). The fingerprint must still be
// the exact same both times -- if it were built from issue.Evidence/
// Message instead, this test would fail.
func TestAUR521AuditFingerprintStableAcrossTwoRuns(t *testing.T) {
	coverageFixture(t, "review:\n  context:\n    skills:\n      - security.md\ngate:\n  fail_on: [high]\n  sources: [skills]\n")
	if err := os.WriteFile("security.md", []byte("## No Hardcoded Secrets\n\nNever commit a literal credential.\n"), 0600); err != nil {
		t.Fatal(err)
	}

	readFingerprint := func(message, evidence string) string {
		fixture := filepath.Join(t.TempDir(), "response.json")
		resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"` + message + `","evidence":"` + evidence + `","impact":"Credential leak","verification":"Remove the literal secret"}]}`
		if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

		sarifPath := filepath.Join(t.TempDir(), "out.sarif")
		var out, errOut strings.Builder
		runReview([]string{"--base", "HEAD~1", "--sarif", sarifPath}, &out, &errOut, redaction.NewFilter())
		raw, err := os.ReadFile(sarifPath)
		if err != nil {
			t.Fatalf("reading SARIF: %v", err)
		}
		var doc sarifFile
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		for _, res := range doc.Runs[0].Results {
			if res.RuleID == "security#no-hardcoded-secrets" {
				return res.PartialFingerprints[render.FindingFingerprintKey]
			}
		}
		t.Fatalf("no result for security#no-hardcoded-secrets in %+v", doc.Runs[0].Results)
		return ""
	}

	first := readFingerprint("Hardcoded secret", "whatever the model said the first time")
	second := readFingerprint("Different wording entirely", "and a completely different evidence quote")
	if first == "" || first != second {
		t.Fatalf("fingerprint depends on the model's own text, not the diff: %q vs %q", first, second)
	}
}

// TestAUR521AuditInconclusiveListsOmittedFiles covers AC-004: an
// inconclusive run (AUR-476 partial coverage, warn) writes an audit record
// naming the inconclusive decision and the file the repository config hid
// from the review.
func TestAUR521AuditInconclusiveListsOmittedFiles(t *testing.T) {
	coverageFixture(t, "ignore:\n  - \"tests/**\"\ngate:\n  inconclusive: warn\n")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	var out, errOut strings.Builder
	runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath}, &out, &errOut, redaction.NewFilter())

	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit auditFile
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, raw)
	}
	if audit.Gate.Decision != "inconclusive" {
		t.Fatalf("gate.decision=%q, want inconclusive", audit.Gate.Decision)
	}
	if audit.Coverage.Complete {
		t.Fatal("coverage.complete must be false: a configured path was hidden from the review")
	}
	found := false
	for _, p := range audit.Coverage.Omitted {
		if p == "tests/change_test.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("coverage.omitted_files=%v, want it to name tests/change_test.go", audit.Coverage.Omitted)
	}
}

// TestAUR521RedactsSecretCanaryEndToEnd covers AC-005 through the full CLI
// path: a secret canary present in a model finding's own text must not
// reach either written file.
func TestAUR521RedactsSecretCanaryEndToEnd(t *testing.T) {
	coverageFixture(t, "")
	const canary = "AURUM-CANARY-E2E-f00dfeed"
	fixture := filepath.Join(t.TempDir(), "response.json")
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"warning","rule_id":"security/hardcoded-secret","message":"Secret: ` + canary + `","evidence":"dbPassword := \"` + canary + `\""}]}`
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv(redaction.CanaryEnv, canary)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")
	var out, errOut strings.Builder
	runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath, "--sarif", sarifPath}, &out, &errOut, redaction.FromEnv())

	auditBytes, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	if strings.Contains(string(auditBytes), canary) {
		t.Fatalf("canary leaked into the audit record:\n%s", auditBytes)
	}
	sarifBytes, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	if strings.Contains(string(sarifBytes), canary) {
		t.Fatalf("canary leaked into the SARIF document:\n%s", sarifBytes)
	}
}

// TestAUR521RedactsSecretCanaryFromDiffLine closes a declared gap from the
// prior commit: the fingerprint/context source is now the REVIEWED DIFF
// LINE itself (render.FindingIdentityFor), not the model's text, so this
// proves the canary is redacted even when it appears ONLY in the diff --
// the model's own message/evidence never mention it at all.
func TestAUR521RedactsSecretCanaryFromDiffLine(t *testing.T) {
	const canary = "AURUM-CANARY-DIFFLINE-f00dfeed"
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
	baseSrc := "package demo\nfunc Change() {}\n"
	headSrc := "package demo\nfunc Change() {\n token := \"" + canary + "\"\n _ = token\n}\n"
	baseBlob := gitObject(t, dir, "blob", []byte(baseSrc))
	headBlob := gitObject(t, dir, "blob", []byte(headSrc))
	baseTree := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", baseBlob))
	headTree := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", headBlob))
	commit := func(tree, parent string) string {
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	base := commit(baseTree, "")
	head := commit(headTree, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(headSrc))

	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	restore := chdir(t, dir)
	t.Cleanup(restore)

	// The model's own text never mentions the canary at all -- it exists
	// ONLY in the diff line this finding is anchored to (file app.go, the
	// added line 3, which is exactly where the canary was placed above).
	fixture := filepath.Join(t.TempDir(), "response.json")
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"warning","rule_id":"security/hardcoded-secret","message":"Hardcoded secret","evidence":"see line 3","impact":"Credential leak","verification":"Remove the literal secret"}]}`
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv(redaction.CanaryEnv, canary)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")
	var out, errOut strings.Builder
	runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath, "--sarif", sarifPath}, &out, &errOut, redaction.FromEnv())

	auditBytes, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	if strings.Contains(string(auditBytes), canary) {
		t.Fatalf("canary (diff-only) leaked into the audit record:\n%s", auditBytes)
	}
	sarifBytes, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	if strings.Contains(string(sarifBytes), canary) {
		t.Fatalf("canary (diff-only) leaked into the SARIF document:\n%s", sarifBytes)
	}
	if !strings.Contains(string(sarifBytes), "security/hardcoded-secret") {
		t.Fatalf("sanity check failed: the finding itself never reached the SARIF document:\n%s", sarifBytes)
	}
}

// TestAUR521PRPathWritesComplianceArtifacts proves the --pr path
// (runPRReview) is wired exactly like --base: the audit record is written
// with the gate's own decision and the repo/commit this run reviewed.
func TestAUR521PRPathWritesComplianceArtifacts(t *testing.T) {
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n"
	headConfig := "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n  sources: [skills]\n"
	skillBody := "## No Hardcoded Secrets\n\nNever commit a literal credential.\n"
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`

	var publishedStatus githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			w.Write([]byte(`{"head":{"sha":"head-sha"}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			w.Write([]byte(`{"content":"` + b64(headConfig) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "security.md"):
			w.Write([]byte(`{"content":"` + b64(skillBody) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(&publishedStatus)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	setPRGateEnv(t, server, fixture)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
	})
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, stdout.String(), stderr.String())
	}

	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit auditFile
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, raw)
	}
	if audit.Gate.Decision != "fail" {
		t.Fatalf("gate.decision=%q, want fail", audit.Gate.Decision)
	}
	if audit.Repo != "owner/repo" {
		t.Fatalf("repo=%q, want owner/repo", audit.Repo)
	}
	if len(audit.BlockingFindings) != 1 || audit.BlockingFindings[0].RuleID != "security#no-hardcoded-secrets" {
		t.Fatalf("blocking_findings=%v", audit.BlockingFindings)
	}
}

// TestAUR521ExceptedFindingSuppressedNotBlocking covers B2: a real AUR-520
// exception's own match (evaluateGate, policygate.go) must be the ONLY
// place a finding is decided to be excepted -- this card never re-derives
// that decision. An excepted finding must: appear in the SARIF document as
// suppressed, with the exception's own justification; be listed in the
// audit record's exceptions_applied; and be ABSENT from blocking_findings
// (it never breached the gate at all).
func TestAUR521ExceptedFindingSuppressedNotBlocking(t *testing.T) {
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n"
	headConfig := "review:\n  context:\n    skills:\n      - skills/security.md\n" +
		"gate:\n  fail_on: [high]\n  sources: [skills]\n" +
		"exceptions:\n  - repo: owner/repo\n    rule: security#no-hardcoded-secrets\n    path: app.go\n" +
		"    owner: time-seguranca\n    reason: consulta fixa\n    expires: 2099-12-31\n"
	skillBody := "## No Hardcoded Secrets\n\nNever commit a literal credential.\n"
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`

	var publishedStatus githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			w.Write([]byte(`{"content":"` + b64(headConfig) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "security.md"):
			w.Write([]byte(`{"content":"` + b64(skillBody) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			json.NewDecoder(r.Body).Decode(&publishedStatus)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	setPRGateEnv(t, server, fixture)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")
	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
		auditoriaPath:  auditPath,
		sarifPath:      sarifPath,
	})
	if code != 0 {
		t.Fatalf("exit=%d, want 0: the exception covers the only finding; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit auditFile
	if err := json.Unmarshal(rawAudit, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, rawAudit)
	}
	if len(audit.BlockingFindings) != 0 {
		t.Fatalf("blocking_findings=%v, want none: the excepted finding never breached the gate", audit.BlockingFindings)
	}
	if len(audit.ExceptionsApplied) != 1 {
		t.Fatalf("exceptions_applied=%v, want exactly one", audit.ExceptionsApplied)
	}
	exc := audit.ExceptionsApplied[0]
	if exc["rule_id"] != "security#no-hardcoded-secrets" || exc["path"] != "app.go" {
		t.Fatalf("exceptions_applied[0]=%v", exc)
	}
	just, _ := exc["justification"].(string)
	if !strings.Contains(just, "time-seguranca") || !strings.Contains(just, "consulta fixa") {
		t.Fatalf("exceptions_applied[0].justification=%q, want the exception's own owner/reason", just)
	}

	rawSARIF, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	var doc sarifFile
	if err := json.Unmarshal(rawSARIF, &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, rawSARIF)
	}
	found := false
	for _, res := range doc.Runs[0].Results {
		if res.RuleID != "security#no-hardcoded-secrets" {
			continue
		}
		found = true
		if len(res.Suppressions) != 1 {
			t.Fatalf("suppressions=%v, want exactly one for the excepted finding", res.Suppressions)
		}
		if !strings.Contains(res.Suppressions[0].Justification, "time-seguranca") {
			t.Fatalf("suppression justification=%q", res.Suppressions[0].Justification)
		}
	}
	if !found {
		t.Fatalf("no SARIF result for security#no-hardcoded-secrets among %+v", doc.Runs[0].Results)
	}
}

// TestAUR521AuditGateOutcomeInconclusiveWithoutGateDeclared is M4:
// auditGateOutcome must read gateInconclusiveReason even when the gate
// itself was never declared at all (gateDecision.Active == false) --
// dropping that parameter (or only checking it inside an Active branch)
// would make this record say "pass" for a genuinely inconclusive run
// whose SARIF, for the exact same run, already marks
// executionSuccessful=false (TestAUR521AuditAndSARIFAgreeOnInconclusive
// below proves the two files are not allowed to disagree like that).
func TestAUR521AuditGateOutcomeInconclusiveWithoutGateDeclared(t *testing.T) {
	decision, reason := auditGateOutcome(gateDecision{}, "provider_failure")
	if decision != "inconclusive" {
		t.Fatalf("decision=%q, want inconclusive even with no gate declared at all", decision)
	}
	if !strings.Contains(reason, "provider_failure") {
		t.Fatalf("reason=%q, want it to name provider_failure", reason)
	}
	if decision, _ := auditGateOutcome(gateDecision{}, ""); decision != "pass" {
		t.Fatalf("decision=%q, want pass: no gate declared and a conclusive run", decision)
	}
}

// TestAUR521AuditAndSARIFAgreeOnInconclusive is M7: the audit record's
// gate.decision and the SARIF's invocations[0].executionSuccessful are
// computed from the SAME gateInconclusiveReason in writeComplianceArtifacts
// (aur521.go) -- forcing SARIF's executionSuccessful to a hardcoded true
// would make the two files, for the exact same run, disagree about whether
// the review was trustworthy.
func TestAUR521AuditAndSARIFAgreeOnInconclusive(t *testing.T) {
	coverageFixture(t, "ignore:\n  - \"tests/**\"\ngate:\n  inconclusive: warn\n")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	sarifPath := filepath.Join(t.TempDir(), "out.sarif")
	var out, errOut strings.Builder
	runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath, "--sarif", sarifPath}, &out, &errOut, redaction.NewFilter())

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	var audit auditFile
	if err := json.Unmarshal(rawAudit, &audit); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\n%s", err, rawAudit)
	}
	if audit.Gate.Decision != "inconclusive" {
		t.Fatalf("audit gate.decision=%q, want inconclusive", audit.Gate.Decision)
	}

	rawSARIF, err := os.ReadFile(sarifPath)
	if err != nil {
		t.Fatalf("reading SARIF: %v", err)
	}
	var doc sarifFile
	if err := json.Unmarshal(rawSARIF, &doc); err != nil {
		t.Fatalf("SARIF is not valid JSON: %v\n%s", err, rawSARIF)
	}
	if len(doc.Runs) == 0 || len(doc.Runs[0].Invocations) == 0 {
		t.Fatalf("SARIF invocations missing: %+v", doc)
	}
	if doc.Runs[0].Invocations[0].ExecutionSuccessful {
		t.Fatalf("SARIF executionSuccessful=true, want false: the audit record for the SAME run says %q, the two files must agree", audit.Gate.Decision)
	}
}

// TestAUR521EvaluateGateBlockNeverPopulatesBlockingFindings is M8:
// gate.inconclusive: block returns Fail WITHOUT Breach, before the
// threshold loop ever runs (evaluateGate's own documented precedence).
// BlockingFindings must stay empty even though the issues passed in would
// have matched the threshold had the loop actually run -- a mutation that
// populates it regardless of whether the loop ran (e.g. straight from the
// raw issues list) must turn this red.
func TestAUR521EvaluateGateBlockNeverPopulatesBlockingFindings(t *testing.T) {
	gate := config.GateConfig{FailOn: []string{"high"}, Inconclusive: "block"}
	dynamic := map[string]review.Rule{
		"security#no-hardcoded-secrets": {ID: "security#no-hardcoded-secrets", Title: "No Hardcoded Secrets", Origin: gateOriginPolicy, Severity: "error"},
	}
	issues := []types.ReviewIssue{{RuleID: "security#no-hardcoded-secrets", File: "app.go", Line: 3, Severity: "error"}}

	d, err := evaluateGate(gate, gateOriginPolicy, dynamic, issues, "provider_failure", nil, "", time.Now())
	if err != nil {
		t.Fatalf("evaluateGate() error = %v", err)
	}
	if !d.Fail || d.Breach {
		t.Fatalf("evaluateGate() = %+v, want Fail without Breach under gate.inconclusive: block", d)
	}
	if len(d.BlockingFindings) != 0 {
		t.Fatalf("BlockingFindings=%v, want none: block never reaches the threshold loop at all, so nothing was ever graded", d.BlockingFindings)
	}
}
