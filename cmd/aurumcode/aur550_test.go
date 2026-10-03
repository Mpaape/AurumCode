package main

// AUR-550 behavior proof, driven through the real `aurumcode review`
// command (runReview/runPRReview), exactly like aur519_e2e_test.go/
// aur537_test.go already do for their own cards. A fake Dependency-Track
// v5 server (httptest, no real network) stands in for the server this
// card never generates a SBOM for (AUR-549's own job, a CycloneDX fixture
// here) and never administers (the card's own Non-goal).

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	igate "github.com/Mpaape/AurumCode/internal/gate"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// dtrackFakeServer is this test file's own minimal Dependency-Track v5
// double: upload always succeeds with a fixed token, GET .../bom/token/
// reports "processing" for pendingPolls calls and then finishes, and GET
// .../metrics/project/.../current returns the configured numbers (or an
// incomplete body, for AC-003's own "never a silent zero" case).
type dtrackFakeServer struct {
	uploadCalls  atomic.Int32
	pollCalls    atomic.Int32
	metricsCalls atomic.Int32
	seenAPIKeys  []string
	pendingPolls int32
	statusCode   int // non-zero: every call fails with this HTTP status
	// uploadStatusCode, when non-zero, fails ONLY the upload call with
	// this status -- a real Dependency-Track server rejecting a BOM it
	// cannot ingest (e.g. a CycloneDX specVersion newer than the
	// server's own minimum, see docs/specs/AUR-550.md) never touches
	// /bom/token or /metrics at all; this is distinct from statusCode
	// above, which fails every endpoint uniformly.
	uploadStatusCode int

	critical, high, violations int
	metricsIncomplete          bool
}

func (f *dtrackFakeServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.seenAPIKeys = append(f.seenAPIKeys, r.Header.Get("X-Api-Key"))
		if f.statusCode != 0 {
			// AC-004's own canary: a real Dependency-Track error response
			// can echo request content back; this fake does the same on
			// purpose, so the test proves the key never reaches a
			// published sink even when the server itself hands it back.
			w.WriteHeader(f.statusCode)
			_, _ = w.Write([]byte(`{"error":"rejected key ` + r.Header.Get("X-Api-Key") + `"}`))
			return
		}
		if f.uploadStatusCode != 0 && r.Method == http.MethodPost && r.URL.Path == "/api/v1/bom" {
			f.uploadCalls.Add(1)
			w.WriteHeader(f.uploadStatusCode)
			_, _ = w.Write([]byte(`{"error":"unsupported CycloneDX specVersion"}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/bom":
			f.uploadCalls.Add(1)
			if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("project") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/bom/token/"):
			n := f.pollCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]bool{"processing": n <= f.pendingPolls})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/refresh"):
			// AUR-570: the optional metrics refresh is accepted, not counted.
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/metrics/project/"):
			f.metricsCalls.Add(1)
			if f.metricsIncomplete {
				_ = json.NewEncoder(w).Encode(map[string]int64{"lastOccurrence": 4102444800000})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int64{
				"critical": int64(f.critical), "high": int64(f.high), "policyViolationsTotal": int64(f.violations),
				"lastOccurrence": 4102444800000, // AUR-570: settled, later than any upload
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// useFakeDTrackClock overrides this card's own injectable clock/sleeper
// (aur550.go) so a configured timeout never costs a test real wall-clock
// time, restoring the production defaults on cleanup.
func useFakeDTrackClock(t *testing.T) {
	t.Helper()
	origNow, origSleep := igate.DTrackClockNow, igate.DTrackSleeper
	cur := time.Unix(0, 0)
	igate.DTrackClockNow = func() time.Time { return cur }
	igate.DTrackSleeper = func(d time.Duration) { cur = cur.Add(d) }
	t.Cleanup(func() { igate.DTrackClockNow, igate.DTrackSleeper = origNow, origSleep })
}

func writeSBOMFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sbom_app_cyclonedx.json")
	body := `{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":[]}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func dtrackConfigYAML(serverURL, bomPath string, inconclusive string) string {
	cfg := "quality_gates:\n" +
		"  ssor_dtrack:\n" +
		"    enabled: true\n" +
		"    server_api_host: \"" + serverURL + "\"\n" +
		"    api_key_secret: DTRACK_API_KEY\n" +
		"    project_id_secret: DTRACK_PROJECT_ID\n" +
		"    thresholds: {max_critical: 0, max_high: 0, policy_violations: 0}\n" +
		"    timeout_seconds: 5\n" +
		"    poll_interval_seconds: 1\n" +
		"    sbom_generator:\n" +
		"      output_file: \"" + bomPath + "\"\n"
	if inconclusive != "" {
		cfg += "gate:\n  inconclusive: " + inconclusive + "\n"
	}
	return cfg
}

func approveFixture(t *testing.T) string {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// TestAUR550BreachFailsGateWithNumbers covers AC-001: metrics over the
// configured thresholds fail the gate, with the metric numbers published
// in stderr/limitations and in the audit record's blocking findings.
func TestAUR550BreachFailsGateWithNumbers(t *testing.T) {
	fs := &dtrackFakeServer{pendingPolls: 0, critical: 3, high: 1, violations: 2}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	dir := cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, ""))
	t.Setenv("DTRACK_API_KEY", "breach-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	auditPath := filepath.Join(dir, "audit.json")
	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	for _, want := range []string{"critical 3 > max_critical 0", "high 1 > max_high 0", "policy_violations 2 > policy_violations 0"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected %q in output:\n%s", want, combined)
		}
	}
	auditRaw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	if !strings.Contains(string(auditRaw), "ssor_dtrack") {
		t.Fatalf("expected the audit record to name ssor_dtrack's own blocking finding:\n%s", auditRaw)
	}
	var audit struct {
		Gate struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		} `json:"gate"`
	}
	if err := json.Unmarshal(auditRaw, &audit); err != nil {
		t.Fatalf("parsing audit record: %v", err)
	}
	if audit.Gate.Decision != "fail" {
		t.Fatalf("audit gate.decision=%q, want fail", audit.Gate.Decision)
	}
	for _, want := range []string{"critical 3 > max_critical 0", "high 1 > max_high 0", "policy_violations 2 > policy_violations 0"} {
		if !strings.Contains(audit.Gate.Reason, want) {
			t.Fatalf("expected %q in audit gate.reason=%q", want, audit.Gate.Reason)
		}
	}
}

// TestAUR550WithinLimitsApproves covers AC-002: metrics within every
// threshold approve this part of the gate (exit 0, breach never set).
func TestAUR550WithinLimitsApproves(t *testing.T) {
	fs := &dtrackFakeServer{pendingPolls: 2, critical: 0, high: 0, violations: 0}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	dir := cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, ""))
	_ = dir
	t.Setenv("DTRACK_API_KEY", "approve-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "ssor_dtrack: aprovado") {
		t.Fatalf("expected an explicit ssor_dtrack approval line:\n%s", combined)
	}
	if fs.uploadCalls.Load() != 1 || fs.metricsCalls.Load() != 2 {
		t.Fatalf("expected one upload and two coinciding metrics reads, got upload=%d metrics=%d", fs.uploadCalls.Load(), fs.metricsCalls.Load())
	}
	if fs.pollCalls.Load() < 2 {
		t.Fatalf("expected at least 2 poll calls (processing -> done), got %d", fs.pollCalls.Load())
	}
}

// TestAUR550PollingTimeoutIsInconclusive covers AC-003 (processing never
// finishes) and is MUT-001's own target: a server that never reports
// processing:false must make the gate inconclusive, never approved.
func TestAUR550PollingTimeoutIsInconclusive(t *testing.T) {
	fs := &dtrackFakeServer{pendingPolls: 1 << 20}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "block"))
	t.Setenv("DTRACK_API_KEY", "timeout-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "dtrack_timeout") {
		t.Fatalf("expected the stable reason token dtrack_timeout in the output:\n%s", combined)
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("a polling timeout must never read as approved:\n%s", out.String())
	}
}

// TestAUR550HTTPErrorIsInconclusive covers AC-003's HTTP-error case (401).
func TestAUR550HTTPErrorIsInconclusive(t *testing.T) {
	fs := &dtrackFakeServer{statusCode: http.StatusUnauthorized}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "block"))
	t.Setenv("DTRACK_API_KEY", "http-error-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "dtrack_http_error") {
		t.Fatalf("expected dtrack_http_error in the output:\n%s%s", out.String(), errOut.String())
	}
}

// TestAUR550UploadRejectedBySpecVersionIsInconclusive: a Dependency-Track
// server that rejects the upload itself with a 4xx (e.g. a CycloneDX
// specVersion newer than the server's own minimum -- see
// docs/specs/AUR-550.md on the pinned Trivy's 1.7 output needing
// Dependency-Track >= 5.1.0 / 4.14.4) must never be read as approved:
// inconclusive per policy, with zero poll/metrics calls (the upload never
// succeeded, so there is no token to poll and no project to grade).
func TestAUR550UploadRejectedBySpecVersionIsInconclusive(t *testing.T) {
	fs := &dtrackFakeServer{uploadStatusCode: http.StatusBadRequest}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "block"))
	t.Setenv("DTRACK_API_KEY", "upload-rejected-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "dtrack_http_error") {
		t.Fatalf("expected dtrack_http_error in the output:\n%s%s", out.String(), errOut.String())
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("a rejected upload must never read as approved:\n%s", out.String())
	}
	if fs.pollCalls.Load() != 0 || fs.metricsCalls.Load() != 0 {
		t.Fatalf("a rejected upload must never be polled or graded: poll=%d metrics=%d", fs.pollCalls.Load(), fs.metricsCalls.Load())
	}
}

// TestAUR550UnreachableServerIsInconclusive covers AC-003's unreachable
// case: a server closed before this run ever calls it.
func TestAUR550UnreachableServerIsInconclusive(t *testing.T) {
	fs := &dtrackFakeServer{}
	srv := httptest.NewServer(fs.handler())
	srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "block"))
	t.Setenv("DTRACK_API_KEY", "unreachable-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "dtrack_unreachable") {
		t.Fatalf("expected dtrack_unreachable in the output:\n%s%s", out.String(), errOut.String())
	}
}

// TestAUR550APIKeyNeverLeaks covers AC-004: the canary. The fake server
// deliberately echoes the API key back in its own error body (exactly
// like a real Dependency-Track error page could), and the server
// genuinely received the key in X-Api-Key -- proving the test is not
// vacuous -- yet the key must be absent from stdout, stderr, the
// published review body, the audit record and the SARIF document alike.
func TestAUR550APIKeyNeverLeaks(t *testing.T) {
	const canary = "CANARY-DTRACK-KEY-f00dbabe"
	fs := &dtrackFakeServer{statusCode: http.StatusUnauthorized}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	dir := cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "block"))
	t.Setenv("DTRACK_API_KEY", canary)
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	auditPath := filepath.Join(dir, "audit.json")
	sarifPath := filepath.Join(dir, "result.sarif")
	var out, errOut strings.Builder
	_ = runReview([]string{"--base", "HEAD~1", "--auditoria", auditPath, "--sarif", sarifPath}, &out, &errOut, redaction.NewFilter())

	if len(fs.seenAPIKeys) == 0 || fs.seenAPIKeys[0] != canary {
		t.Fatalf("test did not actually exercise the key: server saw %v, want first call = %q", fs.seenAPIKeys, canary)
	}

	auditRaw, _ := os.ReadFile(auditPath)
	sarifRaw, _ := os.ReadFile(sarifPath)
	for name, haystack := range map[string]string{
		"stdout": out.String(), "stderr": errOut.String(),
		"audit": string(auditRaw), "sarif": string(sarifRaw),
	} {
		if strings.Contains(haystack, canary) {
			t.Fatalf("the API key leaked into %s:\n%s", name, haystack)
		}
	}
}

// TestAUR550SecretNamesAreConfigurable covers AC-005: the environment
// variable NAMES the client reads come only from api_key_secret/
// project_id_secret, never a literal "DTRACK_API_KEY"/"DTRACK_PROJECT_ID"
// baked into this program -- a differently-named pair of variables must
// work identically.
func TestAUR550SecretNamesAreConfigurable(t *testing.T) {
	fs := &dtrackFakeServer{pendingPolls: 0}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cfg := "quality_gates:\n" +
		"  ssor_dtrack:\n" +
		"    enabled: true\n" +
		"    server_api_host: \"" + srv.URL + "\"\n" +
		"    api_key_secret: MY_CUSTOM_DTRACK_KEY\n" +
		"    project_id_secret: MY_CUSTOM_DTRACK_PROJECT\n" +
		"    thresholds: {max_critical: 0, max_high: 0, policy_violations: 0}\n" +
		"    timeout_seconds: 5\n" +
		"    poll_interval_seconds: 1\n" +
		"    sbom_generator:\n" +
		"      output_file: \"" + bomPath + "\"\n"
	cleanFixture(t, cfg)
	t.Setenv("MY_CUSTOM_DTRACK_KEY", "custom-named-key")
	t.Setenv("MY_CUSTOM_DTRACK_PROJECT", "custom-proj")
	t.Setenv("DTRACK_API_KEY", "")
	t.Setenv("DTRACK_PROJECT_ID", "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	found := false
	for _, k := range fs.seenAPIKeys {
		if k == "custom-named-key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("server never saw the custom-named env var's value; saw %v", fs.seenAPIKeys)
	}
}

// TestAUR550DisabledByDefaultNoOp proves that a run with no
// quality_gates.ssor_dtrack section is byte-identical to this card never
// having existed: no HTTP call at all.
func TestAUR550DisabledByDefaultNoOp(t *testing.T) {
	fs := &dtrackFakeServer{}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	cleanFixture(t, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if fs.uploadCalls.Load() != 0 || fs.pollCalls.Load() != 0 || fs.metricsCalls.Load() != 0 {
		t.Fatalf("expected zero dtrack HTTP calls with no ssor_dtrack section declared")
	}
}

// TestAUR550PollingTimeoutWarnsNeverBlocks covers AC-003 under
// gate.inconclusive: warn (the non-blocking mode): exit 0, the stable
// dtrack_timeout reason still visible, and approval never claimed. This
// is MUT-001's own "warn" counterpart to TestAUR550PollingTimeoutIsInconclusive's
// "block" case -- together they kill a mutation that hardcodes
// applyDTrackGate's blockOnInconclusive to true regardless of the
// configured mode (aur550.go).
func TestAUR550PollingTimeoutWarnsNeverBlocks(t *testing.T) {
	fs := &dtrackFakeServer{pendingPolls: 1 << 20}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "warn"))
	t.Setenv("DTRACK_API_KEY", "warn-timeout-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "dtrack_timeout") {
		t.Fatalf("expected dtrack_timeout still visible under warn:\n%s%s", out.String(), errOut.String())
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("an inconclusive result must never read as approved, even under warn:\n%s", out.String())
	}
}

// TestAUR550SecretMissingWarnsNeverBlocks covers the same "warn never
// blocks" rule for the missing-secret path (no server call at all is even
// possible without a key): exit 0, dtrack_secret_missing visible, never
// approved.
func TestAUR550SecretMissingWarnsNeverBlocks(t *testing.T) {
	fs := &dtrackFakeServer{}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, bomPath, "warn"))
	t.Setenv("DTRACK_API_KEY", "")
	t.Setenv("DTRACK_PROJECT_ID", "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "dtrack_secret_missing") {
		t.Fatalf("expected dtrack_secret_missing visible under warn:\n%s%s", out.String(), errOut.String())
	}
	if fs.uploadCalls.Load() != 0 {
		t.Fatalf("a missing secret must never even attempt an upload")
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("an inconclusive result must never read as approved, even under warn:\n%s", out.String())
	}
}

// dtrackPRMockServer is runPRGateMockServer (aur519_e2e_test.go) plus the
// ONE case that one is missing: POST .../issues/{number}/comments, the
// general-comment endpoint PostIssueComment uses to publish the review's
// own summary body (pr.go's `client.PostIssueComment(ctx, ..., summaryBody)`)
// whenever the model's own reply carries zero issues to attach as line/
// review comments -- exactly this test's own scenario (approveFixture, 0
// issues). Kept local to this file rather than editing the shared helper
// other cards' tests already depend on byte-for-byte.
func dtrackPRMockServer(t *testing.T, diffBody, repoConfig string, published *githubclient.CommitStatus, postedBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(repoConfig)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/issues/48/comments":
			if postedBody != nil {
				var payload struct {
					Body string `json:"body"`
				}
				_ = json.NewDecoder(r.Body).Decode(&payload)
				*postedBody += payload.Body
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(published)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s (Accept=%s)", r.Method, r.URL.Path, r.Header.Get("Accept"))
		}
	}))
}

// TestAUR550PRReviewBreachPublishesNumbersInBodyAndAudit covers AC-001 on
// the --pr path specifically (pr.go's own runPRReview, not just runReview):
// the breach numbers must reach the ACTUAL published review body (the
// general comment runPRReview posts through the GitHub API, captured here
// via dtrackPRMockServer's postedBody) and the audit record's own
// gate.reason field -- not merely stderr. Uses setPRGateEnv, the same env
// harness aur537_test.go/aur519_e2e_test.go already share.
func TestAUR550PRReviewBreachPublishesNumbersInBodyAndAudit(t *testing.T) {
	fs := &dtrackFakeServer{pendingPolls: 0, critical: 3, high: 1, violations: 2}
	dtrackSrv := httptest.NewServer(fs.handler())
	defer dtrackSrv.Close()
	useFakeDTrackClock(t)

	bomPath := writeSBOMFixture(t)
	repoConfig := dtrackConfigYAML(dtrackSrv.URL, bomPath, "")
	t.Setenv("DTRACK_API_KEY", "pr-breach-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")

	var published githubclient.CommitStatus
	var postedBody string
	ghServer := dtrackPRMockServer(t, simpleDiffAUR537, repoConfig, &published, &postedBody)
	defer ghServer.Close()
	setPRGateEnv(t, ghServer, approveFixture(t))

	auditPath := filepath.Join(t.TempDir(), "audit.json")
	var out, errOut strings.Builder
	code := runPRReview(&out, &errOut, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
		auditoriaPath: auditPath,
	})
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	for _, want := range []string{"critical 3 > max_critical 0", "high 1 > max_high 0"} {
		if !strings.Contains(postedBody, want) {
			t.Fatalf("expected %q in the PUBLISHED review body (not just stderr):\n%s", want, postedBody)
		}
	}

	var audit struct {
		Gate struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		} `json:"gate"`
	}
	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("reading audit record: %v", err)
	}
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatalf("parsing audit record: %v", err)
	}
	if audit.Gate.Decision != "fail" {
		t.Fatalf("audit gate.decision=%q, want fail", audit.Gate.Decision)
	}
	for _, want := range []string{"critical 3 > max_critical 0", "high 1 > max_high 0", "policy_violations 2 > policy_violations 0"} {
		if !strings.Contains(audit.Gate.Reason, want) {
			t.Fatalf("expected %q in audit gate.reason=%q", want, audit.Gate.Reason)
		}
	}
}
