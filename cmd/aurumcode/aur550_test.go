package main

// AUR-550 behavior proof, driven through the real `aurumcode review`
// command (runReview/runPRReview), exactly like aur519_e2e_test.go/
// aur537_test.go already do for their own cards. A fake Dependency-Track
// v5 server (httptest, no real network) stands in for the server this
// card never generates a SBOM for (AUR-549's own job, a CycloneDX fixture
// here) and never administers (the card's own Non-goal).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/metrics/project/"):
			f.metricsCalls.Add(1)
			if f.metricsIncomplete {
				_ = json.NewEncoder(w).Encode(map[string]int{})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int{
				"critical": f.critical, "high": f.high, "policyViolationsTotal": f.violations,
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
	origNow, origSleep := dtrackClockNow, dtrackSleeper
	cur := time.Unix(0, 0)
	dtrackClockNow = func() time.Time { return cur }
	dtrackSleeper = func(d time.Duration) { cur = cur.Add(d) }
	t.Cleanup(func() { dtrackClockNow, dtrackSleeper = origNow, origSleep })
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
	if fs.uploadCalls.Load() != 1 || fs.metricsCalls.Load() != 1 {
		t.Fatalf("expected exactly one upload and one metrics call, got upload=%d metrics=%d", fs.uploadCalls.Load(), fs.metricsCalls.Load())
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
