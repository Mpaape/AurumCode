package main

// AUR-519 end-to-end behavior proof: the gate wired into runReview (--base)
// and runPRReview's own publishPolicyGateStatus, driven through the real
// `aurumcode review` command with AURUMCODE_LLM_FIXTURE, exactly like
// aur476_test.go/aur518_test.go already do. Each test fails if the gate
// behavior it names is removed.

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
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

// cleanFixture builds a minimal git repository whose HEAD~1..HEAD diff
// changes app.go with no secret-shaped content at all -- unlike
// coverageFixture/localPassFixture (which both deliberately embed a
// hardcoded-secret line for their own cards), so a test here is never
// confused by the unrelated, pre-existing deterministic analysis pass.
func cleanFixture(t *testing.T) string {
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
	base := commit("package demo\nfunc Change() int {\n return 1\n}\n", "")
	source := "package demo\nfunc Change() int {\n return 42\n}\n"
	head := commit(source, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(source))
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir
}

// TestAUR519GateSeverityBreachFailsCheck covers AC-001: a finding citing a
// repo-opted-in skill section at or above gate.fail_on's threshold fails
// the check (exitFindings) and names the skill and section in the output.
func TestAUR519GateSeverityBreachFailsCheck(t *testing.T) {
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n")
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

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "security#no-hardcoded-secrets") {
		t.Fatalf("expected the skill/section id named in the output:\n%s", combined)
	}
	if !strings.Contains(combined, "No Hardcoded Secrets") {
		t.Fatalf("expected the section title named in the output:\n%s", combined)
	}
}

// TestAUR519GateInconclusiveProviderFailureBlocks covers AC-003 (and is
// MUT-001's anchor): with gate.inconclusive: block, a provider that was
// configured and failed makes the check fail, with the policy-gate's own
// line naming "provider_failure" reaching the output, and the verdict
// never reads as approved.
func TestAUR519GateInconclusiveProviderFailureBlocks(t *testing.T) {
	coverageFixture(t, "gate:\n  inconclusive: block\n")
	// A configured-but-unreadable fixture is a provider that was selected
	// and then failed -- distinct from "nothing configured at all" -- so
	// qualityFailed, not qualitySkipped, drives this scenario (AUR-458).
	t.Setenv("AURUMCODE_LLM_FIXTURE", filepath.Join(t.TempDir(), "missing-response.json"))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--seguranca"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "policy gate: review inconclusive (provider_failure)") {
		t.Fatalf("expected the policy gate's own inconclusive line naming provider_failure:\n%s", combined)
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("verdict must never read as approved under a blocking, inconclusive gate:\n%s", out.String())
	}
}

// TestAUR519GatePartialCoverageInconclusiveWarns covers AC-004: AUR-476's
// own partial-coverage detection feeds the same gate.inconclusive
// configuration. With "warn", the check still passes (exit 0) but the
// alert is visible and the verdict is pulled off "approve" -- neither of
// which existed before this card (a clean, 0-issue "approve" response with
// a config-hidden file used to publish untouched).
func TestAUR519GatePartialCoverageInconclusiveWarns(t *testing.T) {
	coverageFixture(t, "ignore:\n  - \"tests/**\"\ngate:\n  inconclusive: warn\n")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "policy gate: review inconclusive (partial_coverage)") {
		t.Fatalf("expected a visible policy-gate alert naming partial_coverage:\n%s", combined)
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("verdict must not read as approved once the gate flags the review inconclusive, even under warn:\n%s", out.String())
	}
}

// TestAUR519PolicyGateStatusContextIsStable covers AC-006: the policy
// gate's own commit status uses a fixed, documented context name
// ("aurumcode/policy-gate"), distinct from --check's own "aurumcode/review"
// (checkContext), so an org's branch-protection ruleset can require it on
// its own. Mirrors TestRequiredPRQualityFailurePublishesFailingStatus's own
// pattern for publishCheckStatus.
func TestAUR519PolicyGateStatusContextIsStable(t *testing.T) {
	if policyGateContext != "aurumcode/policy-gate" {
		t.Fatalf("policyGateContext changed to %q -- this is a published, stable commit-status contract (AC-006)", policyGateContext)
	}
	var published []githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head":
			var status githubclient.CommitStatus
			if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
				t.Error(err)
			}
			published = append(published, status)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := githubclient.NewClientWithBaseURL("test-token", server.URL)

	var out, errOut strings.Builder
	noGate := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", gateDecision{Active: false}, 42)
	if noGate != 0 || len(published) != 0 {
		t.Fatalf("an undeclared gate must publish nothing at all: exit=%d published=%+v", noGate, published)
	}

	breach := gateDecision{Active: true, Fail: true, Lines: []string{"security#no-hardcoded-secrets: No Hardcoded Secrets (severidade error, limiar error)"}}
	code := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", breach, 42)
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings; stderr=%s", code, errOut.String())
	}
	if len(published) != 1 || published[0].Context != policyGateContext || published[0].State != "failure" {
		t.Fatalf("published=%+v", published)
	}
}

// TestAUR519NoGateConfiguredStaysUntouched proves that without any `gate:`
// key anywhere, the review carries no trace of this card at all: no
// policy-gate line, and a clean response's own "approve" verdict still
// publishes exactly as before AUR-519.
func TestAUR519NoGateConfiguredStaysUntouched(t *testing.T) {
	cleanFixture(t)
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String()+errOut.String(), "policy gate") {
		t.Fatalf("no gate was configured; the review must carry no policy-gate trace at all:\n%s", out.String()+errOut.String())
	}
	if !strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("without a gate, a clean approve verdict must still publish unchanged:\n%s", out.String())
	}
}
