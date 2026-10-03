package main

// AUR-533 end-to-end proof through the real `aurumcode review` command, with
// a local fake GitHub (release listing + assets); no real network.

import (
	"encoding/json"
	igate "github.com/Mpaape/AurumCode/internal/gate"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/artifacts"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

var aur533Gen = time.Date(2026, 10, 2, 3, 17, 0, 0, time.UTC)

type aur533Fake struct {
	srv   *httptest.Server
	calls atomic.Int32
	m     *artifacts.Manifest
}

// newAUR533Fake publishes one artifact generated at aur533Gen. tamper
// ("", "payload", "manifest") corrupts it after the manifest is computed.
func newAUR533Fake(t *testing.T, tamper string) *aur533Fake {
	t.Helper()
	dir := t.TempDir()
	scanners := []byte("vuln_scanner_version: 0.73.0\n")
	if err := os.WriteFile(filepath.Join(dir, "scanners.yml"), scanners, 0o644); err != nil {
		t.Fatal(err)
	}
	d, n, _ := artifacts.HashFile(filepath.Join(dir, "scanners.yml"))
	m := &artifacts.Manifest{
		Schema: artifacts.SchemaV1, GeneratedAt: aur533Gen.Format(time.RFC3339),
		Files:    []artifacts.File{{Path: "scanners.yml", SHA256: d, Size: n, Kind: "scanners"}},
		Scanners: map[string]string{"vuln_scanner_version": "0.73.0"},
	}
	m.SetDigest = artifacts.ComputeSetDigest(m.Files)
	raw, _ := json.Marshal(m)
	if tamper == "manifest" {
		raw = []byte(strings.Replace(string(raw), m.SetDigest, "sha256:"+strings.Repeat("0", 64), 1))
	}
	_ = os.WriteFile(filepath.Join(dir, artifacts.ManifestName), raw, 0o644)
	if tamper == "payload" {
		_ = os.WriteFile(filepath.Join(dir, "scanners.yml"), []byte("vuln_scanner_version: 9.9.9\n"), 0o644)
	}
	f := &aur533Fake{m: m}
	tag, _ := m.Tag()
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		base := f.srv.URL + "/dl/"
		_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": tag, "created_at": m.GeneratedAt, "assets": []map[string]string{
			{"name": artifacts.ManifestName, "browser_download_url": base + artifacts.ManifestName},
			{"name": "scanners.yml", "browser_download_url": base + "scanners.yml"},
		}}})
	})
	mux.Handle("/dl/", http.StripPrefix("/dl/", http.FileServer(http.Dir(dir))))
	return f
}

func useAUR533Env(t *testing.T, api string, now time.Time) {
	t.Helper()
	oa, on, oc := igate.AnalysisDataAPIBase, igate.AnalysisDataNow, igate.AnalysisDataCache
	cache := t.TempDir()
	igate.AnalysisDataAPIBase = api
	igate.AnalysisDataNow = func() time.Time { return now }
	igate.AnalysisDataCache = func() string { return cache }
	t.Cleanup(func() { igate.AnalysisDataAPIBase, igate.AnalysisDataNow, igate.AnalysisDataCache = oa, on, oc })
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))
}

func aur533Config(maxAge string, mode string) string {
	cfg := "analysis_data:\n  repository: o/r\n"
	if maxAge != "" {
		cfg += "  max_age_days: " + maxAge + "\n"
	}
	if mode != "" {
		cfg += "gate:\n  inconclusive: " + mode + "\n"
	}
	return cfg
}

func runAUR533(t *testing.T, extra ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := runReview(append([]string{"--base", "HEAD~1"}, extra...), &out, &errOut, redaction.NewFilter())
	return code, out.String(), errOut.String()
}

// Nothing declared anywhere: no network call, no new line.
func TestAUR533UndeclaredSectionChangesNothing(t *testing.T) {
	f := newAUR533Fake(t, "")
	cleanFixture(t, "")
	useAUR533Env(t, f.srv.URL, aur533Gen)
	code, out, errOut := runAUR533(t)
	if code != 0 || f.calls.Load() != 0 {
		t.Fatalf("exit=%d network calls=%d\n%s%s", code, f.calls.Load(), out, errOut)
	}
	if strings.Contains(out+errOut, "analysis_data") {
		t.Fatalf("undeclared section must add no line:\n%s%s", out, errOut)
	}
}

// AC-002 end to end: a fresh artifact is used and recorded in the audit.
func TestAUR533FreshArtifactIsRecordedInAudit(t *testing.T) {
	f := newAUR533Fake(t, "")
	dir := cleanFixture(t, aur533Config("7", "block"))
	useAUR533Env(t, f.srv.URL, aur533Gen.Add(24*time.Hour))
	audit := filepath.Join(dir, "audit.json")
	code, out, errOut := runAUR533(t, "--auditoria", audit)
	if code != 0 {
		t.Fatalf("fresh artifact must not fail: exit=%d\n%s%s", code, out, errOut)
	}
	if strings.Contains(out+errOut, "inconclusiva") {
		t.Fatalf("fresh artifact produced an inconclusive line:\n%s%s", out, errOut)
	}
	raw, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	tag, _ := f.m.Tag()
	ad, _ := rec["analysis_data"].(map[string]any)
	if ad["digest"] != f.m.SetDigest || ad["generated_at"] != "2026-10-02T03:17:00Z" || ad["tag"] != tag {
		t.Fatalf("audit lacks digest/date/tag: %s", raw)
	}
	if ad["source"] != "remote" {
		t.Fatalf("audit source = %v", ad["source"])
	}
	if rec["policy_digest"] == nil {
		t.Fatal("existing audit fields must survive")
	}
}

// AC-003 end to end: stale is inconclusive by the policy's mode, with reason.
func TestAUR533StaleArtifactIsInconclusiveByMode(t *testing.T) {
	for _, tc := range []struct {
		mode     string
		wantFail bool
	}{{"block", true}, {"warn", false}} {
		t.Run(tc.mode, func(t *testing.T) {
			f := newAUR533Fake(t, "")
			cleanFixture(t, aur533Config("7", tc.mode))
			useAUR533Env(t, f.srv.URL, aur533Gen.Add(20*24*time.Hour))
			code, out, errOut := runAUR533(t)
			all := out + errOut
			if !strings.Contains(all, "analysis_data_stale") || !strings.Contains(all, "inconclusiva") {
				t.Fatalf("missing inconclusive reason:\n%s", all)
			}
			if tc.wantFail && code == 0 {
				t.Fatalf("block mode must fail, exit=0\n%s", all)
			}
			if !tc.wantFail && code != 0 {
				t.Fatalf("warn mode must not fail, exit=%d\n%s", code, all)
			}
			if strings.Contains(all, "analysis_data: aprovado") {
				t.Fatal("never approved")
			}
		})
	}
}

// AC-004 end to end: tampered payload or manifest, and unreachable source.
func TestAUR533DigestMismatchAndUnavailableAreInconclusive(t *testing.T) {
	for _, tamper := range []string{"payload", "manifest"} {
		f := newAUR533Fake(t, tamper)
		cleanFixture(t, aur533Config("7", "block"))
		useAUR533Env(t, f.srv.URL, aur533Gen.Add(time.Hour))
		code, out, errOut := runAUR533(t)
		if code == 0 || !strings.Contains(out+errOut, "analysis_data_digest_mismatch") {
			t.Fatalf("%s tamper: exit=%d\n%s%s", tamper, code, out, errOut)
		}
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	cleanFixture(t, aur533Config("7", "block"))
	useAUR533Env(t, dead.URL, aur533Gen)
	code, out, errOut := runAUR533(t)
	if code == 0 || !strings.Contains(out+errOut, "analysis_data_unavailable") {
		t.Fatalf("unavailable: exit=%d\n%s%s", code, out, errOut)
	}
}

// A central policy declaring analysis_data wins; the repo cannot loosen it.
func TestAUR533CentralPolicyAgeWinsOverRepository(t *testing.T) {
	f := newAUR533Fake(t, "")
	cleanFixture(t, aur533Config("300", "block"))
	policy := policyFixture(t, "analysis_data:\n  repository: o/r\n  max_age_days: 2\ngate:\n  inconclusive: block\n")
	useAUR533Env(t, f.srv.URL, aur533Gen.Add(5*24*time.Hour))
	code, out, errOut := runAUR533(t, "--politica", policy)
	all := out + errOut
	if code == 0 || !strings.Contains(all, "analysis_data_stale") || !strings.Contains(all, "max_age_days=2") {
		t.Fatalf("policy's 2-day limit must govern: exit=%d\n%s", code, all)
	}
	if !strings.Contains(all, "analysis_data do config do repositório foi ignorado") {
		t.Fatalf("overridden repo section must warn:\n%s", all)
	}
}

func runAUR533PR(t *testing.T, repoConfig, auditPath string) (code int, published githubclient.CommitStatus, body string) {
	t.Helper()
	gh := dtrackPRMockServer(t, simpleDiffAUR537, repoConfig, &published, &body)
	t.Cleanup(gh.Close)
	setPRGateEnv(t, gh, approveFixture(t))
	var out, errOut strings.Builder
	code = runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: true, auditoriaPath: auditPath})
	body += out.String() + errOut.String()
	return code, published, body
}

// --pr: a valid artifact is recorded in the audit by the typed field.
func TestAUR533PRFreshArtifactIsRecordedInAudit(t *testing.T) {
	f := newAUR533Fake(t, "")
	useAUR533Env(t, f.srv.URL, aur533Gen.Add(time.Hour))
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, published, _ := runAUR533PR(t, aur533Config("7", "block"), audit)
	if code != 0 || published.State != "success" {
		t.Fatalf("exit=%d status=%q", code, published.State)
	}
	raw, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		AnalysisData *render.AnalysisDataAudit `json:"analysis_data"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil || rec.AnalysisData == nil {
		t.Fatalf("audit lacks analysis_data (%d bytes)", len(raw))
	}
	tag, _ := f.m.Tag()
	if rec.AnalysisData.Digest != f.m.SetDigest || rec.AnalysisData.GeneratedAt != "2026-10-02T03:17:00Z" || rec.AnalysisData.Tag != tag {
		t.Fatalf("wrong analysis_data: %+v", rec.AnalysisData)
	}
}

// --pr: a stale artifact in block mode is not approved and the commit status
// reflects it.
func TestAUR533PRStaleArtifactBlocksAndStatusReflectsIt(t *testing.T) {
	f := newAUR533Fake(t, "")
	useAUR533Env(t, f.srv.URL, aur533Gen.Add(20*24*time.Hour))
	code, published, body := runAUR533PR(t, aur533Config("7", "block"), "")
	if code == 0 {
		t.Fatalf("block + stale must not exit 0 (status %q)", published.State)
	}
	if published.State == "success" || published.State == "" {
		t.Fatalf("status must not be success: %q", published.State)
	}
	if !strings.Contains(body, "analysis_data_stale") {
		t.Fatalf("published body must carry the reason (body %d bytes)", len(body))
	}
}

// Listing unreachable but a verified cached copy exists: usable, marked as
// cache in the audit and in the review's own line.
func TestAUR533CacheFallbackIsMarkedInAuditAndReview(t *testing.T) {
	f := newAUR533Fake(t, "")
	dir := cleanFixture(t, aur533Config("7", "block"))
	useAUR533Env(t, f.srv.URL, aur533Gen.Add(time.Hour))
	if code, out, errOut := runAUR533(t); code != 0 {
		t.Fatalf("priming run failed: %d\n%s%s", code, out, errOut)
	}
	f.srv.Close() // listing now unreachable; the cache (same dir) remains
	audit := filepath.Join(dir, "audit.json")
	code, out, errOut := runAUR533(t, "--auditoria", audit)
	if code != 0 || !strings.Contains(out+errOut, "cópia em cache") {
		t.Fatalf("cache fallback: exit=%d\n%s%s", code, out, errOut)
	}
	raw, _ := os.ReadFile(audit)
	var rec struct {
		AnalysisData *render.AnalysisDataAudit `json:"analysis_data"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil || rec.AnalysisData == nil || rec.AnalysisData.Source != "cache" {
		t.Fatalf("audit must record source=cache (%d bytes)", len(raw))
	}
}
