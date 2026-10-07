package main

// AUR-567: the gate line carries the typed origin for every source, the same
// value the audit records; the gate line and the report show the same message;
// the formal review follows the gate when one is declared.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

func aur567AllSources(t *testing.T) (code int, stdout, stderr string, audit auditFile) {
	t.Helper()
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\nquality_gates:\n  sast:\n    enabled: true\n")
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
	setSemgrepPATH(t, semgrepFake(t, semgrepErrorFinding, false, ""))
	auditPath := filepath.Join(t.TempDir(), "audit.json")
	var o, e strings.Builder
	code = runReview([]string{"--base", "HEAD~1", "--seguranca", "--auditoria", auditPath}, &o, &e, redaction.NewFilter())
	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatal(err)
	}
	return code, o.String(), e.String(), audit
}

// AC-001: one finding of each source; every gate line says `origem <origin>`
// with the audit's own origin, and no line says `origem policy`.
func TestAUR567GateLineCarriesTypedOriginForEverySource(t *testing.T) {
	code, out, errOut, audit := aur567AllSources(t)
	if code != exitFindings {
		t.Fatalf("exit=%d, want %d; stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	seen := map[string]bool{}
	for _, f := range audit.BlockingFindings {
		seen[f.Origin] = true
		found := false
		for _, line := range strings.Split(errOut, "\n") {
			if strings.Contains(line, "policy gate: ") && strings.Contains(line, f.RuleID) {
				found = true
				if !strings.Contains(line, "origem "+f.Origin) {
					t.Errorf("line for %s must say origem %s: %s", f.RuleID, f.Origin, line)
				}
			}
		}
		if !found {
			t.Errorf("no gate line for %s:\n%s", f.RuleID, errOut)
		}
	}
	for _, want := range []string{"skills", "analysis", "sast", "security"} {
		if !seen[want] {
			t.Errorf("audit lacks a %s finding: %+v", want, audit.BlockingFindings)
		}
	}
	if strings.Contains(errOut, "origem policy") || strings.Contains(errOut, "origem repo") {
		t.Errorf("a section origin must not stand in for the typed origin:\n%s", errOut)
	}
}

var aur567ReportLine = regexp.MustCompile(`(?m)^app\.go:\d+: \[error\] (.+)$`)

// AC-002: the gate line shows the report's message after redaction, not a
// message whose first word the redaction filter replaced.
func TestAUR567GateLineAndReportShowTheSameMessage(t *testing.T) {
	_, out, errOut, _ := aur567AllSources(t)
	checked := 0
	for _, m := range aur567ReportLine.FindAllStringSubmatch(out, -1) {
		msg := m[1]
		if !strings.Contains(msg, "Hardcoded secret or credential assigned inline") {
			continue
		}
		checked++
		if !strings.Contains(errOut, msg) {
			t.Errorf("gate line must carry the report message %q:\n%s", msg, errOut)
		}
	}
	if checked == 0 {
		t.Fatalf("report has no analysis hardcoded-secret line:\n%s", out)
	}
	if !strings.Contains(errOut, "(rule security/hardcoded-secret - Hardcoded Secrets)") || strings.Contains(errOut, "[REDACTED] Secrets") {
		t.Errorf("the security line must show the full rule title:\n%s", errOut)
	}
	if strings.Contains(errOut, "[REDACTED] secret or credential") {
		t.Errorf("redaction replaced the title of the rule:\n%s", errOut)
	}
}

const aur567PRSecretDiff = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n"

func aur567PR(t *testing.T, diffBody, cfg, reply string, opts prReviewOptions) (code int, status githubclient.CommitStatus, event string, stderr string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = w.Write([]byte(`{"head":{"sha":"head-sha"}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = w.Write([]byte(`{"content":"` + b64(cfg) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			var body struct {
				Event string `json:"event"`
				Body  string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			event = body.Event + "\n" + body.Body
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(&status)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(reply), 0600); err != nil {
		t.Fatal(err)
	}
	setPRGateEnv(t, server, fixture)
	opts.publicationSet, opts.publication = true, "review"
	var o, e strings.Builder
	code = runPRReview(reviewIO{stdout: &o, stderr: &e, filter: redaction.NewFilter()}, func() prReviewOptions {
		o := opts
		o.prNumber, o.repo, o.publicar, o.naLinha, o.check = 48, "owner/repo", true, true, true
		return o
	}())
	return code, status, event, e.String()
}

// The --pr path counts the security pass: exit 3, status failure, origin security.
func TestAUR567PRSecurityPassFindingFailsGate(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, status, posted, errOut := aur567PR(t, aur567PRSecretDiff, "gate:\n  fail_on: [high]\n  inconclusive: warn\n",
		`{"summary":"ok","verdict":"approve","issues":[]}`, prReviewOptions{seguranca: true, auditoriaPath: audit})
	if code != exitFindings {
		t.Fatalf("exit=%d, want %d; stderr=%s", code, exitFindings, errOut)
	}
	if status.Context != policyGateContext || status.State != "failure" {
		t.Errorf("status=%+v, want failure on %s", status, policyGateContext)
	}
	if !strings.Contains(errOut, "security/hardcoded-secret") || !strings.Contains(errOut, "origem security") {
		t.Errorf("gate line must name the rule and origem security:\n%s", errOut)
	}
	if !strings.Contains(posted, "origem security") {
		t.Errorf("published parecer must cite origem security:\n%s", posted)
	}
	raw, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	var rec auditFile
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, f := range rec.BlockingFindings {
		ok = ok || (f.RuleID == "security/hardcoded-secret" && f.Origin == "security")
	}
	if !ok {
		t.Errorf("audit must carry the security origin: %+v", rec.BlockingFindings)
	}
}

// The formal review follows the gate when one is declared: a warning below the
// threshold is COMMENT, a breach is REQUEST_CHANGES; without a gate the
// historical rule (warning requests changes) stays.
func TestAUR567FormalReviewFollowsDeclaredGate(t *testing.T) {
	warnReply := `{"summary":"ok","verdict":"comment","issues":[{"file":"app.go","line":2,"severity":"warning","rule_id":"quality/high-complexity","message":"Function is complex","evidence":"func Change() {","impact":"Hard to read","verification":"Split it"}]}`
	const diff = "diff --git a/app.go b/app.go\n@@ -1,1 +1,2 @@\n package demo\n+func Change() {}\n"
	cases := []struct {
		name, cfg, reply, diff, wantEvent string
		wantCode                          int
	}{
		{"gate-warning-abaixo-do-limiar", "gate:\n  fail_on: [high]\n", warnReply, diff, "COMMENT", 0},
		{"gate-reprova", "gate:\n  fail_on: [high]\n", warnReply, aur567PRSecretDiff, "REQUEST_CHANGES", exitFindings},
		{"sem-gate", "review: {}\n", warnReply, diff, "REQUEST_CHANGES", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, status, posted, errOut := aur567PR(t, tc.diff, tc.cfg, tc.reply, prReviewOptions{})
			if code != tc.wantCode {
				t.Fatalf("exit=%d, want %d; stderr=%s", code, tc.wantCode, errOut)
			}
			if !strings.HasPrefix(posted, tc.wantEvent+"\n") {
				t.Errorf("formal review event, want %s:\n%s", tc.wantEvent, posted)
			}
			if tc.name == "gate-warning-abaixo-do-limiar" && status.State == "failure" {
				t.Errorf("checks must not fail when the review only comments: %+v", status)
			}
		})
	}
}

func TestAUR567GateAlignedReviewEvent(t *testing.T) {
	for _, tc := range []struct {
		event           string
		declared, fails bool
		want            string
	}{
		{"REQUEST_CHANGES", false, false, "REQUEST_CHANGES"},
		{"APPROVE", false, true, "APPROVE"},
		{"REQUEST_CHANGES", true, false, "COMMENT"},
		{"COMMENT", true, true, "REQUEST_CHANGES"},
		{"APPROVE", true, false, "APPROVE"},
	} {
		if got := blocking.FromGate(tc.declared, gateDecision{Fail: tc.fails}).Event(tc.event); got != tc.want {
			t.Errorf("%+v: got %s", tc, got)
		}
	}
}
